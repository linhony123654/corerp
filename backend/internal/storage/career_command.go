package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

const careerManageCapability = "world.career.manage"

type CareerOrganizationFact struct {
	Definition         core.CareerOrganizationDefinition `json:"definition"`
	CashAccountID      string                            `json:"cash_account_id"`
	CurrencyID         string                            `json:"currency_id"`
	SourceActorEventID string                            `json:"source_actor_event_id"`
}

type CareerApplicationFact struct {
	ApplicationID     string `json:"application_id,omitempty"`
	PositionID        string `json:"position_id"`
	CandidateID       string `json:"candidate_id"`
	Statement         string `json:"statement"`
	PostingEventID    string `json:"posting_event_id"`
	CandidateSourceID string `json:"candidate_source_event_id"`
	Status            string `json:"status"`
	ReferralEventID   string `json:"referral_event_id,omitempty"`
}

// Each record is an immutable, typed Event snapshot. A mutable recruitment
// table is not a competing authority. All reads retain scope and source Event.
type CareerFact struct {
	AggregateExit          *CareerAggregateExitFact       `json:"aggregate_exit,omitempty"`
	Version                string                         `json:"version"`
	Announcement           *CareerAnnouncementFact        `json:"announcement,omitempty"`
	Exit                   *CareerExitFact                `json:"exit,omitempty"`
	Kind                   string                         `json:"kind"`
	RecordID               string                         `json:"record_id"`
	OrganizationID         string                         `json:"organization_id"`
	CandidateID            string                         `json:"candidate_id,omitempty"`
	Organization           *CareerOrganizationFact        `json:"organization,omitempty"`
	GradeScale             *core.CareerGradeScale         `json:"grade_scale,omitempty"`
	PositionChange         *CareerPositionChangeFact      `json:"position_change,omitempty"`
	PositionAssessment     *CareerPositionAssessment      `json:"position_assessment,omitempty"`
	Posting                *core.CareerPostingDefinition  `json:"posting,omitempty"`
	Application            *CareerApplicationFact         `json:"application,omitempty"`
	Interview              *CareerInterviewFact           `json:"interview,omitempty"`
	Evaluation             *CareerEvaluationFact          `json:"evaluation,omitempty"`
	Offer                  *CareerOfferFact               `json:"offer,omitempty"`
	Referral               *CareerReferralFact            `json:"referral,omitempty"`
	Employment             *CareerEmploymentFact          `json:"employment,omitempty"`
	Performance            *CareerPerformanceFact         `json:"performance,omitempty"`
	EmploymentChange       *CareerEmploymentChange        `json:"employment_change,omitempty"`
	Leave                  *CareerLeaveFact               `json:"leave,omitempty"`
	Overtime               *CareerOvertimeFact            `json:"overtime,omitempty"`
	AdoptedWorkScheduleIDs []string                       `json:"adopted_work_schedule_ids,omitempty"`
	AgencyPolicy           *core.OrganizationAgencyPolicy `json:"agency_policy,omitempty"`
	OrganizationReview     *core.OrganizationReviewResult `json:"organization_review,omitempty"`
}

type CareerRecord struct {
	EventID       string     `json:"event_id"`
	EventSequence int64      `json:"event_sequence"`
	WorldTime     string     `json:"world_time"`
	Fact          CareerFact `json:"fact"`
	Replayed      bool       `json:"replayed"`
}

type careerCommandContext struct {
	EventID   string
	Sequence  int64
	WorldTime string
}

type careerPrepare func(*sql.Conn, careerCommandContext) (CareerFact, func() error, error)

// The caller validates its typed request and supplies a narrow authorization
// check. Authorization is repeated even for retries; revoked managers cannot
// retrieve private command results merely by guessing an old idempotency key.
func (s *Store) executeCareerCommand(ctx context.Context, b core.CareerBinding, commandType string, request any, authorize func(*sql.Conn) error, prepare careerPrepare) (CareerRecord, error) {
	record, err := executePrivateFactCommand(s, ctx, b, commandType, request, privateFactDomain{"career", "RPCareerFactRecorded", `{"authorization":"scoped-career-v1"}`}, authorize, func(conn *sql.Conn, c privateFactContext) (CareerFact, func() error, error) {
		fact, apply, err := prepare(conn, careerCommandContext(c))
		fact.Version = "corerp.career.v1"
		return fact, apply, err
	})
	return CareerRecord(record), err
}

