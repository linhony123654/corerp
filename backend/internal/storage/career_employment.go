package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

type CareerEmploymentFact struct {
	ContractID       string   `json:"contract_id"`
	EmployeeID       string   `json:"employee_id"`
	OrganizationID   string   `json:"organization_id"`
	PositionID       string   `json:"position_id"`
	PositionKey      string   `json:"position_key"`
	OccupationID     string   `json:"occupation_id"`
	Grade            string   `json:"grade"`
	WorkplaceID      string   `json:"workplace_id"`
	AfterWorkPlaceID string   `json:"after_work_place_id"`
	WorkStartHour    int      `json:"work_start_hour"`
	WorkEndHour      int      `json:"work_end_hour"`
	StartsOnDay      int      `json:"starts_on_day"`
	EndsOnDay        int      `json:"ends_on_day,omitempty"`
	EffectiveFromDay int      `json:"effective_from_day"`
	TermVersion      int      `json:"term_version"`
	ProbationDays    int      `json:"probation_days"`
	DailyWageMinor   int64    `json:"daily_wage_minor"`
	CurrencyID       string   `json:"currency_id"`
	WagePolicy       string   `json:"wage_policy"`
	LifecycleStatus  string   `json:"lifecycle_status"`
	Capabilities     []string `json:"capabilities,omitempty"`
}

func careerTime(day, hour, minute int) string {
	return time.Date(2026, time.September, 22+day, hour, minute, 0, 0, time.UTC).Format(time.RFC3339)
}

func readCareerEmploymentTerms(ctx context.Context, conn *sql.Conn, contractID string, day int) (CareerEmploymentFact, string, error) {
	var raw, source string
	err := conn.QueryRowContext(ctx, `SELECT e.payload,e.event_id FROM employment_contracts c JOIN events origin ON origin.event_id=c.definition_event_id JOIN events e ON e.instance_id=origin.instance_id AND e.branch_id=origin.branch_id WHERE c.contract_id=? AND e.event_type='RPCareerFactRecorded' AND json_extract(e.payload,'$.employment.contract_id')=c.contract_id AND json_extract(e.payload,'$.employment.effective_from_day')<=? ORDER BY e.event_sequence DESC LIMIT 1`, contractID, day).Scan(&raw, &source)
	if err != nil {
		return CareerEmploymentFact{}, "", classifyMissing(err, "effective career employment terms")
	}
	var fact CareerFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Employment == nil {
		return CareerEmploymentFact{}, "", core.NewError(core.CodeProjectionDiverged, "invalid career terms")
	}
	return *fact.Employment, source, nil
}

