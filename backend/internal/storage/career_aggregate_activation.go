package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

type careerAggregateActivation struct {
	CancelledSchedules          []CareerCancelledSchedule `json:"cancelled_schedules,omitempty"`
	NoticeEventID               string                    `json:"notice_event_id"`
	CandidateID                 string                    `json:"candidate_id"`
	ContractID                  string                    `json:"contract_id"`
	MaterializationID           string                    `json:"materialization_id"`
	FinalPeriodEnd              string                    `json:"final_period_end"`
	EarliestIndependentStartDay int                       `json:"earliest_independent_start_day"`
}

func queueCareerAggregateExit(ctx context.Context, conn *sql.Conn, source string, day int) error {
	var instanceID, branchID string
	if err := conn.QueryRowContext(ctx, `SELECT instance_id,branch_id FROM events WHERE event_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='aggregate_exit_notice'`, source).Scan(&instanceID, &branchID); err != nil {
		return classifyMissing(err, "aggregate exit source")
	}
	payload, err := core.CanonicalJSON(scheduledPayload{Kind: "career_aggregate_exit", SubjectID: source, Day: day})
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,0,'pending',?)`, "career_aggregate_exit_"+source, instanceID, branchID, careerTime(day, 7, 2), careerPayrollPhase, string(payload))
	return err
}

func (s *Store) executeCareerAggregateExit(ctx context.Context, tx *immediateTx, item SchedulerItem, payload scheduledPayload) error {
	instanceID, branchID, scopeErr := recordedSchedulerScope(ctx, tx.conn, item)
	if scopeErr != nil {
		return scopeErr
	}
	var raw string
	if err := tx.conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded'`, payload.SubjectID, instanceID, branchID).Scan(&raw); err != nil {
		return err
	}
	var source CareerFact
	if err := json.Unmarshal([]byte(raw), &source); err != nil {
		return err
	}
	n := source.AggregateExit
	if n == nil || source.Kind != "aggregate_exit_notice" || n.Status != "notice_recorded" || n.ContractID != m2EconomyContractID || n.FinalEarnedDay != payload.Day || n.FinalPeriodEnd != careerTime(payload.Day, 7, 0) || item.WorldTime != careerTime(payload.Day, 7, 2) || n.EarliestIndependentStartDay != payload.Day+1 {
		return core.NewError(core.CodeProjectionDiverged, "invalid aggregate exit activation source")
	}
	// Original-period accrual/payment must run first. Payment may legitimately
	// leave arrears; departure never cancels those claims.
	var completed int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_economic_obligations WHERE contract_id=? AND period_end=? AND status IN ('paid','partial','overdue')`, n.ContractID, n.FinalPeriodEnd).Scan(&completed); err != nil {
		return err
	}
	if completed != 1 {
		return core.NewError(core.CodeProjectionDiverged, "aggregate final period has not settled")
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT s.schedule_id,s.scheduler_item_id FROM agent_schedule_entries s JOIN scheduler_items q ON q.scheduler_item_id=s.scheduler_item_id JOIN events d ON d.event_id=s.definition_event_id WHERE s.agent_id=? AND s.status='active' AND s.activity_code='work' AND s.world_time>=? AND q.status='pending' AND q.instance_id=? AND q.branch_id=? AND d.instance_id=q.instance_id AND d.branch_id=q.branch_id AND d.event_type='RPBackgroundMaterialized' AND json_extract(d.payload,'$.entity_id')=s.agent_id AND EXISTS (SELECT 1 FROM json_each(d.payload,'$.initial_schedule') owned WHERE json_extract(owned.value,'$.employment_contract_id')=? AND json_extract(owned.value,'$.world_time')=s.world_time AND json_extract(owned.value,'$.place_id')=s.place_id AND json_extract(owned.value,'$.activity_code')='work') ORDER BY s.world_time,s.schedule_id`, source.CandidateID, item.WorldTime, instanceID, branchID, n.ContractID)
	if err != nil {
		return err
	}
	var cancelled []CareerCancelledSchedule
	for rows.Next() {
		var schedule CareerCancelledSchedule
		if err := rows.Scan(&schedule.ScheduleID, &schedule.SchedulerItemID); err != nil {
			rows.Close()
			return err
		}
		cancelled = append(cancelled, schedule)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	activation := careerAggregateActivation{CancelledSchedules: cancelled, NoticeEventID: payload.SubjectID, CandidateID: source.CandidateID, ContractID: n.ContractID, MaterializationID: n.MaterializationID, FinalPeriodEnd: n.FinalPeriodEnd, EarliestIndependentStartDay: n.EarliestIndependentStartDay}
	mutation := scheduledMutation{Private: true, EventType: "CareerAggregateExitActivated", EventPayload: activation, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, _ string, _ int64) error {
		for _, schedule := range cancelled {
			if err := execAgentOne(ctx, conn, "cancel sourced aggregate work", `UPDATE agent_schedule_entries SET status='cancelled' WHERE schedule_id=? AND agent_id=? AND status='active'`, schedule.ScheduleID, source.CandidateID); err != nil {
				return err
			}
			if err := execAgentOne(ctx, conn, "cancel sourced aggregate work task", `UPDATE scheduler_items SET status='cancelled' WHERE scheduler_item_id=? AND instance_id=? AND branch_id=? AND status='pending'`, schedule.SchedulerItemID, instanceID, branchID); err != nil {
				return err
			}
		}
		return nil
	}}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, instanceID, branchID); err != nil {
		return err
	}
	if s.beforeCommit != nil {
		return s.beforeCommit()
	}
	return nil
}