func readCareerRecord(ctx context.Context, conn *sql.Conn, instanceID, branchID, kind, id string) (CareerRecord, error) {
	var record CareerRecord
	var raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,event_sequence,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND ((json_extract(payload,'$.kind')=? AND json_extract(payload,'$.record_id')=?) OR (?='application' AND json_extract(payload,'$.application.application_id')=?) OR (?='employment' AND json_extract(payload,'$.employment.contract_id')=?) OR (?='posting' AND json_extract(payload,'$.kind')='organization_review' AND json_extract(payload,'$.posting.position_id')=?)) ORDER BY event_sequence DESC LIMIT 1`, instanceID, branchID, kind, id, kind, id, kind, id, kind, id).Scan(&record.EventID, &record.EventSequence, &record.WorldTime, &raw)
	if err != nil {
		return record, classifyMissing(err, "career "+kind)
	}
	if err := json.Unmarshal([]byte(raw), &record.Fact); err != nil {
		return CareerRecord{}, core.WrapError(core.CodeProjectionDiverged, "decode career fact", err)
	}
	if kind == "posting" && record.Fact.Kind != kind {
		if record.Fact.Posting == nil {
			return CareerRecord{}, core.NewError(core.CodeProjectionDiverged, "posting outcome lacks terms")
		}
		f := record.Fact
		record.Fact = CareerFact{Version: f.Version, Kind: kind, RecordID: id, OrganizationID: f.OrganizationID, Posting: f.Posting}
	}
	if kind == "application" && record.Fact.Kind != kind {
		if record.Fact.Application == nil {
			return CareerRecord{}, core.NewError(core.CodeProjectionDiverged, "application outcome lacks its source snapshot")
		}
		f := record.Fact
		record.Fact = CareerFact{Version: f.Version, Kind: kind, RecordID: id, OrganizationID: f.OrganizationID, CandidateID: careerRecordCandidate(f), Application: f.Application}
	}
	if kind == "employment" && record.Fact.Kind != kind {
		if record.Fact.Employment == nil {
			return CareerRecord{}, core.NewError(core.CodeProjectionDiverged, "employment lacks source terms")
		}
		f := record.Fact
		record.Fact = CareerFact{Version: f.Version, Kind: kind, RecordID: id, OrganizationID: f.OrganizationID, CandidateID: f.Employment.EmployeeID, Employment: f.Employment, EmploymentChange: f.EmploymentChange}
	}
	return record, nil
}

func requireNewCareerRecord(ctx context.Context, conn *sql.Conn, b core.CareerBinding, kind, id string) error {
	_, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, kind, id)
	if core.HasCode(err, core.CodeNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return core.NewError(core.CodeBranchConflict, "career record already exists")
}

func authorizeCareerManager(ctx context.Context, conn *sql.Conn, b core.CareerBinding, orgID string) error {
	var allowed int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g JOIN principals p ON p.principal_id=g.principal_id WHERE g.principal_id=? AND p.status='active' AND g.capability_id IN (?,?) AND g.instance_id=? AND g.branch_id=? AND g.subject_id=? AND g.status='active'`, b.PrincipalID, careerManageCapability, core.CareerPositionManageCapability, b.InstanceID, b.BranchID, orgID).Scan(&allowed)
	if err != nil {
		return err
	}
	if allowed == 0 {
		return core.NewError(core.CodeUnauthorized, "career management requires this organization's active grant")
	}
	return nil
}

func authorizeCareerCandidate(ctx context.Context, conn *sql.Conn, b core.CareerBinding, candidateID string) error {
	var allowed int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles a JOIN principals p ON p.principal_id=a.principal_id WHERE a.agent_id=? AND a.instance_id=? AND a.branch_id=? AND a.principal_id=? AND a.status='active' AND p.status='active' AND p.principal_type='agent'`, candidateID, b.InstanceID, b.BranchID, b.PrincipalID).Scan(&allowed)
	if err != nil {
		return err
	}
	if allowed != 1 {
		if err := authorizeRPControl(ctx, conn, b.PrincipalID, b.InstanceID, b.BranchID, candidateID); err != nil {
			return err
		}
	}
	return validateRPBinding(ctx, conn, b.InstanceID, b.BranchID, candidateID)
}
