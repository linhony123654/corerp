package storage

import (
	"context"
	"database/sql"
	"strings"

	"corerp.local/backend/internal/core"
)

const studioGrantPredicate = `p.status='active' AND g.status='active'
 AND ((p.principal_type='creator' AND g.capability_id='world.inspector.read')
 OR (p.principal_type='operator' AND g.capability_id='diagnostics.inspector.read'))
 AND EXISTS (SELECT 1 FROM json_each(g.field_scope) WHERE value='event')
 AND EXISTS (SELECT 1 FROM json_each(g.field_scope) WHERE value='rule')`

func authorizeStudio(ctx context.Context, tx *sql.Tx, principal, instance, branch string) (string, error) {
	var role string
	err := tx.QueryRowContext(ctx, `SELECT p.principal_type FROM principals p JOIN capability_grants g ON g.principal_id=p.principal_id
 WHERE p.principal_id=? AND g.instance_id=? AND g.branch_id=? AND g.subject_id IN (?, '*') AND `+studioGrantPredicate+` LIMIT 1`, principal, instance, branch, branch).Scan(&role)
	if err == sql.ErrNoRows {
		return "", core.NewError(core.CodeUnauthorized, "principal lacks scoped inspector permission")
	}
	if err != nil {
		return "", core.WrapError(core.CodeStorageFailure, "authorize inspector", err)
	}
	return role, nil
}

type StudioScopeKey struct {
	InstanceID string `json:"instance_id"`
	BranchID   string `json:"branch_id"`
}
type StudioScopeRequest struct {
	PrincipalID string          `json:"principal_id"`
	After       *StudioScopeKey `json:"after,omitempty"`
	Limit       int             `json:"limit,omitempty"`
}
type StudioScope struct {
	InstanceID   string `json:"instance_id"`
	BranchID     string `json:"branch_id"`
	Label        string `json:"label"`
	HeadSequence int64  `json:"head_sequence"`
	AccessLevel  string `json:"access_level"`
}
type StudioScopes struct {
	Scopes    []StudioScope   `json:"scopes"`
	NextAfter *StudioScopeKey `json:"next_after,omitempty"`
}

func studioID(v string) bool { return strings.TrimSpace(v) != "" && len(v) <= 256 }
func studioLimit(n int) (int, error) {
	if n < 0 || n > 50 {
		return 0, core.NewError(core.CodeInvalidArgument, "inspector page limit must be 1–50")
	}
	if n == 0 {
		n = 20
	}
	return n, nil
}

func (s *Store) ListStudioScopes(ctx context.Context, r StudioScopeRequest) (StudioScopes, error) {
	out := StudioScopes{Scopes: []StudioScope{}}
	limit, err := studioLimit(r.Limit)
	if err != nil {
		return out, err
	}
	if !studioID(r.PrincipalID) || (r.After != nil && (!studioID(r.After.InstanceID) || !studioID(r.After.BranchID))) {
		return out, core.NewError(core.CodeInvalidArgument, "invalid inspector scope key")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active' AND principal_type IN ('creator','operator')`, r.PrincipalID).Scan(&role); err != nil {
		if err == sql.ErrNoRows {
			return out, core.NewError(core.CodeUnauthorized, "active creator or operator required")
		}
		return out, err
	}
	after := StudioScopeKey{}
	if r.After != nil {
		after = *r.After
	}
	rows, err := tx.QueryContext(ctx, `SELECT b.instance_id,b.branch_id,b.label,b.head_sequence FROM branches b
 WHERE (b.instance_id,b.branch_id)>(?,?) AND EXISTS (SELECT 1 FROM capability_grants g JOIN principals p ON p.principal_id=g.principal_id
 WHERE p.principal_id=? AND g.instance_id=b.instance_id AND g.branch_id=b.branch_id AND g.subject_id IN (b.branch_id,'*') AND `+studioGrantPredicate+`)
 ORDER BY b.instance_id,b.branch_id LIMIT ?`, after.InstanceID, after.BranchID, r.PrincipalID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var scope StudioScope
		scope.AccessLevel = role
		if err := rows.Scan(&scope.InstanceID, &scope.BranchID, &scope.Label, &scope.HeadSequence); err != nil {
			return StudioScopes{}, err
		}
		if len(out.Scopes) == limit {
			last := out.Scopes[len(out.Scopes)-1]
			out.NextAfter = &StudioScopeKey{last.InstanceID, last.BranchID}
			break
		}
		out.Scopes = append(out.Scopes, scope)
	}
	return out, rows.Err()
}

