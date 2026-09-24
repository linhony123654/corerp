package storage

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

type rpWarmCandidate struct {
	ActorID       string `json:"actor_id"`
	Reason        string `json:"reason"`
	SourceEventID string `json:"source_event_id"`
}

func readRPWarmCandidates(ctx context.Context, conn *sql.Conn, session RPSession, place, at string) ([]rpWarmCandidate, error) {
	policy, err := readRPOpportunityPolicy(ctx, conn, session.InstanceID, session.BranchID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !policy.Fact.Policy.WarmEnabled {
		return nil, nil
	}
	when, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return nil, err
	}
	sources := map[string]rpWarmCandidate{}
	life := &core.RPLifeContext{}
	if err := applyRPSocialLife(ctx, conn, session.ControlledEntityID, life); err != nil {
		return nil, err
	}
	for _, r := range life.Relationships {
		if len(r.SourceEventIDs) > 0 && (r.Trust >= 2 || r.Tension >= 2 || r.Obligation > 0) {
			sources[r.SubjectEntityID] = rpWarmCandidate{r.SubjectEntityID, "important_relationship", r.SourceEventIDs[len(r.SourceEventIDs)-1]}
		}
	}
	rows, err := conn.QueryContext(ctx, `SELECT s.agent_id,s.definition_event_id FROM agent_schedule_entries s JOIN scheduler_items q ON q.scheduler_item_id=s.scheduler_item_id JOIN agent_profiles a ON a.agent_id=s.agent_id WHERE a.instance_id=? AND a.branch_id=? AND a.status='active' AND s.status='active' AND q.status='pending' AND s.place_id=? AND q.world_time>? AND q.world_time<=? ORDER BY q.world_time,s.schedule_id`, session.InstanceID, session.BranchID, place, at, when.Add(time.Hour).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var actor, source string
		if err := rows.Scan(&actor, &source); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := sources[actor]; !ok {
			sources[actor] = rpWarmCandidate{actor, "imminent_scene_appointment", source}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Sort by actual last WARM decision, not map iteration or session order.
	type candidate struct {
		value rpWarmCandidate
		last  int64
	}
	var available []candidate
	for actor, source := range sources {
		if actor == session.ControlledEntityID {
			continue
		}
		var active int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles a JOIN agent_positions p ON p.agent_id=a.agent_id JOIN materialized_entities m ON m.entity_id=a.agent_id WHERE a.agent_id=? AND a.instance_id=? AND a.branch_id=? AND a.status='active' AND m.status='active' AND m.population_count=1 AND p.place_id<>?`, actor, session.InstanceID, session.BranchID, place).Scan(&active); err != nil {
			return nil, err
		}
		if active != 1 {
			continue
		}
		var recent int
		var last int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(event_sequence),0),COALESCE(SUM(CASE WHEN world_time>? THEN 1 ELSE 0 END),0) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWarmDecisionRecorded' AND json_extract(payload,'$.decision.actor_id')=?`, when.Add(-core.RPWarmDecisionInterval).Format(time.RFC3339), session.InstanceID, session.BranchID, actor).Scan(&last, &recent); err != nil {
			return nil, err
		}
		if recent == 0 {
			available = append(available, candidate{source, last})
		}
	}
	sort.Slice(available, func(i, j int) bool {
		if available[i].last != available[j].last {
			return available[i].last < available[j].last
		}
		return available[i].value.ActorID < available[j].value.ActorID
	})
	var result []rpWarmCandidate
	for _, c := range available {
		if len(result) == core.RPWarmDecisionLimit {
			break
		}
		result = append(result, c.value)
	}
	return result, nil
}
