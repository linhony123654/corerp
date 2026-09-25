package storage

import (
	"context"

	"corerp.local/backend/internal/core"
)

type RPContactsReadRequest struct {
	PrincipalID   string `json:"principal_id"`
	SessionID     string `json:"session_id"`
	AfterEntityID string `json:"after_entity_id,omitempty"`
}

type RPKnownContact struct {
	EntityID           string `json:"entity_id"`
	DisplayName        string `json:"display_name"`
	LastKnownWorldTime string `json:"last_known_world_time"`
}

type RPContacts struct {
	Contacts          []RPKnownContact `json:"contacts"`
	NextAfterEntityID string           `json:"next_after_entity_id,omitempty"`
	WorldTime         string           `json:"world_time"`
	ObservationCursor int64            `json:"observation_cursor"`
}

// ReadRPContacts is a view of the controlled character's own knowledge, never
// a directory of the population or a query for contacts' current positions.
func (s *Store) ReadRPContacts(ctx context.Context, r RPContactsReadRequest) (RPContacts, error) {
	var result RPContacts
	if err := (core.RPSessionReadRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID}).Validate(); err != nil {
		return result, err
	}
	if len(r.AfterEntityID) > 256 {
		return result, core.NewError(core.CodeInvalidArgument, "contact cursor is too long")
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
		return result, core.NewError(core.CodeBranchConflict, "contacts require an active session")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.current_world_time,b.head_sequence FROM world_clocks c JOIN branches b ON b.instance_id=c.instance_id AND b.branch_id=c.branch_id WHERE b.instance_id=? AND b.branch_id=?`, session.InstanceID, session.BranchID).Scan(&result.WorldTime, &result.ObservationCursor); err != nil {
		return result, err
	}
	rows, err := tx.conn.QueryContext(ctx, `
		SELECT k.subject_agent_id,n.display_name,MAX(k.learned_world_time)
		FROM agent_knowledge k
		JOIN rp_identity_familiarity f ON f.observer_agent_id=k.observer_agent_id AND f.subject_agent_id=k.subject_agent_id
		AND f.instance_id=? AND f.branch_id=?
		JOIN materialized_entities n ON n.entity_id=k.subject_agent_id
		JOIN agent_profiles a ON a.agent_id=n.entity_id AND a.instance_id=? AND a.branch_id=?
		JOIN events e ON e.event_id=k.source_event_id AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id
		WHERE k.observer_agent_id=? AND k.subject_agent_id<>? AND k.subject_agent_id>?
		AND json_extract(k.claim_payload,'$.claim_type') IN ('agent_presence','speaker_said')
		GROUP BY k.subject_agent_id,n.display_name ORDER BY k.subject_agent_id LIMIT 51`,
		session.InstanceID, session.BranchID, session.InstanceID, session.BranchID, session.ControlledEntityID, session.ControlledEntityID, r.AfterEntityID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Contacts = []RPKnownContact{}
	for rows.Next() {
		var contact RPKnownContact
		if err := rows.Scan(&contact.EntityID, &contact.DisplayName, &contact.LastKnownWorldTime); err != nil {
			return result, err
		}
		result.Contacts = append(result.Contacts, contact)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Contacts) > 50 {
		result.Contacts = result.Contacts[:50]
		result.NextAfterEntityID = result.Contacts[49].EntityID
	}
	return result, nil
}
