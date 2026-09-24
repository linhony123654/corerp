package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

type RPBackgroundResult struct {
	Background    core.RPBackground `json:"background"`
	EventSequence int64             `json:"event_sequence"`
	Replayed      bool              `json:"replayed"`
}

// MaterializeRPBackground follows the existing conserved Cohort materialization.
// A crash between the two commands leaves a valid named entity, not lost assets;
// retrying this second command completes initialization without rematerializing.
func (s *Store) MaterializeRPBackground(ctx context.Context, r core.RPBackgroundRequest) (RPBackgroundResult, error) {
	var empty RPBackgroundResult
	if err := r.Validate(); err != nil {
		return empty, err
	}
	if r.InstanceID != M2DemoInstanceID || r.BranchID != M2DemoBranchID {
		return empty, core.NewError(core.CodeInvalidArgument, "background routine requires supported M2 scheduler scope")
	}
	r.Schedule = append([]core.RPBackgroundSchedule(nil), r.Schedule...)
	for i := range r.Schedule {
		at, _ := time.Parse(time.RFC3339, r.Schedule[i].WorldTime)
		r.Schedule[i].WorldTime = at.UTC().Format(time.RFC3339)
	}
	if err := r.Validate(); err != nil {
		return empty, err
	}
	hash, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	var b core.RPBackground
	b.Version = "corerp.background.v1"
	b.EntityID = r.EntityID
	err = tx.conn.QueryRowContext(ctx, `SELECT n.display_name,n.source_cohort_id,m.materialize_event_id FROM materialized_entities n
	 JOIN cohorts c ON c.cohort_id=n.source_cohort_id JOIN cohort_materializations m ON m.materialization_id=n.materialization_id
	 WHERE n.entity_id=? AND n.status='active' AND n.population_count=1 AND c.instance_id=? AND c.branch_id=?`, r.EntityID, r.InstanceID, r.BranchID).Scan(&b.DisplayName, &b.SourceCohortID, &b.MaterializationEventID)
	if err != nil {
		return empty, classifyMissing(err, "active individual materialization")
	}
	var authorized int
	err = tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g JOIN principals p ON p.principal_id=g.principal_id
	 WHERE g.principal_id=? AND p.status='active' AND g.capability_id='world.cohort.materialize' AND g.instance_id=? AND g.branch_id=?
	 AND g.subject_id IN (?,'*') AND g.status='active'`, r.PrincipalID, r.InstanceID, r.BranchID, b.SourceCohortID).Scan(&authorized)
	if err != nil {
		return empty, err
	}
	if authorized == 0 {
		return empty, core.NewError(core.CodeUnauthorized, "background requires source Cohort materialization authority")
	}
	key := "rp_background:" + r.PrincipalID + ":" + r.IdempotencyKey
	var oldHash, raw string
	var result RPBackgroundResult
	err = tx.conn.QueryRowContext(ctx, `SELECT c.request_hash,e.event_sequence,e.payload FROM commands c JOIN event_batches b ON b.command_id=c.command_id JOIN events e ON e.batch_id=b.batch_id
	 WHERE c.instance_id=? AND c.branch_id=? AND c.command_type='MaterializeRPBackground' AND c.idempotency_key=?`, r.InstanceID, r.BranchID, key).Scan(&oldHash, &result.EventSequence, &raw)
	if err == nil {
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "background retry differs")
		}
		if err := json.Unmarshal([]byte(raw), &result.Background); err != nil {
			return empty, err
		}
		result.Replayed = true
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	var exists int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles WHERE agent_id=?`, r.EntityID).Scan(&exists); err != nil {
		return empty, err
	}
	if exists != 0 {
		return empty, core.NewError(core.CodeMaterializationConflict, "existing individual background/profile cannot be redefined")
	}
	var head int64
	var nowWorld, epoch string
	err = tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, r.InstanceID, r.BranchID).Scan(&head, &nowWorld)
	if err != nil {
		return empty, err
	}
	if head != r.ExpectedHead {
		return empty, core.NewError(core.CodeBranchConflict, "background expected head differs")
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+(SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, r.InstanceID, r.BranchID, r.InstanceID, r.BranchID).Scan(&pending); err != nil {
		return empty, err
	}
	if pending != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "finish RP action before background initialization")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, r.InstanceID, r.BranchID, nowWorld); err != nil {
		return empty, err
	}
	placeEvidence := func(id, kind string) (string, error) {
		var source, actualKind string
		err := tx.conn.QueryRowContext(ctx, `SELECT definition_event_id,place_kind FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=? AND status='active'`, id, r.InstanceID, r.BranchID).Scan(&source, &actualKind)
		if err != nil {
			return "", classifyMissing(err, "background place")
		}
		if kind != "" && actualKind != kind {
			return "", core.NewError(core.CodeInvalidArgument, "background place kind differs")
		}
		return source, nil
	}
	b.ResidenceSourceEventID, err = placeEvidence(r.ResidencePlaceID, "home")
	if err != nil {
		return empty, err
	}
	if _, err := placeEvidence(r.InitialPlaceID, ""); err != nil {
		return empty, err
	}
	nowInstant, err := time.Parse(time.RFC3339, nowWorld)
	if err != nil {
		return empty, err
	}
	from := r.InitialPlaceID
	b.InitialEmployment = []core.RPOwnEmployment{}
	employmentSeen := map[string]bool{}
	for _, item := range r.Schedule {
		at, _ := time.Parse(time.RFC3339, item.WorldTime)
		if !at.After(nowInstant) || at.After(nowInstant.Add(7*24*time.Hour)) {
			return empty, core.NewError(core.CodeInvalidArgument, "initial routine must be within next seven days")
		}
		kind := ""
		if item.ActivityCode == "work" {
			kind = "work"
		}
		if item.ActivityCode == "home" {
			kind = "home"
			if item.PlaceID != r.ResidencePlaceID {
				return empty, core.NewError(core.CodeInvalidArgument, "home routine differs from declared residence")
			}
		}
		if _, err := placeEvidence(item.PlaceID, kind); err != nil {
			return empty, err
		}
		var route int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_place_links WHERE instance_id=? AND branch_id=? AND from_place_id=? AND to_place_id=?`, r.InstanceID, r.BranchID, from, item.PlaceID).Scan(&route); err != nil {
			return empty, err
		}
		if route == 0 {
			return empty, core.NewError(core.CodeInvalidArgument, "background routine lacks an existing route")
		}
		from = item.PlaceID
		// Existing participation proves employment; this authorized background
		// event defines the individual's initial work appointment/site. It does
		// not claim the organization owns the place or create a new job.
		if item.ActivityCode == "work" {
			jobs, err := readRPOwnEmployment(ctx, tx.conn, r.InstanceID, r.BranchID, r.EntityID, item.WorldTime)
			if err != nil {
				return empty, err
			}
			found := false
			for _, job := range jobs {
				if job.ContractID == item.EmploymentContractID {
					found = true
					if !employmentSeen[job.ContractID] {
						b.InitialEmployment = append(b.InitialEmployment, job)
						employmentSeen[job.ContractID] = true
					}
				}
			}
			if !found {
				return empty, core.NewError(core.CodeInvalidArgument, "work routine lacks own effective employment evidence")
			}
		}
	}
	sequence := head + 1
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, r.InstanceID, r.BranchID, sequence, sequence).Scan(&epoch); err != nil {
		return empty, err
	}
	idHash, err := core.HashJSON([]string{r.InstanceID, r.BranchID, key})
	if err != nil {
		return empty, err
	}
	suffix := idHash[7:]
	commandID, eventID, batchID, attemptID := "cmd_rp_background_"+suffix, "event_rp_background_"+suffix, "batch_rp_background_"+suffix, "attempt_rp_background_"+suffix
	principalID := "principal_rp_background_" + suffix
	b.DefinitionEventID = eventID
	b.DefinedWorldTime = nowWorld
	b.AgeMin = r.AgeMin
	b.AgeMax = r.AgeMax
	b.ResidencePlaceID = r.ResidencePlaceID
	b.InitialPlaceID = r.InitialPlaceID
	b.Schedule = r.Schedule
	b.Disposition = core.DeriveRPDisposition(r.EntityID, b.MaterializationEventID)
	encoded, err := core.CanonicalJSON(b)
	if err != nil {
		return empty, err
	}
	batchHash, err := core.HashJSON(struct {
		RequestHash string
		Sequence    int64
		Background  core.RPBackground
	}{hash, sequence, b})
	if err != nil {
		return empty, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"background command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'MaterializeRPBackground',?,?,?,?, '{"authorization":"source-cohort-materialize"}','pending',?)`, []any{commandID, r.InstanceID, r.BranchID, key, hash, head, r.PrincipalID, now}},
		{"background attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-rp2',?,?,?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), hash, now}},
		{"background batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, r.InstanceID, r.BranchID, epoch, head, sequence, sequence, nowWorld, batchHash, now}},
		{"background event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,'RPBackgroundMaterialized',?,?,?)`, []any{eventID, batchID, r.InstanceID, r.BranchID, sequence, r.PrincipalID, nowWorld, string(encoded)}},
		{"background principal", `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'agent',?,'active')`, []any{principalID, b.DisplayName}},
		{"background profile", `INSERT INTO agent_profiles(agent_id,instance_id,branch_id,principal_id,agent_level,goal_code,action_budget_per_day,status,definition_event_id) VALUES (?,?,?,?,'L2','keep_daily_routine',8,'active',?)`, []any{r.EntityID, r.InstanceID, r.BranchID, principalID, eventID}},
		{"background movement", `INSERT INTO agent_movements(movement_id,event_id,agent_id,from_place_id,to_place_id,schedule_id,activity_code,world_time,movement_kind) VALUES (?,?,?,NULL,?,NULL,'present',?,'initialize')`, []any{"movement_" + eventID, eventID, r.EntityID, r.InitialPlaceID, nowWorld}},
		{"background position", `INSERT INTO agent_positions(agent_id,place_id,activity_code,effective_world_time,projection_version,last_event_sequence) VALUES (?,?,'present',?,0,?)`, []any{r.EntityID, r.InitialPlaceID, nowWorld, sequence}},
	}
	for _, st := range statements {
		if err := execAgentOne(ctx, tx.conn, st.name, st.query, st.args...); err != nil {
			return empty, err
		}
	}
	for i, item := range r.Schedule {
		scheduleID := fmt.Sprintf("schedule_%s_%02d", suffix, i)
		itemID := "item_" + scheduleID
		// Same bounded M2 calendar as RP Wait; a future appointment must not
		// reset current_day to zero when the existing scheduler executes it.
		base, _ := time.Parse(time.RFC3339, "2026-09-22T00:00:00Z")
		at, _ := time.Parse(time.RFC3339, item.WorldTime)
		day := int(at.Sub(base) / (24 * time.Hour))
		payload, err := core.CanonicalJSON(agentSchedulePayload{Kind: "agent_move", Day: day, AgentID: r.EntityID, ScheduleID: scheduleID, ToPlaceID: item.PlaceID, ActivityCode: item.ActivityCode})
		if err != nil {
			return empty, err
		}
		if err := execAgentOne(ctx, tx.conn, "background scheduler item", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,20,'pending',?)`, itemID, r.InstanceID, r.BranchID, item.WorldTime, m2AgentPhaseID, string(payload)); err != nil {
			return empty, err
		}
		if err := execAgentOne(ctx, tx.conn, "background schedule", `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?,?,20,?,'active',?)`, scheduleID, r.EntityID, item.WorldTime, item.PlaceID, item.ActivityCode, itemID, eventID); err != nil {
			return empty, err
		}
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, nowWorld, now, encoded); err != nil {
		return empty, err
	}
	// No unfiltered background Outbox: age/residence are not automatically
	// known to every player or bystander. Decision context exposes only self.
	for _, st := range []struct {
		name, query string
		args        []any
	}{
		{"background clock", `UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, []any{sequence, r.InstanceID, r.BranchID, nowWorld}},
		{"background branch", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, []any{sequence, r.InstanceID, r.BranchID, head}},
		{"background committed command", `UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, []any{commandID}},
		{"background committed attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE attempt_id=? AND status='ready'`, []any{now, attemptID}},
	} {
		if err := execAgentOne(ctx, tx.conn, st.name, st.query, st.args...); err != nil {
			return empty, err
		}
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return RPBackgroundResult{Background: b, EventSequence: sequence}, nil
}
