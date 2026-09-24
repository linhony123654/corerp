package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"sort"
)

type CultureAffiliationRequest struct {
	Binding             core.CareerBinding `json:"binding"`
	EntityID            string             `json:"entity_id"`
	DefinitionEventID   string             `json:"definition_event_id"`
	TransmissionEventID string             `json:"transmission_event_id,omitempty"`
	Decision            string             `json:"decision"`
}

func (s *Store) AffiliateRPCulture(ctx context.Context, r CultureAffiliationRequest) (CultureRecord, error) {
	if r.Decision != "join" && r.Decision != "leave" {
		return CultureRecord{}, core.NewError(core.CodeInvalidArgument, "affiliation requires join or leave")
	}
	return s.executeCultureCommand(ctx, r.Binding, "AffiliateRPCulture", r, r.EntityID, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		definition, err := readCultureFact(ctx, conn, r.Binding, r.DefinitionEventID)
		if err != nil {
			return CultureFact{}, nil, err
		}
		if definition.Kind != "definition" || definition.Definition == nil {
			return CultureFact{}, nil, core.NewError(core.CodeInvalidArgument, "affiliation requires supported cultural scope")
		}
		affiliations, err := readOwnCultureAffiliations(ctx, conn, r.EntityID)
		if err != nil {
			return CultureFact{}, nil, err
		}
		var previous *core.RPCultureAffiliation
		for i := range affiliations {
			if affiliations[i].CultureID == definition.Definition.CultureID {
				previous = &affiliations[i]
			}
		}
		active := previous != nil && previous.Status == "joined"
		if (r.Decision == "join" && active) || (r.Decision == "leave" && !active) {
			return CultureFact{}, nil, core.NewError(core.CodeBranchConflict, "affiliation transition does not change current membership")
		}
		affiliation := core.RPCultureAffiliation{EntityID: r.EntityID, CultureID: definition.Definition.CultureID, DefinitionEventID: r.DefinitionEventID, SourceEventID: c.EventID, Status: "joined"}
		if r.Decision == "join" && (definition.Definition.ScopeKind == "world" || definition.Definition.ScopeKind == "region") {
			territory, source, err := readCultureTerritory(ctx, conn, r.Binding, definition.Definition.ScopeKind, definition.Definition.ScopeID)
			if err != nil {
				return CultureFact{}, nil, err
			}
			affiliation.EligibilityEventID = source
			if territory.ScopeKind == "region" {
				var place, presence string
				if err := conn.QueryRowContext(ctx, `SELECT p.place_id,e.event_id FROM agent_positions p JOIN events e ON e.event_sequence=p.last_event_sequence AND e.instance_id=? AND e.branch_id=? WHERE p.agent_id=?`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID).Scan(&place, &presence); err != nil {
					return CultureFact{}, nil, err
				}
				inside := false
				for _, id := range territory.PlaceIDs {
					if id == place {
						inside = true
					}
				}
				if !inside {
					return CultureFact{}, nil, core.NewError(core.CodeUnauthorized, "regional cultural affiliation requires actual presence")
				}
				affiliation.ScopePlaceID = place
				affiliation.PresenceEventID = presence
			}
		}
		if r.Decision == "join" && definition.Definition.ScopeKind == "family" {
			source, err := requireCulturalFamilyMember(ctx, conn, r.Binding, definition.Definition.ScopeID, r.EntityID)
			if err != nil {
				return CultureFact{}, nil, err
			}
			affiliation.EligibilityEventID = source
		}
		if r.Decision == "join" && definition.Definition.ScopeKind == "organization" {
			jobs, err := readRPOwnEmployment(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, c.WorldTime)
			if err != nil {
				return CultureFact{}, nil, err
			}
			for _, job := range jobs {
				if job.OrganizationID == definition.Definition.ScopeID && job.Status != "onboarding" {
					affiliation.EligibilityEventID = job.SourceEventID
					affiliation.EmploymentContractID = job.ContractID
					break
				}
			}
			if affiliation.EligibilityEventID == "" {
				return CultureFact{}, nil, core.NewError(core.CodeUnauthorized, "organization cultural affiliation requires actual effective own employment")
			}
		}
		if r.Decision == "leave" {
			if r.TransmissionEventID != "" {
				return CultureFact{}, nil, core.NewError(core.CodeInvalidArgument, "leave retains original membership knowledge")
			}
			affiliation.Status = "left"
			affiliation.KnowledgeEventID = previous.KnowledgeEventID
			affiliation.DefinitionEventID = previous.DefinitionEventID
			affiliation.EligibilityEventID = previous.EligibilityEventID
			affiliation.EmploymentContractID = previous.EmploymentContractID
			affiliation.PresenceEventID = previous.PresenceEventID
			affiliation.ScopePlaceID = previous.ScopePlaceID
		} else if r.TransmissionEventID == "" {
			if definition.ActorID != r.EntityID {
				return CultureFact{}, nil, core.NewError(core.CodeUnauthorized, "joining requires authored or actually heard definition")
			}
			affiliation.KnowledgeEventID = r.DefinitionEventID
		} else {
			transmission, err := readCultureFact(ctx, conn, r.Binding, r.TransmissionEventID)
			if err != nil {
				return CultureFact{}, nil, err
			}
			heard := false
			if transmission.Kind == "transmission" && transmission.DefinitionEventID == r.DefinitionEventID {
				for _, id := range transmission.ListenerIDs {
					if id == r.EntityID {
						heard = true
					}
				}
			}
			if !heard {
				return CultureFact{}, nil, core.NewError(core.CodeUnauthorized, "membership requires this received version")
			}
			affiliation.KnowledgeEventID = r.TransmissionEventID
		}
		return CultureFact{Kind: "affiliation", ActorID: r.EntityID, DefinitionEventID: affiliation.DefinitionEventID, Affiliation: &affiliation}, nil, nil
	})
}

// Current own membership is an Event-derived view. It carries departures as
// well as joins, so leaving cannot silently erase a sourced part of identity.
func readOwnCultureAffiliations(ctx context.Context, conn *sql.Conn, entity string) ([]core.RPCultureAffiliation, error) {
	rows, err := conn.QueryContext(ctx, `SELECT e.event_id,e.payload FROM events e JOIN agent_profiles a ON a.agent_id=? AND a.instance_id=e.instance_id AND a.branch_id=e.branch_id WHERE e.event_type='RPCultureFactRecorded' AND json_extract(e.payload,'$.kind')='affiliation' AND json_extract(e.payload,'$.actor_id')=? ORDER BY e.event_sequence`, entity, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	current := map[string]core.RPCultureAffiliation{}
	for rows.Next() {
		var eventID, raw string
		var fact CultureFact
		if err := rows.Scan(&eventID, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return nil, err
		}
		a := fact.Affiliation
		if a == nil || a.EntityID != entity || a.SourceEventID != eventID || (a.Status != "joined" && a.Status != "left") {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid affiliation source")
		}
		current[a.CultureID] = *a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var result []core.RPCultureAffiliation
	for _, a := range current {
		result = append(result, a)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CultureID < result[j].CultureID })
	return result, nil
}
