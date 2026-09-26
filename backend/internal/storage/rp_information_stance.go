package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"corerp.local/backend/internal/core"
)

type RPInformationStanceRequest struct {
	Binding   core.CareerBinding `json:"binding"`
	SessionID string             `json:"session_id"`
	MessageID string             `json:"message_id"`
	Stance    string             `json:"stance"`
	Reason    string             `json:"reason,omitempty"`
}

func (r RPInformationStanceRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !studioID(r.SessionID) || !studioID(r.MessageID) || !validRPInformationStance(r.Stance) || len(r.Reason) > 500 {
		return core.NewError(core.CodeInvalidArgument, "bounded information stance required")
	}
	return nil
}

func validRPInformationStance(value string) bool {
	return value == "believe" || value == "doubt" || value == "reject"
}

type RPInformationStanceFact struct {
	Version               string `json:"version"`
	MessageID             string `json:"message_id"`
	ObserverID            string `json:"observer_id"`
	SourceSendEventID     string `json:"source_send_event_id"`
	SourceDeliveryEventID string `json:"source_delivery_event_id"`
	PreviousStanceEventID string `json:"previous_stance_event_id,omitempty"`
	Stance                string `json:"stance"`
	Reason                string `json:"reason,omitempty"`
}

type RPInformationStanceRecord = privateFactRecord[RPInformationStanceFact]

// A stance is the recipient's interpretation of a claim, never a change to
// its source or to authoritative world truth. Corrections append Events.
func (s *Store) RecordRPInformationStance(ctx context.Context, r RPInformationStanceRequest) (RPInformationStanceRecord, error) {
	if err := r.Validate(); err != nil {
		return RPInformationStanceRecord{}, err
	}
	var session RPSession
	return executePrivateFactCommand(s, ctx, r.Binding, "RecordRPInformationStance", r,
		privateFactDomain{"rp_information_stance", "RPInformationStanceRecorded", `{"authorization":"rp-recipient-delivery-v1"}`},
		func(conn *sql.Conn) error {
			var err error
			session, err = loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return err
			}
			if session.InstanceID != r.Binding.InstanceID || session.BranchID != r.Binding.BranchID || session.Status != "active" {
				return core.NewError(core.CodeNotFound, "current RP recipient session required")
			}
			if err := requireCurrentRPSession(ctx, conn, session); err != nil {
				return err
			}
			return authorizeRPControl(ctx, conn, r.Binding.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID)
		},
		func(conn *sql.Conn, c privateFactContext) (RPInformationStanceFact, func() error, error) {
			if session.ObservationCursor != r.Binding.ExpectedHead {
				return RPInformationStanceFact{}, nil, core.NewError(core.CodeBranchConflict, "observe current world before recording stance")
			}
			if err := requireRPInformationStanceWindow(ctx, conn, r); err != nil {
				return RPInformationStanceFact{}, nil, err
			}
			sendID, deliveryID, err := rpInformationRecipientSource(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, r.MessageID, c.Sequence-1)
			if err != nil {
				return RPInformationStanceFact{}, nil, err
			}
			_, previous, err := rpInformationCurrentStance(ctx, conn, session.InstanceID, session.BranchID,
				session.ControlledEntityID, r.MessageID, sendID, deliveryID)
			if err != nil {
				return RPInformationStanceFact{}, nil, err
			}
			fact := RPInformationStanceFact{Version: "corerp.information.stance.v1", MessageID: r.MessageID,
				ObserverID: session.ControlledEntityID, SourceSendEventID: sendID,
				SourceDeliveryEventID: deliveryID, PreviousStanceEventID: previous,
				Stance: r.Stance, Reason: strings.TrimSpace(r.Reason)}
			return fact, nil, nil
		})
}

