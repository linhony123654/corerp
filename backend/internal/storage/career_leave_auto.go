package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

const careerLeaveReviewPhase = "career_leave_review"

type CareerLeaveAutoReview struct {
	PolicySourceEventID string `json:"policy_source_event_id"`
	ManagerPrincipalID  string `json:"manager_principal_id"`
	WorldTime           string `json:"world_time"`
	SchedulerItemID     string `json:"scheduler_item_id"`
}

func planCareerLeaveAutoReview(ctx context.Context, conn *sql.Conn, b core.CareerBinding, job CareerEmploymentFact, source, at string, first int) (*CareerLeaveAutoReview, error) {
	org, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "organization", job.OrganizationID)
	if err != nil {
		return nil, err
	}
	if org.Fact.Organization == nil {
		return nil, core.NewError(core.CodeProjectionDiverged, "missing organization definition")
	}
	definition := org.Fact.Organization.Definition
	policy := definition.LeaveReviewPolicy
	if policy == nil || policy.AutoReviewDelayMinutes == 0 {
		return nil, nil
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	when, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return nil, err
	}
	due := when.Add(time.Duration(policy.AutoReviewDelayMinutes) * time.Minute).UTC().Format(time.RFC3339)
	// A late request remains available for explicit managerial review; never
	// queue an approval at or after the originally owned shift deadline.
	if due >= careerTime(first, job.WorkStartHour, 0) {
		return nil, nil
	}
	return &CareerLeaveAutoReview{org.EventID, definition.ManagerPrincipalID, due, "career_leave_review_" + source}, nil
}

func careerLeaveReviewPayload(source string) scheduledPayload {
	return scheduledPayload{Kind: "career_leave_review", SubjectID: source}
}

func queueCareerLeaveReview(ctx context.Context, conn *sql.Conn, b core.CareerBinding, source string, plan CareerLeaveAutoReview) error {
	payload, err := core.CanonicalJSON(careerLeaveReviewPayload(source))
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,0,'pending',?)`, plan.SchedulerItemID, b.InstanceID, b.BranchID, plan.WorldTime, careerLeaveReviewPhase, string(payload))
	return err
}

func (s *Store) executeCareerLeaveAutoReview(ctx context.Context, tx *immediateTx, item SchedulerItem) error {
	var payload scheduledPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return err
	}
	b := core.CareerBinding{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID}
	var raw, requestedAt string
	if err := tx.conn.QueryRowContext(ctx, `SELECT payload,world_time FROM events WHERE instance_id=? AND branch_id=? AND event_id=? AND event_type='RPCareerFactRecorded'`, b.InstanceID, b.BranchID, payload.SubjectID).Scan(&raw, &requestedAt); err != nil {
		return err
	}
	var requested CareerFact
	if err := json.Unmarshal([]byte(raw), &requested); err != nil {
		return err
	}
	if requested.Kind != "leave" || requested.Leave == nil || requested.Leave.Status != "requested" || requested.Leave.RequestEventID != payload.SubjectID || requested.Leave.AutoReview == nil {
		return core.NewError(core.CodeProjectionDiverged, "invalid automatic leave request source")
	}
	leave := requested.Leave
	plan := *leave.AutoReview
	// Validate against the original request's accepted terms, not later terms
	// that may legitimately change before review. The source Event carries them.
	var termsRaw string
	if err := tx.conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=?`, leave.TermsEventID, b.InstanceID, b.BranchID).Scan(&termsRaw); err != nil {
		return err
	}
	var terms CareerFact
	if err := json.Unmarshal([]byte(termsRaw), &terms); err != nil {
		return err
	}
	if terms.Employment == nil || terms.Employment.ContractID != leave.ContractID {
		return core.NewError(core.CodeProjectionDiverged, "invalid original leave terms")
	}
	job := *terms.Employment
	expected, err := planCareerLeaveAutoReview(ctx, tx.conn, b, job, payload.SubjectID, requestedAt, leave.StartDay)
	canonical, hashErr := core.CanonicalJSON(careerLeaveReviewPayload(payload.SubjectID))
	if err != nil {
		return err
	}
	if hashErr != nil {
		return hashErr
	}
	if expected == nil || *expected != plan || item.SchedulerItemID != plan.SchedulerItemID || item.WorldTime != plan.WorldTime || item.PhaseID != careerLeaveReviewPhase || item.DeclaredPriority != 0 || item.Payload != string(canonical) {
		return core.NewError(core.CodeProjectionDiverged, "automatic leave queue differs from accepted source")
	}
	b.PrincipalID = plan.ManagerPrincipalID
	reason := ""
	if err := authorizeCareerManager(ctx, tx.conn, b, requested.OrganizationID); err != nil {
		if !core.HasCode(err, core.CodeUnauthorized) {
			return err
		}
		reason = "manager_authority_unavailable"
	}
	var fact CareerFact
	var apply func() error
	if reason == "" {
		fact, apply, err = prepareCareerLeaveReview(ctx, tx.conn, b, requested.RecordID, "consider", "Reviewed under the organization's declared leave policy.", item.WorldTime)
		if err != nil {
			if !core.HasCode(err, core.CodeBranchConflict) {
				return err
			}
			reason = "request_no_longer_reviewable"
		}
	}
	mutation := scheduledMutation{Private: true}
	if reason != "" {
		mutation.EventType = "CareerLeaveAutoReviewSkipped"
		mutation.EventPayload = struct {
			RequestEventID string `json:"request_event_id"`
			Reason         string `json:"reason"`
		}{payload.SubjectID, reason}
	} else {
		fact.Version = "corerp.career.v1"
		mutation.EventType, mutation.EventPayload = "RPCareerFactRecorded", fact
		if apply != nil {
			mutation.ApplyDomainRows = func(context.Context, *sql.Conn, string, int64) error { return apply() }
		}
	}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, b.InstanceID, b.BranchID); err != nil {
		return err
	}
	if s.beforeCommit != nil {
		return s.beforeCommit()
	}
	return nil
}
