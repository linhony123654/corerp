package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

const rpInformationPhase = "rp_information_delivery"
const rpInformationDeliverKind = "rp_information_deliver"

type RPInformationSendRequest struct {
	Binding           core.CareerBinding `json:"binding"`
	SessionID         string             `json:"session_id"`
	MessageID         string             `json:"message_id"`
	RecipientEntityID string             `json:"recipient_entity_id"`
	Text              string             `json:"text"`
	AllowRelay        bool               `json:"allow_relay,omitempty"`
}

func (r RPInformationSendRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !studioID(r.SessionID) || !studioID(r.MessageID) || !studioID(r.RecipientEntityID) ||
		strings.TrimSpace(r.Text) == "" || len(r.Text) > 2000 {
		return core.NewError(core.CodeInvalidArgument, "bounded direct message required")
	}
	return nil
}

type RPInformationFact struct {
	Version                      string `json:"version"`
	MessageID                    string `json:"message_id"`
	SenderID                     string `json:"sender_id"`
	RecipientID                  string `json:"recipient_id"`
	Text                         string `json:"text"`
	Channel                      string `json:"channel"`
	Visibility                   string `json:"visibility"`
	ClaimedReliability           string `json:"claimed_reliability"`
	AllowRelay                   bool   `json:"allow_relay,omitempty"`
	ForwardedFromSendEventID     string `json:"forwarded_from_send_event_id,omitempty"`
	ForwardedFromDeliveryEventID string `json:"forwarded_from_delivery_event_id,omitempty"`
	OrganizationID               string `json:"organization_id,omitempty"`
	SourceCareerEventID          string `json:"source_career_event_id,omitempty"`
	InstitutionID                string `json:"institution_id,omitempty"`
	SourceLawEventID             string `json:"source_law_event_id,omitempty"`
	AudienceContractID           string `json:"audience_contract_id,omitempty"`
	AudienceTermsEventID         string `json:"audience_terms_event_id,omitempty"`
	SourceEventID                string `json:"source_event_id,omitempty"`
	RecipientPlaceID             string `json:"recipient_place_id,omitempty"`
	DeliverWorldTime             string `json:"deliver_world_time"`
}

type RPInformationRecord = privateFactRecord[RPInformationFact]

type rpInformationClaim struct {
	ClaimType          string `json:"claim_type"`
	SenderEntityID     string `json:"sender_entity_id"`
	MessageID          string `json:"message_id"`
	Text               string `json:"text"`
	Channel            string `json:"channel"`
	ClaimedReliability string `json:"claimed_reliability"`
	MayRelay           bool   `json:"may_relay,omitempty"`
	Forwarded          bool   `json:"forwarded,omitempty"`
}

func rpInformationClaimJSON(sent RPInformationFact) (string, error) {
	encoded, err := core.CanonicalJSON(rpInformationClaim{ClaimType: "message_received", SenderEntityID: sent.SenderID,
		MessageID: sent.MessageID, Text: sent.Text, Channel: sent.Channel,
		ClaimedReliability: sent.ClaimedReliability, MayRelay: sent.AllowRelay,
		Forwarded: sent.ForwardedFromSendEventID != ""})
	return string(encoded), err
}

func rpInformationDeliveryItem(sendEventID, dueText string) (SchedulerItem, scheduledPayload, error) {
	due, err := time.Parse(time.RFC3339Nano, dueText)
	if err != nil {
		return SchedulerItem{}, scheduledPayload{}, core.NewError(core.CodeProjectionDiverged, "invalid direct message due time")
	}
	base := time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)
	day := int(due.Sub(base) / (24 * time.Hour))
	if day < 0 {
		return SchedulerItem{}, scheduledPayload{}, core.NewError(core.CodeProjectionDiverged, "message delivery predates RP epoch")
	}
	payload := scheduledPayload{Kind: rpInformationDeliverKind, Day: day, SubjectID: sendEventID}
	encoded, err := core.CanonicalJSON(payload)
	if err != nil {
		return SchedulerItem{}, scheduledPayload{}, err
	}
	return SchedulerItem{SchedulerItemID: "sched_rp_information_" + sendEventID, WorldTime: dueText,
		PhaseID: rpInformationPhase, DeclaredPriority: 0, Status: "pending", Payload: string(encoded)}, payload, nil
}

