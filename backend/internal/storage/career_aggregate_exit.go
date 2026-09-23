package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"corerp.local/backend/internal/core"
)

type CareerAggregateExitFact struct {
	ContractID                  string `json:"contract_id"`
	MaterializationID           string `json:"materialization_id"`
	SplitEventID                string `json:"split_event_id"`
	FinalEarnedDay              int    `json:"final_earned_day"`
	FinalPeriodEnd              string `json:"final_period_end"`
	EarliestIndependentStartDay int    `json:"earliest_independent_start_day"`
	Status                      string `json:"status"`
	Notice                      string `json:"notice"`
}

func appendCareerAggregateExitMemories(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, life *core.RPLifeContext) error {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,world_time,COALESCE(json_extract(payload,'$.aggregate_exit.final_earned_day'),json_extract(payload,'$.earliest_independent_start_day')-1),COALESCE(json_extract(payload,'$.aggregate_exit.notice'),''),event_type FROM events WHERE instance_id=? AND branch_id=? AND ((event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='aggregate_exit_notice') OR event_type='CareerAggregateExitActivated') AND json_extract(payload,'$.candidate_id')=? ORDER BY event_sequence DESC LIMIT 5`, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		memory := core.RPLifeMemory{Kind: "own_aggregate_exit_notice", SubjectEntityID: input.NPCEntityID}
		var day int
		var notice, kind string
		if err := rows.Scan(&memory.SourceEventID, &memory.WorldTime, &day, &notice, &kind); err != nil {
			return err
		}
		memory.Text = fmt.Sprintf("Requested exit from aggregate employment after the earned period ending day %d at 07:00. This notice is not completed termination or permission to start overlapping employment. Notice: %s", day, notice)
		if kind == "CareerAggregateExitActivated" {
			memory.Kind = "own_aggregate_employment_ended"
			memory.Text = fmt.Sprintf("Aggregate employment ended after the earned period ending day %d at 07:00. Existing earned claims remain owed; an independent job may start from day %d.", day, day+1)
		}
		life.SalientMemories = append(life.SalientMemories, memory)
	}
	return rows.Err()
}

// Only scheduled activation, not this notice alone, changes eligibility.
func (s *Store) RequestCareerAggregateExit(ctx context.Context, r core.CareerAggregateExitRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "RequestCareerAggregateExit", r, func(conn *sql.Conn) error {
		return authorizeCareerCandidate(ctx, conn, b, r.CandidateID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if r.ContractID != m2EconomyContractID {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "unsupported aggregate wage contract")
		}
		if err := verifyM2WageParticipationReturns(ctx, conn); err != nil {
			return CareerFact{}, nil, err
		}
		var materialization, splitEvent, starts, ends, organization, raw string
		err := conn.QueryRowContext(ctx, `SELECT s.materialization_id,s.split_event_id,s.effective_from,w.effective_until,w.actor_id,e.payload
		FROM m2_wage_participation_splits s JOIN m2_cohort_contracts w ON w.contract_id=s.contract_id AND w.kind='wage'
		JOIN cohort_materializations m ON m.materialization_id=s.materialization_id AND m.status='active' AND m.entity_id=s.entity_id AND m.materialize_event_id=s.split_event_id AND m.materialize_sequence=s.split_event_sequence
		JOIN events e ON e.event_id=s.split_event_id AND e.event_sequence=s.split_event_sequence AND e.event_type='CohortMaterialized' AND e.instance_id=? AND e.branch_id=?
		WHERE s.entity_id=? AND s.contract_id=? AND s.worker_count=1 AND s.cohort_id=?`, b.InstanceID, b.BranchID, r.CandidateID, r.ContractID, M2DemoCohortID).Scan(&materialization, &splitEvent, &starts, &ends, &organization, &raw)
		if err != nil {
			return CareerFact{}, nil, classifyMissing(err, "active aggregate wage participation")
		}
		var origin materializationEventPayload
		if err := json.Unmarshal([]byte(raw), &origin); err != nil {
			return CareerFact{}, nil, err
		}
		hash, err := core.HashJSON(m2WageParticipation{r.ContractID, starts, 1})
		if err != nil {
			return CareerFact{}, nil, err
		}
		if origin.MaterializationID != materialization || origin.EntityID != r.CandidateID || origin.SourceCohortID != M2DemoCohortID || origin.PopulationCount != 1 || origin.WageParticipationHash != hash {
			return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "aggregate exit lacks original participation authority")
		}
		final := m2WageTime(r.FinalEarnedDay, 7, 0)
		if final <= c.WorldTime || final < starts || final >= ends {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "final earned period must be future and within participation")
		}
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.aggregate_exit.materialization_id')=?`, b.InstanceID, b.BranchID, materialization).Scan(&count); err != nil {
			return CareerFact{}, nil, err
		}
		if count != 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "aggregate exit notice already exists")
		}
		fact := CareerAggregateExitFact{r.ContractID, materialization, splitEvent, r.FinalEarnedDay, final, r.FinalEarnedDay + 1, "notice_recorded", r.Notice}
		return CareerFact{Kind: "aggregate_exit_notice", RecordID: materialization, OrganizationID: organization, CandidateID: r.CandidateID, AggregateExit: &fact}, func() error { return queueCareerAggregateExit(ctx, conn, c.EventID, r.FinalEarnedDay) }, nil
	})
}
