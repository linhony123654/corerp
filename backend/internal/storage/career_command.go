package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

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
	AggregateExit          *CareerAggregateExitFact      `json:"aggregate_exit,omitempty"`
	Version                string                        `json:"version"`
	Announcement           *CareerAnnouncementFact       `json:"announcement,omitempty"`
	Exit                   *CareerExitFact               `json:"exit,omitempty"`
	Kind                   string                        `json:"kind"`
	RecordID               string                        `json:"record_id"`
	OrganizationID         string                        `json:"organization_id"`
	CandidateID            string                        `json:"candidate_id,omitempty"`
	Organization           *CareerOrganizationFact       `json:"organization,omitempty"`
	GradeScale             *core.CareerGradeScale        `json:"grade_scale,omitempty"`
	PositionChange         *CareerPositionChangeFact     `json:"position_change,omitempty"`
	PositionAssessment     *CareerPositionAssessment     `json:"position_assessment,omitempty"`
	Posting                *core.CareerPostingDefinition `json:"posting,omitempty"`
	Application            *CareerApplicationFact        `json:"application,omitempty"`
	Interview              *CareerInterviewFact          `json:"interview,omitempty"`
	Evaluation             *CareerEvaluationFact         `json:"evaluation,omitempty"`
	Offer                  *CareerOfferFact              `json:"offer,omitempty"`
	Referral               *CareerReferralFact           `json:"referral,omitempty"`
	Employment             *CareerEmploymentFact         `json:"employment,omitempty"`
	Performance            *CareerPerformanceFact        `json:"performance,omitempty"`
	EmploymentChange       *CareerEmploymentChange       `json:"employment_change,omitempty"`
	Leave                  *CareerLeaveFact              `json:"leave,omitempty"`
	Overtime               *CareerOvertimeFact           `json:"overtime,omitempty"`
	AdoptedWorkScheduleIDs []string                      `json:"adopted_work_schedule_ids,omitempty"`
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
	var empty CareerRecord
	if err := b.Validate(); err != nil {
		return empty, err
	}
	hash, err := core.HashJSON(request)
	if err != nil {
		return empty, err
	}
	key, err := core.HashJSON([]string{b.PrincipalID, b.IdempotencyKey})
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if err := authorize(tx.conn); err != nil {
		return empty, err
	}
	var previous CareerRecord
	var oldHash, raw string
	err = tx.conn.QueryRowContext(ctx, `SELECT c.request_hash,e.event_id,e.event_sequence,e.world_time,e.payload FROM commands c JOIN event_batches b ON b.command_id=c.command_id JOIN events e ON e.batch_id=b.batch_id
	 WHERE c.instance_id=? AND c.branch_id=? AND c.command_type=? AND c.idempotency_key=? AND c.status='committed'`, b.InstanceID, b.BranchID, commandType, key).Scan(&oldHash, &previous.EventID, &previous.EventSequence, &previous.WorldTime, &raw)
	if err == nil {
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "career retry differs")
		}
		if err := json.Unmarshal([]byte(raw), &previous.Fact); err != nil {
			return empty, err
		}
		previous.Replayed = true
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	var head int64
	var worldTime, epoch string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, b.InstanceID, b.BranchID).Scan(&head, &worldTime); err != nil {
		return empty, classifyMissing(err, "career world")
	}
	if head != b.ExpectedHead {
		return empty, core.NewError(core.CodeBranchConflict, "career expected head differs")
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+(SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, b.InstanceID, b.BranchID, b.InstanceID, b.BranchID).Scan(&pending); err != nil {
		return empty, err
	}
	if pending != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "finish active RP action before career command")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, b.InstanceID, b.BranchID, worldTime); err != nil {
		return empty, err
	}
	id, err := core.HashJSON([]string{b.InstanceID, b.BranchID, commandType, key})
	if err != nil {
		return empty, err
	}
	suffix := id[7:]
	commandID, batchID, attemptID := "cmd_career_"+suffix, "batch_career_"+suffix, "attempt_career_"+suffix
	c := careerCommandContext{EventID: "event_career_" + suffix, Sequence: head + 1, WorldTime: worldTime}
	fact, apply, err := prepare(tx.conn, c)
	if err != nil {
		return empty, err
	}
	fact.Version = "corerp.career.v1"
	encoded, err := core.CanonicalJSON(fact)
	if err != nil {
		return empty, err
	}
	batchHash, err := core.HashJSON(struct {
		RequestHash string     `json:"request_hash"`
		Sequence    int64      `json:"sequence"`
		WorldTime   string     `json:"world_time"`
		Fact        CareerFact `json:"fact"`
	}{hash, c.Sequence, worldTime, fact})
	if err != nil {
		return empty, err
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, b.InstanceID, b.BranchID, c.Sequence, c.Sequence).Scan(&epoch); err != nil {
		return empty, err
	}
	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	for _, st := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,?,?,?,?,?, '{"authorization":"scoped-career-v1"}','pending',?)`, []any{commandID, b.InstanceID, b.BranchID, commandType, key, hash, head, b.PrincipalID, nowText}},
		{`INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-career',?,?,?)`, []any{commandID, attemptID, now.Add(30 * time.Second).Format(time.RFC3339Nano), hash, nowText}},
		{`INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, b.InstanceID, b.BranchID, epoch, head, c.Sequence, c.Sequence, worldTime, batchHash, nowText}},
		{`INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,'RPCareerFactRecorded',?,?,?)`, []any{c.EventID, batchID, b.InstanceID, b.BranchID, c.Sequence, b.PrincipalID, worldTime, string(encoded)}},
	} {
		if _, err := tx.conn.ExecContext(ctx, st.query, st.args...); err != nil {
			return empty, core.WrapError(core.CodeStorageFailure, "record career command", err)
		}
	}
	if apply != nil {
		if err := apply(); err != nil {
			return empty, err
		}
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", c.EventID, commandID, attemptID, worldTime, nowText, encoded); err != nil {
		return empty, err
	}
	// No public Outbox: candidates' statements/assessments are not world facts
	// known to all clients. Public discovery is a separate filtered read.
	for _, st := range []struct {
		query string
		args  []any
	}{
		{`UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, []any{c.Sequence, b.InstanceID, b.BranchID, worldTime}},
		{`UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, []any{c.Sequence, b.InstanceID, b.BranchID, head}},
		{`UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, []any{commandID}},
		{`UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE attempt_id=? AND status='ready'`, []any{nowText, attemptID}},
	} {
		if err := execAgentOne(ctx, tx.conn, "career compare-and-swap", st.query, st.args...); err != nil {
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
	return CareerRecord{EventID: c.EventID, EventSequence: c.Sequence, WorldTime: worldTime, Fact: fact}, nil
}

func readCareerRecord(ctx context.Context, conn *sql.Conn, instanceID, branchID, kind, id string) (CareerRecord, error) {
	var record CareerRecord
	var raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,event_sequence,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND ((json_extract(payload,'$.kind')=? AND json_extract(payload,'$.record_id')=?) OR (?='application' AND json_extract(payload,'$.application.application_id')=?) OR (?='employment' AND json_extract(payload,'$.employment.contract_id')=?)) ORDER BY event_sequence DESC LIMIT 1`, instanceID, branchID, kind, id, kind, id, kind, id).Scan(&record.EventID, &record.EventSequence, &record.WorldTime, &raw)
	if err != nil {
		return record, classifyMissing(err, "career "+kind)
	}
	if err := json.Unmarshal([]byte(raw), &record.Fact); err != nil {
		return CareerRecord{}, core.WrapError(core.CodeProjectionDiverged, "decode career fact", err)
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
