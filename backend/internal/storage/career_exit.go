package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type CareerExitFact struct {
	Kind                   string `json:"kind"`
	RequestedByPrincipalID string `json:"requested_by_principal_id"`
	EffectiveFromDay       int    `json:"effective_from_day"`
	Notice                 string `json:"notice"`
}

func (s *Store) EndCareerEmployment(ctx context.Context, r core.CareerExitRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "EndCareerEmployment", r, func(conn *sql.Conn) error {
		org, err := readCareerContractOrganization(ctx, conn, b, r.ContractID)
		if err != nil {
			return err
		}
		if r.Kind != "resignation" {
			return authorizeCareerManager(ctx, conn, b, org)
		}
		var employee string
		if err := conn.QueryRowContext(ctx, `SELECT employee_entity_id FROM employment_contracts WHERE contract_id=?`, r.ContractID).Scan(&employee); err != nil {
			return err
		}
		return authorizeCareerCandidate(ctx, conn, b, employee)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		job, source, day, err := currentCareerEmployment(ctx, conn, b, r.ContractID, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if r.EffectiveFromDay <= day || r.EffectiveFromDay > day+30 {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "exit must take effect one to thirty days ahead")
		}
		var pending int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.employment.contract_id')=? AND json_extract(payload,'$.employment.effective_from_day')>?`, b.InstanceID, b.BranchID, job.ContractID, day).Scan(&pending); err != nil {
			return CareerFact{}, nil, err
		}
		if pending != 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "resolve pending employment terms before exit")
		}
		overtime, err := careerAcceptedOvertime(ctx, conn, job.ContractID, r.EffectiveFromDay, day+31)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if len(overtime) > 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "explicitly cancel agreed overtime on or after exit first")
		}
		adopted, err := careerAdoptableWorkSchedules(ctx, conn, job, r.EffectiveFromDay, 0)
		if err != nil {
			return CareerFact{}, nil, err
		}
		job.TermVersion++
		job.EffectiveFromDay, job.EndsOnDay, job.LifecycleStatus, job.Capabilities = r.EffectiveFromDay, r.EffectiveFromDay, "ended", nil
		fact := CareerFact{Kind: "employment", RecordID: job.ContractID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, Employment: &job, Exit: &CareerExitFact{Kind: r.Kind, RequestedByPrincipalID: b.PrincipalID, EffectiveFromDay: r.EffectiveFromDay, Notice: r.Notice}, EmploymentChange: &CareerEmploymentChange{Kind: r.Kind, PreviousTermsEventID: source, Notice: r.Notice}, AdoptedWorkScheduleIDs: adopted}
		if r.Kind != "resignation" {
			fact.EmploymentChange.ManagerPrincipalID = b.PrincipalID
		}
		return fact, func() error {
			return queueCareerPayrollItem(ctx, conn, c.EventID, r.EffectiveFromDay, 0, "career_terms_effective")
		}, nil
	})
}

func careerPlannedEndDay(ctx context.Context, conn *sql.Conn, contract string) (int, error) {
	var day int
	err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(CAST(json_extract(payload,'$.employment.ends_on_day') AS INTEGER)),0) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.employment.contract_id')=?`, M2DemoInstanceID, M2DemoBranchID, contract).Scan(&day)
	return day, err
}

func readCareerUnemployment(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, life *core.RPLifeContext) error {
	if len(life.Employment) > 0 {
		return nil
	}
	var state core.RPUnemployment
	err := conn.QueryRowContext(ctx, `SELECT json_extract(a.payload,'$.contract_id'),json_extract(a.payload,'$.effective_from_day'),json_extract(e.payload,'$.exit.kind'),a.event_id FROM events a JOIN events e ON e.event_id=json_extract(a.payload,'$.terms_event_id') AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id JOIN employment_contracts c ON c.contract_id=json_extract(a.payload,'$.contract_id') AND c.status='ended' WHERE a.instance_id=? AND a.branch_id=? AND a.event_type='CareerEmploymentTermsActivated' AND json_extract(a.payload,'$.contract_status')='ended' AND json_extract(e.payload,'$.employment.employee_id')=? ORDER BY a.event_sequence DESC LIMIT 1`, input.InstanceID, input.BranchID, input.NPCEntityID).Scan(&state.PreviousContractID, &state.SinceDay, &state.Kind, &state.SourceEventID)
	if err == sql.ErrNoRows {
		err = conn.QueryRowContext(ctx, `SELECT json_extract(payload,'$.contract_id'),json_extract(payload,'$.earliest_independent_start_day')-1,'resignation',event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='CareerAggregateExitActivated' AND json_extract(payload,'$.candidate_id')=? ORDER BY event_sequence DESC LIMIT 1`, input.InstanceID, input.BranchID, input.NPCEntityID).Scan(&state.PreviousContractID, &state.SinceDay, &state.Kind, &state.SourceEventID)
		if err == sql.ErrNoRows {
			return nil
		}
	}
	if err != nil {
		return err
	}
	life.Unemployment = &state
	return nil
}