func (s *Store) AcceptCareerOffer(ctx context.Context, r core.CareerOfferAcceptRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	if b.InstanceID != M2DemoInstanceID || b.BranchID != M2DemoBranchID {
		return CareerRecord{}, core.NewError(core.CodeInvalidArgument, "career employment requires supported M2 scheduler")
	}
	return s.executeCareerCommand(ctx, b, "AcceptCareerOffer", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordCandidate(ctx, conn, b, "offer", r.OfferID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "offer", r.OfferID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		offer := record.Fact.Offer
		if offer == nil || offer.Status != "offered" {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "offer is no longer open")
		}
		now, err := time.Parse(time.RFC3339, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		expires, err := time.Parse(time.RFC3339, offer.ExpiresAt)
		if err != nil || !expires.After(now) {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "offer expired")
		}
		var raw, latest string
		if err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='evaluation' AND json_extract(payload,'$.evaluation.application_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, offer.ApplicationID).Scan(&latest, &raw); err != nil {
			return CareerFact{}, nil, err
		}
		var evaluation CareerFact
		if err := json.Unmarshal([]byte(raw), &evaluation); err != nil || evaluation.Evaluation == nil {
			return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "invalid offer assessment")
		}
		if latest != offer.EvaluationEventID || evaluation.Evaluation.Decision != "advance" {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "offer assessment was superseded")
		}
		managerBinding := b
		managerBinding.PrincipalID = evaluation.Evaluation.EvaluatorPrincipalID
		if err := authorizeCareerManager(ctx, conn, managerBinding, record.Fact.OrganizationID); err != nil {
			return CareerFact{}, nil, err
		}
		for _, assessment := range evaluation.Evaluation.Assessments {
			if !assessment.Passed {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "offer qualification no longer passes")
			}
		}
		posting, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", offer.PositionID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if err := requireCareerPostingCompatibility(ctx, conn, b, offer.PostingEventID, posting); err != nil {
			return CareerFact{}, nil, err
		}
		if len(evaluation.Evaluation.Assessments) != len(posting.Fact.Posting.RequiredQualifications) {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "offer posting terms changed")
		}
		if err := requireCareerCredentials(ctx, conn, b, careerRecordCandidate(record.Fact), posting.Fact.Posting.RequiredCredentials, c.WorldTime); err != nil {
			return CareerFact{}, nil, err
		}
		var count int
		count, err = careerPositionOccupancy(ctx, conn, offer.PositionKey)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if count >= posting.Fact.Posting.Capacity {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "position has no remaining vacancy")
		}
		candidate := careerRecordCandidate(record.Fact)
		application, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "application", offer.ApplicationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if application.Fact.Application == nil || application.Fact.Application.Status != "submitted" {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "application is no longer pending")
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_contracts WHERE employee_entity_id=? AND status='active'`, candidate).Scan(&count); err != nil {
			return CareerFact{}, nil, err
		}
		if count != 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "end existing employment before accepting overlapping job")
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_participation_splits s JOIN m2_cohort_contracts w ON w.contract_id=s.contract_id LEFT JOIN m2_wage_participation_returns r ON r.materialization_id=s.materialization_id WHERE s.entity_id=? AND (w.effective_until IS NULL OR w.effective_until>?) AND (r.effective_from IS NULL OR r.effective_from>?) AND NOT EXISTS (SELECT 1 FROM events ended WHERE ended.instance_id=? AND ended.branch_id=? AND ended.event_type='CareerAggregateExitActivated' AND json_extract(ended.payload,'$.materialization_id')=s.materialization_id AND json_extract(ended.payload,'$.earliest_independent_start_day')<=?)`, candidate, careerTime(offer.StartsOnDay, 0, 0), careerTime(offer.StartsOnDay, 0, 0), b.InstanceID, b.BranchID, offer.StartsOnDay).Scan(&count); err != nil {
			return CareerFact{}, nil, err
		}
		if count != 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "aggregate wage participation must end explicitly before replacement employment")
		}
		org, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "organization", record.Fact.OrganizationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if org.Fact.Organization == nil || offer.WorkStartHour != 8 || offer.WorkEndHour != 12 || offer.WagePolicy != "guaranteed_daily_v1" {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "offer lacks supported work window")
		}
		workplace := org.Fact.Organization.Definition.WorkplaceID
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places p WHERE p.place_id=? AND p.instance_id=? AND p.branch_id=? AND p.status='active' AND (p.place_id=? OR EXISTS (SELECT 1 FROM rp_place_links l WHERE l.instance_id=p.instance_id AND l.branch_id=p.branch_id AND l.from_place_id=? AND l.to_place_id=p.place_id))`, r.AfterWorkPlaceID, b.InstanceID, b.BranchID, workplace, workplace).Scan(&count); err != nil {
			return CareerFact{}, nil, err
		}
		if count != 1 {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "after-work destination needs an existing scoped route")
		}
		var cash, currency, name string
		if err := conn.QueryRowContext(ctx, `SELECT n.asset_account_id,a.currency_id,n.display_name FROM materialized_entities n JOIN accounts a ON a.account_id=n.asset_account_id AND a.owner_id=n.entity_id AND a.account_type='asset' AND a.closed_by_event_id IS NULL WHERE n.entity_id=? AND n.status='active' AND n.population_count=1`, candidate).Scan(&cash, &currency, &name); err != nil {
			return CareerFact{}, nil, classifyMissing(err, "candidate's actual cash account")
		}
		if currency != offer.CurrencyID || currency != org.Fact.Organization.CurrencyID {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "career payroll currency differs from accounts")
		}
		contractID := "employment_" + c.EventID
		employment := &CareerEmploymentFact{ContractID: contractID, EmployeeID: candidate, OrganizationID: record.Fact.OrganizationID, PositionID: offer.PositionID, PositionKey: offer.PositionKey, OccupationID: posting.Fact.Posting.OccupationID, Grade: posting.Fact.Posting.Grade, WorkplaceID: workplace, AfterWorkPlaceID: r.AfterWorkPlaceID, WorkStartHour: offer.WorkStartHour, WorkEndHour: offer.WorkEndHour, StartsOnDay: offer.StartsOnDay, EffectiveFromDay: offer.StartsOnDay, TermVersion: 1, ProbationDays: offer.ProbationDays, DailyWageMinor: offer.DailyWageMinor, CurrencyID: currency, WagePolicy: "guaranteed_daily_v1"}
		employment.Capabilities = append([]string(nil), posting.Fact.Posting.Capabilities...)
		if ok, err := careerWorkdayCompatible(ctx, conn, *employment, offer.StartsOnDay); err != nil || !ok {
			if err == nil {
				err = core.NewError(core.CodeBranchConflict, "initial workday conflicts with existing commitments")
			}
			return CareerFact{}, nil, err
		}
		employment.LifecycleStatus = "probation"
		if employment.ProbationDays == 0 {
			employment.LifecycleStatus = "regular"
		}
		offer.Status, offer.ContractID = "accepted", contractID
		record.Fact.Employment = employment
		record.Fact.AdoptedWorkScheduleIDs, err = careerAdoptableWorkSchedules(ctx, conn, *employment, employment.StartsOnDay, 0)
		if err != nil {
			return CareerFact{}, nil, err
		}
		application.Fact.Application.ApplicationID = offer.ApplicationID
		application.Fact.Application.Status = "accepted"
		record.Fact.Application = application.Fact.Application
		return record.Fact, func() error {
			if _, err := conn.ExecContext(ctx, `INSERT INTO economic_entities(entity_id,entity_kind,display_name,account_id,private_finances) VALUES (?,'employee',?,?,1) ON CONFLICT(entity_id) DO NOTHING`, candidate, name, cash); err != nil {
				return err
			}
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM economic_entities WHERE entity_id=? AND account_id=? AND entity_kind='employee' AND private_finances=1`, candidate, cash).Scan(&count); err != nil || count != 1 {
				return core.NewError(core.CodeProjectionDiverged, "candidate economic registry differs")
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO employment_contracts(contract_id,employer_entity_id,employee_entity_id,employer_account_id,employee_account_id,position_id,gross_wage_minor,currency_id,pay_period_days,starts_on_day,status,definition_event_id) VALUES (?,?,?,?,?,?,?,?,1,?,'active',?)`, contractID, employment.OrganizationID, candidate, org.Fact.Organization.CashAccountID, cash, employment.PositionKey, employment.DailyWageMinor, currency, employment.StartsOnDay, c.EventID); err != nil {
				return err
			}
			if err := createCareerLedger(ctx, conn, *employment, c); err != nil {
				return err
			}
			rulesHash, err := core.HashJSON("career-payroll-v1")
			if err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO scheduler_phases(phase_id,description,ruleset_hash) VALUES (?,'Individual career daily payroll',?) ON CONFLICT(phase_id) DO NOTHING`, careerPayrollPhase, rulesHash); err != nil {
				return err
			}
			if err := queueCareerWorkday(ctx, conn, *employment, employment.StartsOnDay, c.EventID); err != nil {
				return err
			}
			if len(employment.Capabilities) != 0 {
				if err := queueCareerPayrollItem(ctx, conn, c.EventID, employment.StartsOnDay, 0, "career_terms_effective"); err != nil {
					return err
				}
			}
			return queueCareerPayroll(ctx, conn, contractID, employment.StartsOnDay+1)
		}, nil
	})
}

