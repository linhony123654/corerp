package storage

import (
	"context"
	"encoding/json"

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
	PlaceID          string `json:"place_id"`
	LearnedWorldTime string `json:"learned_world_time"`
	SourceEventID    string `json:"source_event_id"`
	Text             string `json:"text,omitempty"`
	Action           string `json:"action,omitempty"`
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
	result = RPClientContext{ProtocolVersion: RPClientProtocolVersion, SessionID: session.SessionID, InstanceID: session.InstanceID, BranchID: session.BranchID, ObserverEntityID: session.ControlledEntityID, SubjectEntityID: r.SubjectEntityID, Facts: []RPContextFact{}}
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.current_world_time,b.head_sequence FROM world_clocks c JOIN branches b ON b.instance_id=c.instance_id AND b.branch_id=c.branch_id WHERE b.instance_id=? AND b.branch_id=?`, session.InstanceID, session.BranchID).Scan(&result.WorldTime, &result.ObservationCursor); err != nil {
		return result, err
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT k.subject_agent_id,k.place_id,k.learned_world_time,k.source_event_id,k.claim_payload
	 FROM agent_knowledge k JOIN events e ON e.event_id=k.source_event_id
	 WHERE k.observer_agent_id=? AND e.instance_id=? AND e.branch_id=?
	 AND e.world_time<=? AND k.learned_world_time<=? AND (?='' OR k.subject_agent_id=?)
	 AND json_extract(k.claim_payload,'$.claim_type') IN ('agent_presence','speaker_said','interpersonal_action')
	 ORDER BY e.event_sequence DESC,k.claim_key LIMIT ?`, session.ControlledEntityID, session.InstanceID, session.BranchID, result.WorldTime, result.WorldTime, r.SubjectEntityID, r.SubjectEntityID, r.Limit+1)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var fact RPContextFact
		var raw string
		if err := rows.Scan(&fact.SubjectEntityID, &fact.PlaceID, &fact.LearnedWorldTime, &fact.SourceEventID, &raw); err != nil {
			return result, err
		}
		var claim struct {
			Kind        string `json:"claim_type"`
			Text        string `json:"text"`
			Description string `json:"description"`
			Action      string `json:"action"`
		}
		if err := json.Unmarshal([]byte(raw), &claim); err != nil {
			return result, core.WrapError(core.CodeProjectionDiverged, "invalid known context evidence", err)
		}
		fact.Kind = claim.Kind
		switch claim.Kind {
		case "speaker_said":
			fact.Text = claim.Text
		case "interpersonal_action":
			fact.Text, fact.Action = claim.Description, claim.Action
		}
		result.Facts = append(result.Facts, fact)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Facts) > r.Limit {
		result.Facts = result.Facts[:r.Limit]
		result.MoreFacts = true
	}
	return result, nil
}
