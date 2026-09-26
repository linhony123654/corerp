package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// A relay repeats the exact learned claim. It cannot promote a private
// message into public truth or edit the claim while retaining its provenance.
type RPInformationRelayRequest struct {
	Binding            core.CareerBinding `json:"binding"`
	SessionID          string             `json:"session_id"`
	MessageID          string             `json:"message_id"`
	ForwardedMessageID string             `json:"forwarded_message_id"`
	RecipientEntityID  string             `json:"recipient_entity_id"`
}

func (r RPInformationRelayRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !studioID(r.SessionID) || !studioID(r.MessageID) || !studioID(r.ForwardedMessageID) ||
		!studioID(r.RecipientEntityID) || r.MessageID == r.ForwardedMessageID {
		return core.NewError(core.CodeInvalidArgument, "bounded information relay required")
	}
	return nil
}

// A sender may explicitly allow one recipient-to-known-contact relay. The
// relay itself is private and not relayable again; the original sender's
// consent does not become a transitive broadcast permission.
func (s *Store) RelayRPInformation(ctx context.Context, r RPInformationRelayRequest) (RPInformationRecord, error) {
	if err := r.Validate(); err != nil {
		return RPInformationRecord{}, err
	}
	var session RPSession
	return executePrivateFactCommand(s, ctx, r.Binding, "RelayRPInformation", r,
		privateFactDomain{"rp_information_relay", "RPInformationSent", `{"authorization":"rp-recipient-explicit-one-hop-relay-v1"}`},
		func(conn *sql.Conn) error {
			var err error
			session, err = loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return err
			}
			if session.InstanceID != r.Binding.InstanceID || session.BranchID != r.Binding.BranchID || session.Status != "active" {
				return core.NewError(core.CodeNotFound, "current RP relay session required")
			}
			if err := requireCurrentRPSession(ctx, conn, session); err != nil {
				return err
			}
			return authorizeRPControl(ctx, conn, r.Binding.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID)
		},
		func(conn *sql.Conn, c privateFactContext) (RPInformationFact, func() error, error) {
			if session.ObservationCursor != r.Binding.ExpectedHead {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "observe current world before relaying")
			}
			if err := requireRPInformationRelayWindow(ctx, conn, r); err != nil {
				return RPInformationFact{}, nil, err
			}
			parentSendID, parentDeliveryID, err := rpInformationRecipientSource(ctx, conn, session.InstanceID, session.BranchID,
				session.ControlledEntityID, r.ForwardedMessageID, c.Sequence-1)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			var raw string
			if err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInformationSent'`,
				parentSendID, session.InstanceID, session.BranchID).Scan(&raw); err != nil {
				return RPInformationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "relay source missing")
			}
			var parent RPInformationFact
			if err := json.Unmarshal([]byte(raw), &parent); err != nil {
				return RPInformationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "relay source invalid")
			}
			if parent.Channel != "direct_message" || !parent.AllowRelay || parent.ForwardedFromSendEventID != "" || parent.RecipientID != session.ControlledEntityID {
				return RPInformationFact{}, nil, core.NewError(core.CodeUnauthorized, "private claim has no relay permission")
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
				return RPInformationFact{}, nil, core.NewError(core.CodeNotFound, "known distinct relay recipient required")
			}
			if err := validateRPBinding(ctx, conn, session.InstanceID, session.BranchID, recipient); err != nil {
				return RPInformationFact{}, nil, core.NewError(core.CodeNotFound, "relay recipient unavailable")
			}
			var duplicate int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`,
				session.InstanceID, session.BranchID, r.MessageID).Scan(&duplicate); err != nil {
				return RPInformationFact{}, nil, err
			}
			if duplicate != 0 {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "message ID already used")
			}
			at, err := time.Parse(time.RFC3339Nano, c.WorldTime)
			if err != nil {
				return RPInformationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "invalid relay world time")
			}
			due := at.Add(time.Minute).UTC().Format(time.RFC3339Nano)
			item, _, err := rpInformationDeliveryItem(c.EventID, due)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			fact := RPInformationFact{Version: "corerp.information.v1", MessageID: r.MessageID,
				SenderID: session.ControlledEntityID, RecipientID: recipient, Text: parent.Text,
				Channel: "rumor", Visibility: "private", ClaimedReliability: "unverified", DeliverWorldTime: due,
				ForwardedFromSendEventID: parentSendID, ForwardedFromDeliveryEventID: parentDeliveryID}
			return fact, func() error {
				rulesHash, err := core.HashJSON("rp-information-delivery-v1")
				if err != nil {
					return err
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO scheduler_phases(phase_id,description,ruleset_hash) VALUES (?,'Private in-world information delivery',?) ON CONFLICT(phase_id) DO NOTHING`, rpInformationPhase, rulesHash); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "queue rumor delivery", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,'pending',?)`,
					item.SchedulerItemID, session.InstanceID, session.BranchID, item.WorldTime, item.PhaseID, item.DeclaredPriority, item.Payload)
			}, nil
		})
}
