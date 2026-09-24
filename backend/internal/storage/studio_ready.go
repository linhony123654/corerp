package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type StudioWorldSaveRequest struct {
	Genesis           StudioGenesisRequest `json:"genesis"`
	Binding           core.CareerBinding   `json:"binding"`
	PlayerPrincipalID string               `json:"player_principal_id"`
}
type StudioWorldReadyFact struct {
	Version           string `json:"version"`
	GenesisEventID    string `json:"genesis_event_id"`
	ActivationEventID string `json:"activation_event_id"`
	PlayerPrincipalID string `json:"player_principal_id"`
	EntityID          string `json:"entity_id"`
	GrantID           string `json:"grant_id"`
}

// SaveStudioWorld is a local readiness transition, not external publication.
// It binds an existing player identity; no credential is created or disclosed.
func (s *Store) SaveStudioWorld(ctx context.Context, r StudioWorldSaveRequest) (privateFactRecord[StudioWorldReadyFact], error) {
	var empty privateFactRecord[StudioWorldReadyFact]
	if err := validateStudioGenesisRequest(r.Genesis); err != nil {
		return empty, err
	}
	if !studioID(r.PlayerPrincipalID) {
		return empty, core.NewError(core.CodeInvalidArgument, "existing bounded player identity required")
	}
	if r.Binding.PrincipalID != r.Genesis.PrincipalID || r.Binding.InstanceID != r.Genesis.InstanceID || r.Binding.BranchID != "br_main" {
		return empty, core.NewError(core.CodeUnauthorized, "save binding differs")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "SaveStudioWorld", r, privateFactDomain{"studio_ready", "StudioWorldReady", `{"authorization":"sourced-world-create"}`},
		func(conn *sql.Conn) error { return authorizeSavedStudioGenesis(ctx, conn, r.Genesis) },
		func(conn *sql.Conn, c privateFactContext) (StudioWorldReadyFact, func() error, error) {
			var fact StudioWorldReadyFact
			packages, err := readStudioActivePackages(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID)
			if err != nil {
				return fact, nil, err
			}
			if packages == nil {
				return fact, nil, core.NewError(core.CodeBranchConflict, "Studio packages must be active before saving")
			}
			var count int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM principals WHERE principal_id=? AND principal_type='player' AND status='active'`, r.PlayerPrincipalID).Scan(&count); err != nil {
				return fact, nil, err
			}
			if count != 1 {
				return fact, nil, core.NewError(core.CodeUnauthorized, "save requires an existing active player, not creator/ops")
			}
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_instances w JOIN world_clocks clock ON clock.instance_id=w.instance_id AND clock.branch_id=? WHERE w.instance_id=? AND w.lifecycle_state='paused' AND clock.status='paused' AND clock.current_world_time=? AND NOT EXISTS(SELECT 1 FROM events e WHERE e.instance_id=w.instance_id AND e.branch_id=clock.branch_id AND e.event_type='StudioWorldReady')`, r.Binding.BranchID, r.Binding.InstanceID, r.Genesis.Spec.StartWorldTime).Scan(&count); err != nil {
				return fact, nil, err
			}
			if count != 1 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "world must be initially prepared and not already saved")
			}
			id := func(kind, key string) string {
				v, _ := core.StudioWorldObjectID(r.Binding.InstanceID, kind, key)
				return v
			}
			for _, person := range r.Genesis.Spec.People {
				if person.Player {
					fact.EntityID = id("entity", person.Key)
				}
			}
			if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, fact.EntityID); err != nil {
				return fact, nil, err
			}
			fact.Version = "corerp.studio-ready.v1"
			fact.GenesisEventID = id("event", "genesis")
			fact.ActivationEventID = packages.Lock.ActivationEventID
			fact.PlayerPrincipalID = r.PlayerPrincipalID
			fact.GrantID = id("grant", "player-control")
			return fact, func() error {
				if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO capability_definitions(capability_id,description,policy_version) VALUES ('world.rp.control','Control an existing entity for RP','rp1-v1')`); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "Studio player control", `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,'world.rp.control',?,?,?,'[]','active',?)`, fact.GrantID, fact.PlayerPrincipalID, r.Binding.InstanceID, r.Binding.BranchID, fact.EntityID, c.EventID); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "Studio world ready", `UPDATE world_instances SET lifecycle_state='active' WHERE instance_id=? AND lifecycle_state='paused'`, r.Binding.InstanceID); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "Studio ready clock", `UPDATE world_clocks SET status='ready' WHERE instance_id=? AND branch_id=? AND status='paused'`, r.Binding.InstanceID, r.Binding.BranchID)
			}, nil
		})
}

// Shared SQL predicate used by control authorization and binding discovery.
// g=capability_grants, w=world_instances. Legacy world grants are unchanged.
const studioControlPredicate = `(w.world_definition_id<>'corerp.studio.world' OR
 (w.lifecycle_state='active' AND g.field_scope='[]' AND g.amount_limit_minor IS NULL AND EXISTS(
 SELECT 1 FROM events ready WHERE ready.event_id=g.definition_event_id AND ready.instance_id=g.instance_id AND ready.branch_id=g.branch_id
 AND ready.event_type='StudioWorldReady' AND json_extract(ready.payload,'$.version')='corerp.studio-ready.v1'
 AND json_extract(ready.payload,'$.grant_id')=g.grant_id AND json_extract(ready.payload,'$.player_principal_id')=g.principal_id
 AND json_extract(ready.payload,'$.entity_id')=g.subject_id)))`

func requireStudioWriteRules(ctx context.Context, conn *sql.Conn, instance, branch string) error {
	// Genesis/installation have no active packages yet. Once saved, every
	// chronology-checked external command and scheduled mutation must retain
	// the exact active source. Read-only replay is intentionally independent.
	var saved int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='StudioWorldReady'`, instance, branch).Scan(&saved); err != nil {
		return err
	}
	if saved == 0 {
		return nil
	}
	_, err := readStudioActivePackages(ctx, conn, instance, branch)
	return err
}
