package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

const organizationReviewPhase = "organization_review"

func organizationReviewItem(source, at string, policy core.OrganizationAgencyPolicy) (SchedulerItem, scheduledPayload, error) {
	when, err := time.Parse(time.RFC3339Nano, at)
	if err != nil || policy.Validate() != nil {
		return SchedulerItem{}, scheduledPayload{}, core.NewError(core.CodeProjectionDiverged, "invalid organization schedule source")
	}
	due := when.Add(time.Duration(policy.ReviewFrequencyHours) * time.Hour).UTC()
	day := int(due.Sub(time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
	if day < 0 {
		return SchedulerItem{}, scheduledPayload{}, core.NewError(core.CodeProjectionDiverged, "organization review predates world epoch")
	}
	payload := scheduledPayload{Kind: organizationReviewPhase, Day: day, SubjectID: source}
	raw, err := core.CanonicalJSON(payload)
	if err != nil {
		return SchedulerItem{}, payload, err
	}
	return SchedulerItem{SchedulerItemID: "sched_org_review_" + source, WorldTime: due.Format(time.RFC3339Nano), PhaseID: organizationReviewPhase, Status: "pending", Payload: string(raw)}, payload, nil
}

func queueOrganizationReview(ctx context.Context, conn *sql.Conn, b core.CareerBinding, source, at string, policy core.OrganizationAgencyPolicy) error {
	if !policy.AutomaticReview || policy.Status != "active" {
		return nil
	}
	item, _, err := organizationReviewItem(source, at, policy)
	if err != nil {
		return err
	}
	hash, err := core.HashJSON("organization-review-v1")
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO scheduler_phases(phase_id,description,ruleset_hash) VALUES (?,'Manager-authorized periodic organization review',?)`, organizationReviewPhase, hash); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,0,'pending',?)`, item.SchedulerItemID, b.InstanceID, b.BranchID, item.WorldTime, item.PhaseID, item.Payload)
	return err
}

func organizationScheduleSource(ctx context.Context, q replayQuerier, instance, branch, source string) (core.OrganizationAgencyPolicy, string, error) {
	var raw, at string
	if err := q.QueryRowContext(ctx, `SELECT payload,world_time FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded'`, source, instance, branch).Scan(&raw, &at); err != nil {
		return core.OrganizationAgencyPolicy{}, "", err
	}
	var f CareerFact
	if json.Unmarshal([]byte(raw), &f) != nil || f.Version != "corerp.career.v1" {
		return core.OrganizationAgencyPolicy{}, "", core.NewError(core.CodeProjectionDiverged, "invalid organization queue source")
	}
	if f.Kind == "organization_review" && f.OrganizationReview != nil {
		if err := q.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='agency_policy' AND event_sequence<?`, f.OrganizationReview.PolicySourceEventID, instance, branch, f.OrganizationReview.EventSequence).Scan(&raw); err != nil {
			return core.OrganizationAgencyPolicy{}, "", err
		}
		var policyFact CareerFact
		if json.Unmarshal([]byte(raw), &policyFact) != nil || policyFact.OrganizationID != f.OrganizationID {
			return core.OrganizationAgencyPolicy{}, "", core.NewError(core.CodeProjectionDiverged, "review policy lineage differs")
		}
		f = policyFact
	}
	if f.Kind != "agency_policy" || f.AgencyPolicy == nil || f.AgencyPolicy.Validate() != nil || f.OrganizationID != f.AgencyPolicy.OrganizationID {
		return core.OrganizationAgencyPolicy{}, "", core.NewError(core.CodeProjectionDiverged, "organization queue has no valid policy")
	}
	return *f.AgencyPolicy, at, nil
}

type organizationReviewSkipped struct {
	SourceEventID string `json:"source_event_id"`
	Reason        string `json:"reason"`
}

func (s *Store) executeOrganizationReview(ctx context.Context, tx *immediateTx, item SchedulerItem) error {
	instance, branch, err := recordedSchedulerScope(ctx, tx.conn, item)
	if err != nil {
		return err
	}
	var payload scheduledPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return err
	}
	policy, at, err := organizationScheduleSource(ctx, tx.conn, instance, branch, payload.SubjectID)
	if err != nil {
		return err
	}
	want, expectedPayload, err := organizationReviewItem(payload.SubjectID, at, policy)
	if err != nil {
		return err
	}
	if !policy.AutomaticReview || policy.Status != "active" || want != item || payload != expectedPayload {
		return core.NewError(core.CodeProjectionDiverged, "organization queue differs from source")
	}
	var latest string
	if err := tx.conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind') IN ('agency_policy','organization_review') AND json_extract(payload,'$.organization_id')=? ORDER BY event_sequence DESC LIMIT 1`, instance, branch, policy.OrganizationID).Scan(&latest); err != nil {
		return err
	}
	b := core.CareerBinding{PrincipalID: policy.ManagerPrincipalID, InstanceID: instance, BranchID: branch}
	reason := ""
	if latest != payload.SubjectID {
		reason = "superseded_source"
	}
	if reason == "" {
		if err := authorizeCareerManager(ctx, tx.conn, b, policy.OrganizationID); err != nil {
			if !core.HasCode(err, core.CodeUnauthorized) {
				return err
			}
			reason = "manager_authority_unavailable"
		}
	}
	mutation := scheduledMutation{Private: true}
	if reason != "" {
		mutation.EventType = "OrganizationReviewSkipped"
		mutation.EventPayload = organizationReviewSkipped{payload.SubjectID, reason}
	} else {
		var head int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, instance, branch).Scan(&head); err != nil {
			return err
		}
		fact, apply, err := s.prepareOrganizationReview(ctx, tx.conn, core.OrganizationReviewRequest{Binding: b, OrganizationID: policy.OrganizationID, PolicyID: policy.PolicyID}, careerCommandContext{EventID: "event_" + item.SchedulerItemID, Sequence: head + 1, WorldTime: item.WorldTime})
		if err != nil {
			return err
		}
		fact.Version = "corerp.career.v1"
		fact.OrganizationReview.ScheduleSourceEventID = payload.SubjectID
		mutation.EventType = "RPCareerFactRecorded"
		mutation.EventPayload = fact
		mutation.ApplyDomainRows = func(context.Context, *sql.Conn, string, int64) error { return apply() }
	}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, instance, branch); err != nil {
		return err
	}
	if s.beforeCommit != nil {
		return s.beforeCommit()
	}
	return nil
}