func readM2WageDepartures(ctx context.Context, conn *sql.Conn, periodEnd string) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT e.payload,e.world_time,COALESCE(n.payload,'') FROM events e LEFT JOIN events n ON n.event_id=json_extract(e.payload,'$.notice_event_id') AND n.instance_id=e.instance_id AND n.branch_id=e.branch_id AND n.event_type='RPCareerFactRecorded' WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='CareerAggregateExitActivated' AND json_extract(e.payload,'$.contract_id')=? AND json_extract(e.payload,'$.final_period_end')<? AND e.world_time<=? ORDER BY e.event_sequence`, M2DemoInstanceID, M2DemoBranchID, m2EconomyContractID, periodEnd, periodEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ended := map[string]bool{}
	for rows.Next() {
		var raw, at, noticeRaw string
		if err := rows.Scan(&raw, &at, &noticeRaw); err != nil {
			return nil, err
		}
		var activation careerAggregateActivation
		var notice CareerFact
		if json.Unmarshal([]byte(raw), &activation) != nil || json.Unmarshal([]byte(noticeRaw), &notice) != nil || notice.AggregateExit == nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "aggregate departure lacks notice authority")
		}
		n := notice.AggregateExit
		if notice.Kind != "aggregate_exit_notice" || notice.CandidateID != activation.CandidateID || n.ContractID != activation.ContractID || n.MaterializationID != activation.MaterializationID || n.FinalPeriodEnd != activation.FinalPeriodEnd || n.EarliestIndependentStartDay != activation.EarliestIndependentStartDay || at != careerTime(n.FinalEarnedDay, 7, 2) {
			return nil, core.NewError(core.CodeProjectionDiverged, "aggregate departure differs from notice")
		}
		id := activation.MaterializationID
		if id == "" || ended[id] {
			return nil, core.NewError(core.CodeProjectionDiverged, "duplicate aggregate wage departure")
		}
		ended[id] = true
	}
	return ended, rows.Err()
}

// Population includes former workers; economic participation is a separate
// conserved subset. Pre-contract named agents are not part of its baseline.
func verifyM2WagePopulation(ctx context.Context, conn *sql.Conn, baseline, population int64) error {
	var named int64
	err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(m.population_count),0) FROM cohort_materializations m JOIN m2_cohort_contracts w ON w.cohort_id=m.source_cohort_id JOIN events e ON e.event_id=w.definition_event_id WHERE w.contract_id=? AND m.materialize_sequence>e.event_sequence AND m.status='active'`, m2EconomyContractID).Scan(&named)
	if err != nil {
		return err
	}
	if population <= 0 || population+named != baseline {
		return core.NewError(core.CodeProjectionDiverged, "aggregate population differs from source baseline")
	}
	return nil
}