func createCareerLedger(ctx context.Context, conn *sql.Conn, employment CareerEmploymentFact, c careerCommandContext) error {
	prefix := "account_wage_" + employment.ContractID
	for _, account := range []struct{ suffix, owner, kind string }{
		{"expense", employment.OrganizationID, "expense"}, {"payable", employment.OrganizationID, "liability"},
		{"receivable", employment.EmployeeID, "receivable"}, {"income", employment.EmployeeID, "income"},
	} {
		id := prefix + "_" + account.suffix
		if _, err := conn.ExecContext(ctx, `INSERT INTO accounts(account_id,owner_id,currency_id,account_type,overdraft_limit_minor,opened_by_event_id) VALUES (?,?,?,?,0,?)`, id, account.owner, employment.CurrencyID, account.kind, c.EventID); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO account_balances(account_id,balance_minor,projection_version,last_event_sequence) VALUES (?,0,0,?)`, id, c.Sequence); err != nil {
			return err
		}
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO obligation_ledger_accounts(obligation_kind,contract_id,expense_account_id,payable_account_id,receivable_account_id,income_account_id,currency_id,definition_event_id) VALUES ('wage',?,?,?,?,?,?,?)`, employment.ContractID, prefix+"_expense", prefix+"_payable", prefix+"_receivable", prefix+"_income", employment.CurrencyID, c.EventID)
	return err
}

