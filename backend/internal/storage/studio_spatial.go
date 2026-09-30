package storage

import (
	"context"
	"database/sql"
	"fmt"

	"corerp.local/backend/internal/core"
)

type StudioSpatialFact struct {
	Version           string `json:"version"`
	GenesisEventID    string `json:"genesis_event_id"`
	PlayerEntityID    string `json:"player_entity_id"`
	PlayerPrincipalID string `json:"player_principal_id"`
	PlaceCount        int    `json:"place_count"`
	ParticipantCount  int    `json:"participant_count"`
	ObjectCount       int    `json:"object_count"`
}

// PrepareStudioSpatial saves the declared topology and initial physical actors.
// It creates no control grants and does not activate the pending Rule Epoch.
func (s *Store) PrepareStudioSpatial(ctx context.Context, r StudioGenesisRequest) (privateFactRecord[StudioSpatialFact], error) {
	var empty privateFactRecord[StudioSpatialFact]
	if err := validateStudioGenesisRequest(r); err != nil {
		return empty, err
	}
	id := func(kind, key string) string { v, _ := core.StudioWorldObjectID(r.InstanceID, kind, key); return v }
	binding := core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: r.InstanceID, BranchID: "br_main", ExpectedHead: int64(1 + len(r.Spec.People)), IdempotencyKey: id("key", "spatial")}
	return executePrivateFactCommand(s, ctx, binding, "PrepareStudioSpatial", r,
		privateFactDomain{"studio_spatial", "StudioSpatialPrepared", `{"authorization":"sourced-world-create"}`},
		func(conn *sql.Conn) error { return authorizeSavedStudioGenesis(ctx, conn, r) },
		func(conn *sql.Conn, c privateFactContext) (StudioSpatialFact, func() error, error) {
			fact := StudioSpatialFact{Version: "corerp.studio-spatial.v1", GenesisEventID: id("event", "genesis"), PlaceCount: len(r.Spec.Places), ParticipantCount: len(r.Spec.People), ObjectCount: len(r.Spec.Objects)}
			var ready int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_instances w JOIN world_clocks c ON c.instance_id=w.instance_id AND c.branch_id='br_main' WHERE w.instance_id=? AND w.lifecycle_state='paused' AND c.status='paused' AND c.current_world_time=?`, r.InstanceID, r.Spec.StartWorldTime).Scan(&ready); err != nil {
				return fact, nil, err
			}
			if ready != 1 || c.WorldTime != r.Spec.StartWorldTime {
				return fact, nil, core.NewError(core.CodeBranchConflict, "spatial preparation requires paused initial world")
			}
			for _, person := range r.Spec.People {
				if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM materialized_entities e JOIN cohorts c ON c.cohort_id=e.source_cohort_id JOIN cohort_materializations m ON m.materialization_id=e.materialization_id JOIN events v ON v.event_id=m.materialize_event_id WHERE e.entity_id=? AND e.materialization_id=? AND e.source_cohort_id=? AND e.population_count=1 AND e.status='active' AND m.status='active' AND c.instance_id=? AND c.branch_id='br_main' AND v.instance_id=c.instance_id AND v.branch_id=c.branch_id AND v.actor_id=? AND v.event_type='CohortMaterialized'`, id("entity", person.Key), id("materialization", person.Key), id("cohort", "population"), r.InstanceID, r.PrincipalID).Scan(&ready); err != nil {
					return fact, nil, err
				}
				if ready != 1 {
					return fact, nil, core.NewError(core.CodeProjectionDiverged, "declared participant materialization missing")
				}
				if person.Player {
					fact.PlayerEntityID = id("entity", person.Key)
					fact.PlayerPrincipalID = id("principal", person.Key)
				}
			}
			return fact, func() error {
				exec := func(q string, args ...any) error {
					return execAgentOne(ctx, conn, "Studio spatial preparation", q, args...)
				}
				for _, place := range r.Spec.Places {
					if err := exec(`INSERT INTO agent_places(place_id,instance_id,branch_id,display_name,place_kind,status,definition_event_id) VALUES (?,?,'br_main',?,?,'active',?)`, id("place", place.Key), r.InstanceID, place.Name, place.Kind, c.EventID); err != nil {
						return err
					}
					if err := exec(`INSERT INTO rp_location_nodes(location_id,instance_id,branch_id,readable_path,generator_version,definition_event_id) VALUES (?,?,'br_main',?,'declared',?)`, id("place", place.Key), r.InstanceID, "/"+place.Key, c.EventID); err != nil {
						return err
					}
				}
				for i, link := range r.Spec.Links {
					for j, endpoints := range [][2]string{{link.From, link.To}, {link.To, link.From}} {
						if err := exec(`INSERT INTO rp_place_links(link_id,instance_id,branch_id,from_place_id,to_place_id,definition_event_id) VALUES (?,?,'br_main',?,?,?)`, id("link", fmt.Sprintf("link-%02d-%d", i, j)), r.InstanceID, id("place", endpoints[0]), id("place", endpoints[1]), c.EventID); err != nil {
							return err
						}
					}
				}
				for _, person := range r.Spec.People {
					principal, entity, place := id("principal", person.Key), id("entity", person.Key), id("place", person.Place)
					kind, goal := "agent", "keep_daily_routine"
					if person.Player {
						kind, goal = "player", "player_controlled"
					}
					if err := exec(`INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, principal, kind, person.Name); err != nil {
						return err
					}
					if err := exec(`INSERT INTO agent_profiles(agent_id,instance_id,branch_id,principal_id,agent_level,goal_code,action_budget_per_day,persona_text,status,definition_event_id) VALUES (?,?,'br_main',?,'L2',?,8,?,'active',?)`, entity, r.InstanceID, principal, goal, person.Persona, c.EventID); err != nil {
						return err
					}
					if err := exec(`INSERT INTO agent_movements(movement_id,event_id,agent_id,from_place_id,to_place_id,schedule_id,activity_code,world_time,movement_kind) VALUES (?,?,?,NULL,?,NULL,'present',?,'initialize')`, id("movement", person.Key), c.EventID, entity, place, c.WorldTime); err != nil {
						return err
					}
					if err := exec(`INSERT INTO agent_positions(agent_id,place_id,activity_code,effective_world_time,projection_version,last_event_sequence) VALUES (?,?,'present',?,0,?)`, entity, place, c.WorldTime, c.Sequence); err != nil {
						return err
					}
					for i, entry := range person.Routine {
						item, schedule := id("item", fmt.Sprintf("%s-r%02d", person.Key, i)), id("schedule", fmt.Sprintf("%s-r%02d", person.Key, i))
						payload, err := core.CanonicalJSON(agentSchedulePayload{Kind: "agent_move", Day: 0, AgentID: entity, ScheduleID: schedule, ToPlaceID: id("place", entry.Place), ActivityCode: entry.ActivityCode})
						if err != nil {
							return err
						}
						if err := exec(`INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,'pending',?)`, item, r.InstanceID, "br_main", entry.WorldTime, studioRoutinePhaseID, i, string(payload)); err != nil {
							return err
						}
						if err := exec(`INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?,?,?,?,'active',?)`, schedule, entity, entry.WorldTime, id("place", entry.Place), entry.ActivityCode, i, item, c.EventID); err != nil {
							return err
						}
					}
				}
				for _, object := range r.Spec.Objects {
					if err := exec(`INSERT INTO rp_scene_objects(object_id,instance_id,branch_id,object_key,display_name,object_kind,place_id,state_code,definition_event_id,state_event_id,projection_version,last_event_sequence) VALUES (?,?,'br_main',?,?,?,?,?,?,?,0,?)`, id("object", object.Key), r.InstanceID, object.Key, object.Name, object.Kind, id("place", object.Place), object.InitialState, c.EventID, c.EventID, c.Sequence); err != nil {
						return err
					}
				}
				return nil
			}, nil
		})
}