// Resolve the client-safe message ID to this observer's exact delivered
// observation. Both the mutable Knowledge row and Observation must still
// agree with the send/delivery Events; a guessed message ID teaches nothing.
func rpInformationRecipientSource(ctx context.Context, conn *sql.Conn, instanceID, branchID, observerID, messageID string, head int64) (string, string, error) {
	var sendID string
	err := conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=? AND event_sequence<=?`,
		instanceID, branchID, messageID, head).Scan(&sendID)
	if err == sql.ErrNoRows {
		return "", "", core.NewError(core.CodeNotFound, "delivered message not known to this recipient")
	}
	if err != nil {
		return "", "", err
	}
	_, observations, err := rpInformationExpected(ctx, conn, instanceID, branchID, head)
	if err != nil {
		return "", "", err
	}
	var expected rpInformationObservation
	for _, observation := range observations {
		if observation.ClaimKey == "information:"+sendID && observation.ObserverID == observerID {
			expected = observation
			break
		}
	}
	if expected.ID == "" {
		return "", "", core.NewError(core.CodeNotFound, "delivered message not known to this recipient")
	}
	var actual rpInformationObservation
	err = conn.QueryRowContext(ctx, `SELECT observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload FROM observation_records WHERE observation_id=?`, expected.ID).Scan(
		&actual.ID, &actual.SourceEventID, &actual.ObserverID, &actual.SubjectID, &actual.PlaceID,
		&actual.Channel, &actual.WorldTime, &actual.ClaimKey, &actual.ClaimJSON)
	if err != nil || actual != expected {
		return "", "", core.NewError(core.CodeProjectionDiverged, "information recipient observation differs from source")
	}
	var subjectID, placeID, sourceID, observationID, learnedAt, claimJSON string
	var sequence int64
	err = conn.QueryRowContext(ctx, `SELECT subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,last_event_sequence FROM agent_knowledge WHERE observer_agent_id=? AND claim_key=?`,
		observerID, expected.ClaimKey).Scan(&subjectID, &placeID, &sourceID, &observationID, &learnedAt, &claimJSON, &sequence)
	if err != nil {
		return "", "", core.NewError(core.CodeProjectionDiverged, "information recipient knowledge differs from source")
	}
	var deliverySequence int64
	if err := conn.QueryRowContext(ctx, `SELECT event_sequence FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInformationDelivered'`, expected.SourceEventID, instanceID, branchID).Scan(&deliverySequence); err != nil {
		return "", "", core.NewError(core.CodeProjectionDiverged, "information delivery source missing")
	}
	if subjectID != expected.SubjectID || placeID != expected.PlaceID || sourceID != expected.SourceEventID ||
		observationID != expected.ID || learnedAt != expected.WorldTime || claimJSON != expected.ClaimJSON || sequence != deliverySequence {
		return "", "", core.NewError(core.CodeProjectionDiverged, "information recipient knowledge differs from source")
	}
	return sendID, expected.SourceEventID, nil
}

// Read a source-only correction chain, oldest first. No mutable belief table
// can erase a prior interpretation or manufacture a current stance.
func rpInformationCurrentStance(ctx context.Context, q replayQuerier, instanceID, branchID, observerID, messageID, sendID, deliveryID string) (string, string, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationStanceRecorded' AND json_extract(payload,'$.source_send_event_id')=? AND json_extract(payload,'$.observer_id')=? ORDER BY event_sequence`,
		instanceID, branchID, sendID, observerID)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	lastID, stance := "", ""
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return "", "", err
		}
		var fact RPInformationStanceFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Version != "corerp.information.stance.v1" ||
			fact.MessageID != messageID || fact.ObserverID != observerID || fact.SourceSendEventID != sendID ||
			fact.SourceDeliveryEventID != deliveryID || fact.PreviousStanceEventID != lastID ||
			!validRPInformationStance(fact.Stance) || len(fact.Reason) > 500 {
			return "", "", core.NewError(core.CodeProjectionDiverged, "information stance source chain differs")
		}
		lastID, stance = id, fact.Stance
	}
	if err := rows.Err(); err != nil {
		return "", "", err
	}
	return stance, lastID, nil
}