func careerWorkdayCompatible(ctx context.Context, conn *sql.Conn, job CareerEmploymentFact, day int) (bool, error) {
	var conflicts int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND status='active' AND world_time>=? AND world_time<=? AND NOT ((world_time=? AND place_id=? AND activity_code='work') OR (world_time=? AND place_id=? AND activity_code<>'work'))`, job.EmployeeID, careerTime(day, job.WorkStartHour, 0), careerTime(day, job.WorkEndHour, 0), careerTime(day, job.WorkStartHour, 0), job.WorkplaceID, careerTime(day, job.WorkEndHour, 0), job.AfterWorkPlaceID).Scan(&conflicts)
	return conflicts == 0, err
}

func queueCareerWorkday(ctx context.Context, conn *sql.Conn, job CareerEmploymentFact, day int, source string) error {
	return queueCareerWorkdayWithKey(ctx, conn, job, day, source, fmt.Sprintf("career_shift_%s_%d", job.ContractID, day))
}

func queueCareerWorkdayWithKey(ctx context.Context, conn *sql.Conn, job CareerEmploymentFact, day int, source, key string) error {
	leave, err := careerApprovedLeave(ctx, conn, job.ContractID, day)
	if err != nil {
		return err
	}
	if leave != "" {
		return nil
	}
	for _, appointment := range []struct {
		hour            int
		place, activity string
	}{{job.WorkStartHour, job.WorkplaceID, "work"}, {job.WorkEndHour, job.AfterWorkPlaceID, "present"}} {
		at := careerTime(day, appointment.hour, 0)
		var existing int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=? AND status='active'`, job.EmployeeID, at).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 { // Compatibility was checked; retain the existing source.
			continue
		}
		id := fmt.Sprintf("%s_%d", key, appointment.hour)
		payload, err := core.CanonicalJSON(agentSchedulePayload{Kind: "agent_move", Day: day, AgentID: job.EmployeeID, ScheduleID: id, ToPlaceID: appointment.place, ActivityCode: appointment.activity})
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,20,'pending',?)`, "item_"+id, M2DemoInstanceID, M2DemoBranchID, at, m2AgentPhaseID, string(payload)); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?,?,20,?,'active',?)`, id, job.EmployeeID, at, appointment.place, appointment.activity, "item_"+id, source); err != nil {
			return err
		}
	}
	return nil
}
