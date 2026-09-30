package storage

import (
	"context"
	"encoding/json"
	"strings"

	"corerp.local/backend/internal/core"
)

const RPClientProtocolVersion = "corerp.client.v1"

type RPContextReadRequest struct {
	PrincipalID     string `json:"principal_id"`
	SessionID       string `json:"session_id"`
	SubjectEntityID string `json:"subject_entity_id,omitempty"`
	Limit           int    `json:"limit,omitempty"`
}

// A known speech is evidence that someone spoke, not that its content is true.
// Historical place/time is never a claim about the subject's current location.
type RPContextFact struct {
	Kind             string `json:"kind"`
	SubjectEntityID  string `json:"subject_entity_id"`
	TargetEntityID   string `json:"target_entity_id,omitempty"`
	PlaceID          string `json:"place_id"`
	LearnedWorldTime string `json:"learned_world_time"`
	SourceEventID    string `json:"source_event_id"`
	Text             string `json:"text,omitempty"`
	Action           string `json:"action,omitempty"`
	Channel          string `json:"channel,omitempty"`
	Reliability      string `json:"claimed_reliability,omitempty"`
	MessageID        string `json:"message_id,omitempty"`
	Stance           string `json:"stance,omitempty"`
	MayRelay         bool   `json:"may_relay,omitempty"`
	Forwarded        bool   `json:"forwarded,omitempty"`
}

type RPClientContext struct {
	ProtocolVersion   string          `json:"protocol_version"`
	SessionID         string          `json:"session_id"`
	InstanceID        string          `json:"instance_id"`
	BranchID          string          `json:"branch_id"`
	ObserverEntityID  string          `json:"observer_entity_id"`
	SubjectEntityID   string          `json:"subject_entity_id,omitempty"`
	WorldTime         string          `json:"world_time"`
	ObservationCursor int64           `json:"observation_cursor"`
	Facts             []RPContextFact `json:"facts"`
	MoreFacts         bool            `json:"more_facts"`
}

