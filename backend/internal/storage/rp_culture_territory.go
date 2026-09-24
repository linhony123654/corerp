package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"strings"
)

const cultureDefineCapability = "world.culture.define"

type CultureTerritoryRequest struct {
	Binding   core.CareerBinding `json:"binding"`
	ScopeKind string             `json:"scope_kind"`
	ScopeID   string             `json:"scope_id"`
	StewardID string             `json:"steward_id"`
	PlaceIDs  []string           `json:"place_ids"`
}
type CultureTerritoryFact struct {
	GrantStatus          string   `json:"grant_status,omitempty"`
	ScopeKind            string   `json:"scope_kind"`
	ScopeID              string   `json:"scope_id"`
	StewardID            string   `json:"steward_id"`
	StewardPrincipalID   string   `json:"steward_principal_id"`
	PlaceIDs             []string `json:"place_ids"`
	PlaceSourceEventIDs  []string `json:"place_source_event_ids"`
	BuilderSourceEventID string   `json:"builder_source_event_id"`
	GrantID              string   `json:"grant_id"`
}

// Explicit world-pack configuration. Only the original branch's still-active
// creator construction grant qualifies, not an agent or unrelated Cohort grant.
func (s *Store) DefineRPCultureTerritory(ctx context.Context, r CultureTerritoryRequest) (CultureRecord, error) {
	if strings.TrimSpace(r.ScopeID) == "" || len(r.ScopeID) > 256 || (r.ScopeKind != "world" && r.ScopeKind != "region") || len(r.PlaceIDs) > 64 || (r.ScopeKind == "world" && (r.ScopeID != r.Binding.InstanceID || len(r.PlaceIDs) != 0)) || (r.ScopeKind == "region" && len(r.PlaceIDs) == 0) {
		return CultureRecord{}, core.NewError(core.CodeInvalidArgument, "invalid culture territory")
	}
	var builderSource string
	authorize := func(conn *sql.Conn) error {
		var err error
		builderSource, err = authorizeCultureBuilder(ctx, conn, r.Binding)
		return err
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPCultureTerritory", r, privateFactDomain{"culture", "RPCultureFactRecorded", `{"authorization":"culture-territory-v1"}`}, authorize, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		var exists int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='territory' AND json_extract(payload,'$.territory.scope_kind')=? AND json_extract(payload,'$.territory.scope_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.ScopeKind, r.ScopeID).Scan(&exists); err != nil {
			return CultureFact{}, nil, err
		}
		if exists != 0 {
			return CultureFact{}, nil, core.NewError(core.CodeBranchConflict, "territory already defined")
		}
		if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.StewardID); err != nil {
			return CultureFact{}, nil, err
		}
		territory := &CultureTerritoryFact{ScopeKind: r.ScopeKind, ScopeID: r.ScopeID, StewardID: r.StewardID, PlaceIDs: append([]string{}, r.PlaceIDs...), PlaceSourceEventIDs: []string{}, BuilderSourceEventID: builderSource, GrantID: "grant_" + c.EventID}
		if err := conn.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=?`, r.StewardID).Scan(&territory.StewardPrincipalID); err != nil {
			return CultureFact{}, nil, err
		}
		seen := map[string]bool{}
		for _, place := range territory.PlaceIDs {
			if seen[place] {
				return CultureFact{}, nil, core.NewError(core.CodeInvalidArgument, "duplicate regional place")
			}
			seen[place] = true
			var source string
			if err := conn.QueryRowContext(ctx, `SELECT definition_event_id FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=? AND status='active'`, place, r.Binding.InstanceID, r.Binding.BranchID).Scan(&source); err != nil {
				return CultureFact{}, nil, classifyMissing(err, "actual regional place")
			}
			territory.PlaceSourceEventIDs = append(territory.PlaceSourceEventIDs, source)
		}
		fact := CultureFact{Version: "corerp.culture.v1", Kind: "territory", ActorID: r.Binding.PrincipalID, Territory: territory}
		return fact, func() error {
			if _, err := conn.ExecContext(ctx, `INSERT INTO capability_definitions(capability_id,description,policy_version) VALUES (?,'Define culture only within an explicitly installed cultural scope','culture-v1') ON CONFLICT(capability_id) DO NOTHING`, cultureDefineCapability); err != nil {
				return err
			}
			_, err := conn.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,?,?,?,?,'[]','active',?)`, territory.GrantID, territory.StewardPrincipalID, cultureDefineCapability, r.Binding.InstanceID, r.Binding.BranchID, r.ScopeKind+":"+r.ScopeID, c.EventID)
			return err
		}, nil
	})
}

func readCultureTerritory(ctx context.Context, conn *sql.Conn, b core.CareerBinding, kind, id string) (CultureTerritoryFact, string, error) {
	var raw, eventID string
	var fact CultureFact
	err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind') IN ('territory','territory_authority') AND json_extract(payload,'$.territory.scope_kind')=? AND json_extract(payload,'$.territory.scope_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, kind, id).Scan(&eventID, &raw)
	if err != nil {
		return CultureTerritoryFact{}, "", classifyMissing(err, "culture territory")
	}
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return CultureTerritoryFact{}, "", err
	}
	if fact.Territory == nil {
		return CultureTerritoryFact{}, "", core.NewError(core.CodeProjectionDiverged, "missing territory source")
	}
	return *fact.Territory, eventID, nil
}

func authorizeCultureTerritory(ctx context.Context, conn *sql.Conn, b core.CareerBinding, kind, id string) error {
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g JOIN events e ON e.event_id=g.definition_event_id AND e.instance_id=g.instance_id AND e.branch_id=g.branch_id AND e.event_type='RPCultureFactRecorded' AND json_extract(e.payload,'$.kind') IN ('territory','territory_authority') AND json_extract(e.payload,'$.territory.grant_id')=g.grant_id AND json_extract(e.payload,'$.territory.steward_principal_id')=g.principal_id AND json_extract(e.payload,'$.territory.scope_kind')||':'||json_extract(e.payload,'$.territory.scope_id')=g.subject_id WHERE g.principal_id=? AND g.instance_id=? AND g.branch_id=? AND g.capability_id=? AND g.subject_id=? AND g.status='active' AND g.field_scope='[]' AND g.amount_limit_minor IS NULL AND COALESCE(json_extract(e.payload,'$.territory.grant_status'),'active')='active' AND NOT EXISTS (SELECT 1 FROM events newer WHERE newer.instance_id=e.instance_id AND newer.branch_id=e.branch_id AND newer.event_type='RPCultureFactRecorded' AND json_extract(newer.payload,'$.kind')='territory_authority' AND json_extract(newer.payload,'$.territory.grant_id')=g.grant_id AND newer.event_sequence>e.event_sequence)`, b.PrincipalID, b.InstanceID, b.BranchID, cultureDefineCapability, kind+":"+id).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return core.NewError(core.CodeUnauthorized, "culture requires active territorial definition grant")
	}
	return nil
}
