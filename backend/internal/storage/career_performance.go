package storage

import (
	"context"
	"database/sql"
	"time"

	"corerp.local/backend/internal/core"
)

type CareerPerformanceFact struct {
	ContractID          string   `json:"contract_id"`
	TermsEventID        string   `json:"terms_event_id"`
	ReviewerPrincipalID string   `json:"reviewer_principal_id"`
	EvidenceEventIDs    []string `json:"evidence_event_ids"`
	Assessment          string   `json:"assessment"`
	Reason              string   `json:"reason"`
	AdvisoryNote        string   `json:"advisory_note,omitempty"`
}

type CareerEmploymentChange struct {
	Kind                 string `json:"kind"`
	PreviousTermsEventID string `json:"previous_terms_event_id"`
	PerformanceEventID   string `json:"performance_event_id"`
	ManagerPrincipalID   string `json:"manager_principal_id"`
	Notice               string `json:"notice"`
}

func readCareerContractOrganization(ctx context.Context, conn *sql.Conn, b core.CareerBinding, contract string) (string, error) {
	var organization string
	err := conn.QueryRowContext(ctx, `SELECT c.employer_entity_id FROM employment_contracts c JOIN events e ON e.event_id=c.definition_event_id WHERE c.contract_id=? AND e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded' AND json_extract(e.payload,'$.employment.contract_id')=c.contract_id`, contract, b.InstanceID, b.BranchID).Scan(&organization)
	if err != nil {
		return "", classifyMissing(err, "scoped career employment")
	}
	return organization, nil
}

func currentCareerEmployment(ctx context.Context, conn *sql.Conn, b core.CareerBinding, contract, worldTime string) (CareerEmploymentFact, string, int, error) {
	var empty CareerEmploymentFact
	if b.InstanceID != M2DemoInstanceID || b.BranchID != M2DemoBranchID {
		return empty, "", 0, core.NewError(core.CodeInvalidArgument, "career lifecycle requires supported M2 calendar")
	}
	if _, err := readCareerContractOrganization(ctx, conn, b, contract); err != nil {
		return empty, "", 0, err
	}
	var status string
	var starts int
	if err := conn.QueryRowContext(ctx, `SELECT status,starts_on_day FROM employment_contracts WHERE contract_id=?`, contract).Scan(&status, &starts); err != nil {
		return empty, "", 0, err
	}
	now, err := time.Parse(time.RFC3339Nano, worldTime)
	if err != nil {
		return empty, "", 0, err
	}
	day := int(now.Sub(time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
	if status != "active" || day < starts {
		return empty, "", 0, core.NewError(core.CodeBranchConflict, "employment is not currently effective and active")
	}
	job, source, err := readCareerEmploymentTerms(ctx, conn, contract, day)
	return job, source, day, err
}

func (s *Store) RecordCareerPerformance(ctx context.Context, r core.CareerPerformanceRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "RecordCareerPerformance", r, func(conn *sql.Conn) error {
		org, err := readCareerContractOrganization(ctx, conn, b, r.ContractID)
		if err != nil {
			return err
		}
		return authorizeCareerManager(ctx, conn, b, org)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "performance", r.ReviewID); err != nil {
			return CareerFact{}, nil, err
		}
		job, source, _, err := currentCareerEmployment(ctx, conn, b, r.ContractID, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		for _, id := range r.EvidenceEventIDs {
			var count int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='WageObligationAccrued' AND json_extract(payload,'$.attendance.contract_id')=? AND json_extract(payload,'$.attendance.employee_id')=?`, id, b.InstanceID, b.BranchID, job.ContractID, job.EmployeeID).Scan(&count); err != nil {
				return CareerFact{}, nil, err
			}
			if count != 1 {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "performance evidence must be completed work for this employment")
			}
		}
		return CareerFact{Kind: "performance", RecordID: r.ReviewID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, Performance: &CareerPerformanceFact{ContractID: job.ContractID, TermsEventID: source, ReviewerPrincipalID: b.PrincipalID, EvidenceEventIDs: append([]string(nil), r.EvidenceEventIDs...), Assessment: r.Assessment, Reason: r.Reason, AdvisoryNote: r.AdvisoryNote}}, nil, nil
	})
}

func (s *Store) RegularizeCareerEmployment(ctx context.Context, r core.CareerRegularizationRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "RegularizeCareerEmployment", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordManager(ctx, conn, b, "performance", r.ReviewID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		review, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "performance", r.ReviewID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if review.Fact.Performance == nil || review.Fact.Performance.Assessment != "meets_expectations" {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "regularization requires a favorable performance review")
		}
		performance := review.Fact.Performance
		job, source, day, err := currentCareerEmployment(ctx, conn, b, performance.ContractID, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if job.LifecycleStatus != "probation" || day < job.StartsOnDay+job.ProbationDays {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "agreed probation period is not complete or employment is already regular")
		}
		if source != performance.TermsEventID {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "performance review refers to superseded employment terms")
		}
		var latest string
		if err := conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='performance' AND json_extract(payload,'$.performance.contract_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, job.ContractID).Scan(&latest); err != nil {
			return CareerFact{}, nil, err
		}
		if latest != review.EventID {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "performance review was superseded")
		}
		reviewer := b
		reviewer.PrincipalID = performance.ReviewerPrincipalID
		if err := authorizeCareerManager(ctx, conn, reviewer, job.OrganizationID); err != nil {
			return CareerFact{}, nil, err
		}
		var pending int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.employment.contract_id')=? AND json_extract(payload,'$.employment.effective_from_day')>?`, b.InstanceID, b.BranchID, job.ContractID, day).Scan(&pending); err != nil {
			return CareerFact{}, nil, err
		}
		if pending != 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "resolve pending effective terms before regularization")
		}
		job.TermVersion++
		job.EffectiveFromDay, job.LifecycleStatus = day, "regular"
		return CareerFact{Kind: "employment", RecordID: job.ContractID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, Employment: &job, EmploymentChange: &CareerEmploymentChange{Kind: "regularized", PreviousTermsEventID: source, PerformanceEventID: review.EventID, ManagerPrincipalID: b.PrincipalID, Notice: r.Notice}}, nil, nil
	})
}
