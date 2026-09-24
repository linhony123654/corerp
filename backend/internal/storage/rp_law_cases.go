package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
)

type LawCasesRequest struct {
	PrincipalID string `json:"principal_id"`
	InstanceID  string `json:"instance_id"`
	BranchID    string `json:"branch_id"`
	ActorID     string `json:"actor_id"`
}

func (s *Store) ReadOwnRPLawCases(ctx context.Context, r LawCasesRequest) ([]core.RPLawCase, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	conn := tx.conn
	b := core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: r.InstanceID, BranchID: r.BranchID}
	if err := authorizeCareerCandidate(ctx, conn, b, r.ActorID); err != nil {
		return nil, err
	}
	cases, err := readOwnRPLawCases(ctx, conn, r.InstanceID, r.BranchID, r.ActorID)
	if err != nil {
		return nil, err
	}
	if cases == nil {
		cases = []core.RPLawCase{}
	}
	return cases, nil
}

// A private receipt/case view, not a list of the institution's defendants.
func readOwnRPLawCases(ctx context.Context, conn *sql.Conn, instance, branch, actor string) ([]core.RPLawCase, error) {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND (json_extract(payload,'$.enforcement.actor_id')=? OR json_extract(payload,'$.dispute.actor_id')=? OR json_extract(payload,'$.review.actor_id')=?) ORDER BY event_sequence`, instance, branch, actor, actor, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cases := map[string]core.RPLawCase{}
	var order []string
	for rows.Next() {
		var id, raw string
		var fact InstitutionFact
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return nil, err
		}
		switch fact.Kind {
		case "law_enforcement":
			e := fact.Enforcement
			if e == nil {
				return nil, core.NewError(core.CodeProjectionDiverged, "case lacks enforcement")
			}
			cases[id] = core.RPLawCase{InstitutionID: fact.InstitutionID, EnforcementEventID: id, ViolationEventID: e.ViolationEventID, EnactmentEventID: e.EnactmentEventID, FineMinor: e.FineMinor, Status: "enforced"}
			order = append(order, id)
		case "law_dispute":
			d := fact.Dispute
			if d == nil {
				return nil, core.NewError(core.CodeProjectionDiverged, "case lacks dispute")
			}
			c, ok := cases[d.EnforcementEventID]
			if !ok {
				return nil, core.NewError(core.CodeProjectionDiverged, "dispute lacks own case")
			}
			c.Status = "disputed"
			c.DisputeEventID = id
			cases[d.EnforcementEventID] = c
		case "law_review":
			r := fact.Review
			if r == nil {
				return nil, core.NewError(core.CodeProjectionDiverged, "case lacks review")
			}
			c, ok := cases[r.EnforcementEventID]
			if !ok {
				return nil, core.NewError(core.CodeProjectionDiverged, "review lacks own case")
			}
			c.Status = r.Decision
			c.ReviewEventID = id
			c.RefundedMinor = r.RefundedMinor
			cases[r.EnforcementEventID] = c
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(order) > 16 {
		order = order[len(order)-16:]
	}
	var result []core.RPLawCase
	for _, id := range order {
		result = append(result, cases[id])
	}
	return result, nil
}
