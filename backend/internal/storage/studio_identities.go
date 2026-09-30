package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// RPIdentitiesDeclared is the generic form of the demo's hardcoded acquaintance
// seed: a world declares mutual-recognition pairs at creation, the identity
// projection derives them from this event, and the apply callback writes the
// same rows a replay would expect. The M2 demo's RPParticipantsInitialized
// special case remains for already-committed demo worlds.
type RPIdentitiesDeclaredFact struct {
	Version       string                       `json:"version"`
	Pairs         [][2]string                  `json:"pairs"`
	Relationships []RPIdentityRelationshipFact `json:"relationships,omitempty"`
}

type RPIdentityRelationshipFact struct {
	ActorEntityID   string   `json:"actor_entity_id"`
	SubjectEntityID string   `json:"subject_entity_id"`
	Role            string   `json:"role"`
	AddressTo       []string `json:"address_to,omitempty"`
	SelfReference   string   `json:"self_reference,omitempty"`
}

func (s *Store) PrepareStudioIdentities(ctx context.Context, r StudioGenesisRequest) (privateFactRecord[RPIdentitiesDeclaredFact], error) {
	var empty privateFactRecord[RPIdentitiesDeclaredFact]
	if err := validateStudioGenesisRequest(r); err != nil {
		return empty, err
	}
	id := func(kind, key string) string { v, _ := core.StudioWorldObjectID(r.InstanceID, kind, key); return v }
	binding := core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: r.InstanceID, BranchID: "br_main", ExpectedHead: int64(2 + len(r.Spec.People)), IdempotencyKey: id("key", "identities")}
	return executePrivateFactCommand(s, ctx, binding, "PrepareStudioIdentities", r,
		privateFactDomain{"studio_identities", "RPIdentitiesDeclared", `{"authorization":"sourced-world-create"}`},
		func(conn *sql.Conn) error { return authorizeSavedStudioGenesis(ctx, conn, r) },
		func(conn *sql.Conn, c privateFactContext) (RPIdentitiesDeclaredFact, func() error, error) {
			fact := RPIdentitiesDeclaredFact{Version: "corerp.studio-identities.v1"}
			var ready int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_instances w JOIN world_clocks cl ON cl.instance_id=w.instance_id AND cl.branch_id='br_main' WHERE w.instance_id=? AND w.lifecycle_state='paused' AND cl.status='paused' AND cl.current_world_time=?`, r.InstanceID, r.Spec.StartWorldTime).Scan(&ready); err != nil {
				return fact, nil, err
			}
			if ready != 1 || c.WorldTime != r.Spec.StartWorldTime {
				return fact, nil, core.NewError(core.CodeBranchConflict, "identity declaration requires paused prepared world")
			}
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id='br_main' AND event_type='StudioSpatialPrepared'`, r.InstanceID).Scan(&ready); err != nil {
				return fact, nil, err
			}
			if ready != 1 {
				return fact, nil, core.NewError(core.CodeProjectionDiverged, "identity declaration requires prepared spatial actors")
			}
			for _, pair := range r.Spec.Acquaintances {
				fact.Pairs = append(fact.Pairs, [2]string{id("entity", pair[0]), id("entity", pair[1])})
			}
			for _, relation := range r.Spec.Relationships {
				fact.Relationships = append(fact.Relationships, RPIdentityRelationshipFact{
					ActorEntityID: id("entity", relation.From), SubjectEntityID: id("entity", relation.To),
					Role: relation.Role, AddressTo: relation.AddressTo, SelfReference: relation.SelfReference,
				})
			}
			return fact, func() error {
				for _, pair := range fact.Pairs {
					for _, direction := range [][2]string{pair, {pair[1], pair[0]}} {
						if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO rp_identity_familiarity(observer_agent_id,subject_agent_id,instance_id,branch_id,source_event_id,learned_world_time,origin_kind) VALUES (?,?,?,?,?,?,'declared')`, direction[0], direction[1], r.InstanceID, "br_main", c.EventID, c.WorldTime); err != nil {
							return core.WrapError(core.CodeStorageFailure, "record declared identity", err)
						}
					}
				}
				return nil
			}, nil
		})
}