// ReadRPContext reads the controlled observer, never another character's
// DecisionInput. It deliberately does not call Observe or mutate its cursor.
func (s *Store) ReadRPContext(ctx context.Context, r RPContextReadRequest) (RPClientContext, error) {
	var result RPClientContext
	if err := (core.RPSessionReadRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID}).Validate(); err != nil {
		return result, err
	}
	if len(r.SubjectEntityID) > 256 || r.Limit < 0 || r.Limit > 50 {
		return result, core.NewError(core.CodeInvalidArgument, "context requires bounded subject and limit 1–50")
	}
	if r.Limit == 0 {
		r.Limit = 20
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return result, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if session.Status != "active" {
		return result, core.NewError(core.CodeBranchConflict, "context requires an active session")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if r.SubjectEntityID != "" && !strings.HasPrefix(r.SubjectEntityID, "person_") {
		known, err := rpIdentityKnown(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, r.SubjectEntityID)
		if err != nil {
			return result, err
		}
		if !known {
			return result, core.NewError(core.CodeNotFound, "unidentified context subject")
		}
	}
	subjectID, err := rpResolvePublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, r.SubjectEntityID)
	if err != nil {
		return result, err
	}
	publicSubjectID := ""
	if subjectID != "" {
		publicSubjectID, err = rpPublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, subjectID)
		if err != nil {
			return result, err
		}
	}
	result = RPClientContext{ProtocolVersion: RPClientProtocolVersion, SessionID: session.SessionID, InstanceID: session.InstanceID, BranchID: session.BranchID, ObserverEntityID: session.ControlledEntityID, SubjectEntityID: publicSubjectID, Facts: []RPContextFact{}}
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.current_world_time,b.head_sequence FROM world_clocks c JOIN branches b ON b.instance_id=c.instance_id AND b.branch_id=c.branch_id WHERE b.instance_id=? AND b.branch_id=?`, session.InstanceID, session.BranchID).Scan(&result.WorldTime, &result.ObservationCursor); err != nil {
		return result, err
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT k.subject_agent_id,k.place_id,k.learned_world_time,k.source_event_id,k.claim_payload
	 FROM agent_knowledge k JOIN events e ON e.event_id=k.source_event_id
	 WHERE k.observer_agent_id=? AND e.instance_id=? AND e.branch_id=?
	 AND e.world_time<=? AND k.learned_world_time<=? AND (?='' OR k.subject_agent_id=?)
	 AND json_extract(k.claim_payload,'$.claim_type') IN ('agent_presence','speaker_said','interpersonal_action','message_received','nonverbal_action','object_interaction')
	 ORDER BY e.event_sequence DESC,k.claim_key LIMIT ?`, session.ControlledEntityID, session.InstanceID, session.BranchID, result.WorldTime, result.WorldTime, subjectID, subjectID, r.Limit+1)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var fact RPContextFact
		var raw string
		if err := rows.Scan(&fact.SubjectEntityID, &fact.PlaceID, &fact.LearnedWorldTime, &fact.SourceEventID, &raw); err != nil {
			rows.Close()
			return result, err
		}
		var claim struct {
			Kind        string `json:"claim_type"`
			Text        string `json:"text"`
			Description string `json:"description"`
			Action      string `json:"action"`
			Target      string `json:"target_entity_id"`
			Channel     string `json:"channel"`
			Reliability string `json:"claimed_reliability"`
			MessageID   string `json:"message_id"`
			MayRelay    bool   `json:"may_relay"`
			Forwarded   bool   `json:"forwarded"`
		}
		if err := json.Unmarshal([]byte(raw), &claim); err != nil {
			rows.Close()
			return result, core.WrapError(core.CodeProjectionDiverged, "invalid known context evidence", err)
		}
		fact.Kind = claim.Kind
		switch claim.Kind {
		case "speaker_said":
			fact.Text = claim.Text
		case "interpersonal_action", "nonverbal_action", "object_interaction":
			fact.Text, fact.Action = claim.Description, claim.Action
			fact.TargetEntityID = claim.Target
		case "message_received":
			fact.Text, fact.Channel, fact.Reliability = claim.Text, claim.Channel, claim.Reliability
			fact.MessageID, fact.MayRelay, fact.Forwarded = claim.MessageID, claim.MayRelay, claim.Forwarded
			fact.PlaceID = "" // A remote receipt does not prove sender co-location.
		}
		result.Facts = append(result.Facts, fact)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	if len(result.Facts) > r.Limit {
		result.Facts = result.Facts[:r.Limit]
		result.MoreFacts = true
	}
	for i := range result.Facts {
		fact := &result.Facts[i]
		if fact.Kind == "message_received" {
			sendID, deliveryID, sourceErr := rpInformationRecipientSource(ctx, tx.conn, session.InstanceID,
				session.BranchID, session.ControlledEntityID, fact.MessageID, result.ObservationCursor)
			if sourceErr != nil {
				return result, sourceErr
			}
			if fact.SourceEventID != deliveryID {
				return result, core.NewError(core.CodeProjectionDiverged, "context message differs from delivery source")
			}
			fact.Stance, _, err = rpInformationCurrentStance(ctx, tx.conn, session.InstanceID,
				session.BranchID, session.ControlledEntityID, fact.MessageID, sendID, deliveryID)
			if err != nil {
				return result, err
			}
		}
		originalID := fact.SubjectEntityID
		fact.SubjectEntityID, err = rpPublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, originalID)
		if err != nil {
			return result, err
		}
		if fact.TargetEntityID != "" {
			fact.TargetEntityID, err = rpPublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, fact.TargetEntityID)
			if err != nil {
				return result, err
			}
		}
		if fact.SubjectEntityID != originalID {
			fact.SourceEventID, err = rpAnonymousEvidenceID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, fact.SourceEventID)
			if err != nil {
				return result, err
			}
		}
	}
	return result, nil
}
