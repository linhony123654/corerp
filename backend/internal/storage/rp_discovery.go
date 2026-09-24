package storage

import (
	"context"
	"strings"

	"corerp.local/backend/internal/core"
)

type RPBindingKey struct {
	InstanceID string `json:"instance_id"`
	BranchID   string `json:"branch_id"`
	EntityID   string `json:"entity_id"`
}

type RPDiscoverRequest struct {
	PrincipalID string        `json:"principal_id"`
	After       *RPBindingKey `json:"after,omitempty"`
	Limit       int           `json:"limit,omitempty"`
}

type RPAvailableBinding struct {
	InstanceID  string `json:"instance_id"`
	BranchID    string `json:"branch_id"`
	EntityID    string `json:"entity_id"`
	DisplayName string `json:"display_name"`
}

type RPDiscovery struct {
	ProtocolVersion string               `json:"protocol_version"`
	Bindings        []RPAvailableBinding `json:"bindings"`
	NextAfter       *RPBindingKey        `json:"next_after,omitempty"`
}

// DiscoverRPBindings reveals only the caller's usable control bindings, not
// creator world inventories or NPC private facts. A listed binding is a hint:
// OpenRPSession rechecks current authorization and validity before accepting it.
func (s *Store) DiscoverRPBindings(ctx context.Context, r RPDiscoverRequest) (RPDiscovery, error) {
	var out RPDiscovery
	if strings.TrimSpace(r.PrincipalID) == "" || r.Limit < 0 || r.Limit > 50 {
		return out, core.NewError(core.CodeInvalidArgument, "principal and discovery limit 1–50 required")
	}
	after := RPBindingKey{}
	if r.After != nil {
		after = *r.After
		for _, v := range []string{after.InstanceID, after.BranchID, after.EntityID} {
			if strings.TrimSpace(v) == "" || len(v) > 256 {
				return out, core.NewError(core.CodeInvalidArgument, "discovery continuation requires bounded binding IDs")
			}
		}
	}
	if r.Limit == 0 {
		r.Limit = 20
	}
	// Matches authorizeRPControl's exact-one active grant and validateRPBinding's
	// active individual/spatial/source-world constraints. No wildcard control.
	rows, err := s.db.QueryContext(ctx, `SELECT g.instance_id,g.branch_id,g.subject_id,e.display_name
 FROM capability_grants g JOIN principals caller ON caller.principal_id=g.principal_id
 JOIN world_instances w ON w.instance_id=g.instance_id
 JOIN materialized_entities e ON e.entity_id=g.subject_id
 JOIN cohorts source ON source.cohort_id=e.source_cohort_id
 JOIN agent_profiles a ON a.agent_id=e.entity_id
 JOIN agent_positions position ON position.agent_id=a.agent_id
 JOIN branches b ON b.instance_id=g.instance_id AND b.branch_id=g.branch_id
 WHERE g.principal_id=? AND caller.principal_type='player' AND caller.status='active'
 AND g.capability_id='world.rp.control' AND g.status='active'
 AND e.status='active' AND e.population_count=1 AND a.status='active'
 AND source.instance_id=g.instance_id AND source.branch_id=g.branch_id
 AND a.instance_id=source.instance_id AND a.branch_id=source.branch_id
 AND `+studioControlPredicate+`
 AND (g.instance_id,g.branch_id,g.subject_id)>(?,?,?)
 GROUP BY g.instance_id,g.branch_id,g.subject_id,e.display_name HAVING COUNT(*)=1
 ORDER BY g.instance_id,g.branch_id,g.subject_id LIMIT ?`, r.PrincipalID, after.InstanceID, after.BranchID, after.EntityID, r.Limit+1)
	if err != nil {
		return out, core.WrapError(core.CodeStorageFailure, "discover RP bindings", err)
	}
	defer rows.Close()
	out = RPDiscovery{ProtocolVersion: RPClientProtocolVersion, Bindings: []RPAvailableBinding{}}
	for rows.Next() {
		var item RPAvailableBinding
		if err := rows.Scan(&item.InstanceID, &item.BranchID, &item.EntityID, &item.DisplayName); err != nil {
			return RPDiscovery{}, err
		}
		out.Bindings = append(out.Bindings, item)
	}
	if err := rows.Err(); err != nil {
		return RPDiscovery{}, err
	}
	if len(out.Bindings) > r.Limit {
		out.Bindings = out.Bindings[:r.Limit]
		last := out.Bindings[len(out.Bindings)-1]
		out.NextAfter = &RPBindingKey{InstanceID: last.InstanceID, BranchID: last.BranchID, EntityID: last.EntityID}
	}
	return out, nil
}