// Sending is an actor action; it does not teach the remote recipient. Only a
// later sourced scheduler delivery creates their observation and Knowledge.
func (s *Store) SendRPInformation(ctx context.Context, r RPInformationSendRequest) (RPInformationRecord, error) {
	if err := r.Validate(); err != nil {
		return RPInformationRecord{}, err
	}
	var session RPSession
	return executePrivateFactCommand(s, ctx, r.Binding, "SendRPInformation", r,
		privateFactDomain{"rp_information", "RPInformationSent", `{"authorization":"rp-session-known-contact-v1"}`},
		func(conn *sql.Conn) error {
			var err error
			session, err = loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return err
			}
			if session.InstanceID != r.Binding.InstanceID || session.BranchID != r.Binding.BranchID || session.Status != "active" {
				return core.NewError(core.CodeNotFound, "current RP sender session required")
			}
			if err := requireCurrentRPSession(ctx, conn, session); err != nil {
				return err
			}
			return authorizeRPControl(ctx, conn, r.Binding.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID)
		},
		func(conn *sql.Conn, c privateFactContext) (RPInformationFact, func() error, error) {
			if session.ObservationCursor != r.Binding.ExpectedHead {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "observe current world before sending")
			}
			if err := requireRPInformationSendWindow(ctx, conn, r); err != nil {
				return RPInformationFact{}, nil, err
			}
			if err := validateRPBinding(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
				return RPInformationFact{}, nil, err
			}
			recipient, err := rpResolvePublicEntityID(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, r.RecipientEntityID)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			known, err := rpIdentityKnown(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, recipient)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			if !known || recipient == session.ControlledEntityID {
				return RPInformationFact{}, nil, core.NewError(core.CodeNotFound, "known distinct message recipient required")
			}
			if err := validateRPBinding(ctx, conn, session.InstanceID, session.BranchID, recipient); err != nil {
				return RPInformationFact{}, nil, core.NewError(core.CodeNotFound, "recipient is unavailable in this world")
			}
			var duplicate int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`, session.InstanceID, session.BranchID, r.MessageID).Scan(&duplicate); err != nil {
				return RPInformationFact{}, nil, err
			}
			if duplicate != 0 {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "message ID already used")
			}
			at, err := time.Parse(time.RFC3339Nano, c.WorldTime)
			if err != nil {
				return RPInformationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "invalid message world time")
			}
			due := at.Add(time.Minute).UTC().Format(time.RFC3339Nano)
			item, _, err := rpInformationDeliveryItem(c.EventID, due)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			fact := RPInformationFact{Version: "corerp.information.v1", MessageID: r.MessageID,
				SenderID: session.ControlledEntityID, RecipientID: recipient, Text: r.Text,
				Channel: "direct_message", Visibility: "private", ClaimedReliability: "unverified",
				AllowRelay: r.AllowRelay, DeliverWorldTime: due}
			return fact, func() error {
				rulesHash, err := core.HashJSON("rp-information-delivery-v1")
				if err != nil {
					return err
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO scheduler_phases(phase_id,description,ruleset_hash) VALUES (?,'Private in-world information delivery',?) ON CONFLICT(phase_id) DO NOTHING`, rpInformationPhase, rulesHash); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "queue direct message delivery", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,'pending',?)`, item.SchedulerItemID, session.InstanceID, session.BranchID, item.WorldTime, item.PhaseID, item.DeclaredPriority, item.Payload)
			}, nil
		})
}

func (s *Store) executeRPInformationDelivery(ctx context.Context, tx *immediateTx, item SchedulerItem) error {
	instanceID, branchID, err := recordedSchedulerScope(ctx, tx.conn, item)
	if err != nil {
		return err
	}
	if item.PhaseID != rpInformationPhase {
		return core.NewError(core.CodeProjectionDiverged, "information delivery phase differs")
	}
	var payload scheduledPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return core.NewError(core.CodeProjectionDiverged, "invalid information delivery queue")
	}
	if payload.Kind != rpInformationDeliverKind || item.SchedulerItemID != "sched_rp_information_"+payload.SubjectID {
		return core.NewError(core.CodeProjectionDiverged, "information delivery queue source differs")
	}
	var raw string
	if err := tx.conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInformationSent'`, payload.SubjectID, instanceID, branchID).Scan(&raw); err != nil {
		return classifyMissing(err, "sourced direct message")
	}
	var sent RPInformationFact
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		return core.NewError(core.CodeProjectionDiverged, "invalid information send source")
	}
	expectedItem, expectedPayload, err := rpInformationDeliveryItem(payload.SubjectID, sent.DeliverWorldTime)
	if err != nil || sent.Version != "corerp.information.v1" || !validRPInformationChannel(sent) ||
		sent.Visibility != "private" || sent.ClaimedReliability != "unverified" || sent.SenderID == "" ||
		sent.RecipientID == "" || sent.SenderID == sent.RecipientID || sent.Text == "" ||
		item != expectedItem || payload != expectedPayload {
		return core.NewError(core.CodeProjectionDiverged, "information delivery differs from send source")
	}
	var place string
	if err := tx.conn.QueryRowContext(ctx, `SELECT p.place_id FROM agent_positions p JOIN agent_profiles a ON a.agent_id=p.agent_id AND a.instance_id=? AND a.branch_id=? WHERE p.agent_id=?`, instanceID, branchID, sent.RecipientID).Scan(&place); err != nil {
		return classifyMissing(err, "direct message recipient location")
	}
	delivered := sent
	delivered.SourceEventID = payload.SubjectID
	delivered.RecipientPlaceID = place
	claimJSON, err := rpInformationClaimJSON(sent)
	if err != nil {
		return err
	}
	mutation := scheduledMutation{Private: true, EventType: "RPInformationDelivered", EventPayload: delivered,
		ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
			observationID := "observation_" + eventID + "_" + sent.RecipientID
			claimKey := "information:" + payload.SubjectID
			if err := execAgentOne(ctx, conn, "information observation", `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,?,?,?,?)`, observationID, eventID, sent.RecipientID, sent.SenderID, place, sent.Channel, item.WorldTime, claimKey, claimJSON); err != nil {
				return err
			}
			return execAgentOne(ctx, conn, "direct message knowledge", `INSERT INTO agent_knowledge(observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,0,?)`, sent.RecipientID, claimKey, sent.SenderID, place, eventID, observationID, item.WorldTime, claimJSON, sequence)
		}}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, instanceID, branchID); err != nil {
		return err
	}
	if s.beforeCommit != nil {
		return s.beforeCommit()
	}
	return nil
}