type StudioTimelineRequest struct {
	PrincipalID     string `json:"principal_id"`
	InstanceID      string `json:"instance_id"`
	BranchID        string `json:"branch_id"`
	ThroughSequence int64  `json:"through_sequence,omitempty"`
	BeforeSequence  int64  `json:"before_sequence,omitempty"`
	Limit           int    `json:"limit,omitempty"`
}
type StudioTimelineEvent struct {
	EventID   string `json:"event_id"`
	Sequence  int64  `json:"event_sequence"`
	EventType string `json:"event_type"`
	WorldTime string `json:"world_time"`
}
type StudioTimeline struct {
	InstanceID         string                `json:"instance_id"`
	BranchID           string                `json:"branch_id"`
	AccessLevel        string                `json:"access_level"`
	ThroughSequence    int64                 `json:"through_sequence"`
	NextBeforeSequence int64                 `json:"next_before_sequence,omitempty"`
	Events             []StudioTimelineEvent `json:"events"`
}

func (s *Store) ListStudioEvents(ctx context.Context, r StudioTimelineRequest) (StudioTimeline, error) {
	var out StudioTimeline
	limit, err := studioLimit(r.Limit)
	if err != nil {
		return out, err
	}
	if !studioID(r.PrincipalID) || !studioID(r.InstanceID) || !studioID(r.BranchID) || r.ThroughSequence < 0 || r.BeforeSequence < 0 || r.ThroughSequence >= core.MaxJSONSafeInteger || r.BeforeSequence >= core.MaxJSONSafeInteger {
		return out, core.NewError(core.CodeInvalidArgument, "invalid inspector timeline bounds")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	role, err := authorizeStudio(ctx, tx, r.PrincipalID, r.InstanceID, r.BranchID)
	if err != nil {
		return out, err
	}
	var head int64
	if err := tx.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, r.InstanceID, r.BranchID).Scan(&head); err != nil {
		return out, classifyMissing(err, "inspector branch")
	}
	if r.ThroughSequence == 0 {
		r.ThroughSequence = head
	}
	if r.ThroughSequence > head {
		return out, core.NewError(core.CodeInvalidArgument, "timeline anchor exceeds branch head")
	}
	if r.BeforeSequence == 0 {
		r.BeforeSequence = r.ThroughSequence + 1
	}
	if r.BeforeSequence > r.ThroughSequence+1 {
		return out, core.NewError(core.CodeInvalidArgument, "timeline cursor exceeds anchor")
	}
	out = StudioTimeline{InstanceID: r.InstanceID, BranchID: r.BranchID, AccessLevel: role, ThroughSequence: r.ThroughSequence, Events: []StudioTimelineEvent{}}
	rows, err := tx.QueryContext(ctx, `SELECT event_id,event_sequence,event_type,world_time FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_sequence<? ORDER BY event_sequence DESC LIMIT ?`, r.InstanceID, r.BranchID, r.ThroughSequence, r.BeforeSequence, limit+1)
	if err != nil {
		return StudioTimeline{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var e StudioTimelineEvent
		if err := rows.Scan(&e.EventID, &e.Sequence, &e.EventType, &e.WorldTime); err != nil {
			return StudioTimeline{}, err
		}
		if len(out.Events) == limit {
			out.NextBeforeSequence = out.Events[len(out.Events)-1].Sequence
			break
		}
		out.Events = append(out.Events, e)
	}
	return out, rows.Err()
}
