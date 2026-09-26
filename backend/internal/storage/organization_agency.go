package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// DefineOrganizationAgencyPolicy establishes the operational review rules and
// budget thresholds for an organization. Requires manager authorization.
func (s *Store) DefineOrganizationAgencyPolicy(ctx context.Context, r core.OrganizationAgencyPolicyRequest) (core.OrganizationAgencyPolicy, error) {
	if err := r.Validate(); err != nil {
		return core.OrganizationAgencyPolicy{}, err
	}
	b, p := r.Binding, r.Policy
	record, err := s.executeCareerCommand(ctx, b, "DefineOrganizationAgencyPolicy", r, func(conn *sql.Conn) error {
		return authorizeCareerManager(ctx, conn, b, p.OrganizationID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		// Ensure organization exists
		organization, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "organization", p.OrganizationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		// Ensure target position exists
		posting, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", p.TargetPositionID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if organization.Fact.Organization == nil || organization.Fact.Organization.Definition.ManagerPrincipalID != p.ManagerPrincipalID ||
			posting.Fact.Posting == nil || posting.Fact.Posting.OrganizationID != p.OrganizationID {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "agency policy must bind its source manager and own posting")
		}
		prior, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "agency_policy", p.OrganizationID)
		if err != nil && !core.HasCode(err, core.CodeNotFound) {
			return CareerFact{}, nil, err
		}
		if err == nil && (prior.Fact.AgencyPolicy == nil || prior.Fact.AgencyPolicy.PolicyID != p.PolicyID) {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "policy revisions must preserve policy identity")
		}

		fact := CareerFact{
			Kind:           "agency_policy",
			RecordID:       p.OrganizationID,
			OrganizationID: p.OrganizationID,
			AgencyPolicy:   &p,
		}

		projection := func() error {
			_, err := conn.ExecContext(ctx, `INSERT INTO organization_agency_policies(
				policy_id, instance_id, branch_id, organization_id, manager_principal_id,
				review_frequency_hours, reserve_target_minor, hiring_threshold_minor, freeze_threshold_minor,
				target_position_id, default_capacity, status, definition_event_id, automatic_review
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(instance_id, branch_id, organization_id) DO UPDATE SET
				policy_id=excluded.policy_id,
				manager_principal_id=excluded.manager_principal_id,
				review_frequency_hours=excluded.review_frequency_hours,
				reserve_target_minor=excluded.reserve_target_minor,
				hiring_threshold_minor=excluded.hiring_threshold_minor,
				freeze_threshold_minor=excluded.freeze_threshold_minor,
				target_position_id=excluded.target_position_id,
				default_capacity=excluded.default_capacity,
				status=excluded.status,
				definition_event_id=excluded.definition_event_id, automatic_review=excluded.automatic_review`,
				p.PolicyID, b.InstanceID, b.BranchID, p.OrganizationID, p.ManagerPrincipalID,
				p.ReviewFrequencyHours, p.ReserveTargetMinor, p.HiringThresholdMinor, p.FreezeThresholdMinor,
				p.TargetPositionID, p.DefaultCapacity, p.Status, c.EventID, p.AutomaticReview)
			if err != nil {
				return err
			}
			return queueOrganizationReview(ctx, conn, b, c.EventID, c.WorldTime, p)
		}

		return fact, projection, nil
	})
	if err != nil {
		return core.OrganizationAgencyPolicy{}, err
	}
	if record.Fact.AgencyPolicy == nil {
		return core.OrganizationAgencyPolicy{}, core.NewError(core.CodeProjectionDiverged, "agency policy fact missing")
	}
	return *record.Fact.AgencyPolicy, nil
}

func readOrganizationAgencyPolicyWithConn(ctx context.Context, conn *sql.Conn, instanceID, branchID, orgID string) (core.OrganizationAgencyPolicy, error) {
	record, err := readCareerRecord(ctx, conn, instanceID, branchID, "agency_policy", orgID)
	if err != nil {
		return core.OrganizationAgencyPolicy{}, err
	}
	p := record.Fact.AgencyPolicy
	if p == nil || p.Validate() != nil || p.OrganizationID != orgID || record.Fact.OrganizationID != orgID {
		return core.OrganizationAgencyPolicy{}, core.NewError(core.CodeProjectionDiverged, "agency policy source differs")
	}
	return *p, nil
}

// ReadOrganizationAgencyPolicy reads the active agency policy for an organization.
func (s *Store) ReadOrganizationAgencyPolicy(ctx context.Context, principalID, instanceID, branchID, orgID string) (core.OrganizationAgencyPolicy, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return core.OrganizationAgencyPolicy{}, err
	}
	defer conn.Close()
	if err := authorizeCareerManager(ctx, conn, core.CareerBinding{PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID}, orgID); err != nil {
		return core.OrganizationAgencyPolicy{}, err
	}
	return readOrganizationAgencyPolicyWithConn(ctx, conn, instanceID, branchID, orgID)
}

// collectOrganizationEvidence gathers objective financial and staffing metrics.
// No private employee personal chats or health details are accessed.
func (s *Store) collectOrganizationEvidence(ctx context.Context, conn *sql.Conn, instanceID, branchID, orgID, positionID, worldTime string) (core.OrganizationEvidence, error) {
	orgRec, err := readCareerRecord(ctx, conn, instanceID, branchID, "organization", orgID)
	if err != nil {
		return core.OrganizationEvidence{}, err
	}
	if orgRec.Fact.Organization == nil {
		return core.OrganizationEvidence{}, core.NewError(core.CodeProjectionDiverged, "organization record missing definition")
	}
	cashAccountID := orgRec.Fact.Organization.CashAccountID
	if err := verifyScopedAccountProjection(ctx, conn, instanceID, branchID, cashAccountID, orgRec.Fact.Organization.CurrencyID); err != nil {
		return core.OrganizationEvidence{}, err
	}

	var cashBalance int64
	if err := conn.QueryRowContext(ctx, `SELECT balance_minor FROM account_balances WHERE account_id=?`, cashAccountID).Scan(&cashBalance); err != nil {
		return core.OrganizationEvidence{}, err
	}

	var head int64
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, instanceID, branchID).Scan(&head); err != nil {
		return core.OrganizationEvidence{}, err
	}
	wagePayables, activeEmployees, err := organizationBusinessSnapshot(ctx, conn, instanceID, branchID, orgID, orgRec.Fact.Organization.CurrencyID, worldTime, head)
	if err != nil {
		return core.OrganizationEvidence{}, err
	}

	// Read posting capacity and status
	postingRec, err := readCareerRecord(ctx, conn, instanceID, branchID, "posting", positionID)
	if err != nil {
		return core.OrganizationEvidence{}, err
	}
	if postingRec.Fact.Posting == nil {
		return core.OrganizationEvidence{}, core.NewError(core.CodeProjectionDiverged, "posting record missing definition")
	}

	capacity := postingRec.Fact.Posting.Capacity
	status := postingRec.Fact.Posting.Status
	if status == "" {
		status = "active"
	}

	return core.OrganizationEvidence{
		OrganizationID:    orgID,
		WorldTime:         worldTime,
		CashBalanceMinor:  cashBalance,
		WagePayablesMinor: wagePayables,
		NetHeadroomMinor:  cashBalance - wagePayables,
		ActiveEmployees:   activeEmployees,
		CurrentCapacity:   capacity,
		PostingStatus:     status,
	}, nil
}

// ConductOrganizationReview executes an authoritative review round, evaluating
// evidence against policy thresholds and enacting operational decisions (freeze, unfreeze, capacity).
func (s *Store) ConductOrganizationReview(ctx context.Context, r core.OrganizationReviewRequest) (core.OrganizationReviewResult, error) {
	if err := r.Validate(); err != nil {
		return core.OrganizationReviewResult{}, err
	}
	b := r.Binding

	record, err := s.executeCareerCommand(ctx, b, "ConductOrganizationReview", r, func(conn *sql.Conn) error {
		return authorizeCareerManager(ctx, conn, b, r.OrganizationID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		return s.prepareOrganizationReview(ctx, conn, r, c)
	})
	if err != nil {
		return core.OrganizationReviewResult{}, err
	}
	if record.Fact.OrganizationReview == nil {
		return core.OrganizationReviewResult{}, core.NewError(core.CodeProjectionDiverged, "organization review fact missing")
	}
	result := *record.Fact.OrganizationReview
	result.Replay = record.Replayed
	return result, nil
}

func (s *Store) prepareOrganizationReview(ctx context.Context, conn *sql.Conn, r core.OrganizationReviewRequest, c careerCommandContext) (CareerFact, func() error, error) {
	b := r.Binding
	policy, err := readOrganizationAgencyPolicyWithConn(ctx, conn, b.InstanceID, b.BranchID, r.OrganizationID)
	if err != nil {
		return CareerFact{}, nil, err
	}
	if policy.Status != "active" {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "agency policy is not active")
	}
	if policy.PolicyID != r.PolicyID {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "agency policy changed since review request")
	}

	evidence, err := s.collectOrganizationEvidence(ctx, conn, b.InstanceID, b.BranchID, r.OrganizationID, policy.TargetPositionID, c.WorldTime)
	if err != nil {
		return CareerFact{}, nil, err
	}
	evidence.WorldTime = c.WorldTime
	var previous string
	err = conn.QueryRowContext(ctx, `SELECT world_time FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='organization_review' AND json_extract(payload,'$.organization_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, r.OrganizationID).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		return CareerFact{}, nil, err
	}
	if err == nil {
		last, lastErr := time.Parse(time.RFC3339Nano, previous)
		now, nowErr := time.Parse(time.RFC3339Nano, evidence.WorldTime)
		if lastErr != nil || nowErr != nil {
			return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "invalid organization review clock")
		}
		if now.Before(last.Add(time.Duration(policy.ReviewFrequencyHours) * time.Hour)) {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "organization review interval has not elapsed")
		}
	}

	decision := core.EvaluateOrganizationPolicy(policy, evidence)
	policyRecord, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "agency_policy", r.OrganizationID)
	if err != nil {
		return CareerFact{}, nil, err
	}

	reviewID, err := organizationReviewID(b, r.OrganizationID, evidence.WorldTime, c.EventID)
	if err != nil {
		return CareerFact{}, nil, err
	}

	result := core.OrganizationReviewResult{
		PolicySourceEventID: policyRecord.EventID,
		DecisionPrincipalID: b.PrincipalID,
		ReviewID:            reviewID,
		OrganizationID:      r.OrganizationID,
		PolicyID:            policy.PolicyID,
		WorldTime:           evidence.WorldTime,
		Evidence:            evidence,
		Decision:            decision,
		EventID:             c.EventID,
		EventSequence:       c.Sequence,
		Replay:              false,
	}

	fact := CareerFact{
		Kind:               "organization_review",
		RecordID:           reviewID,
		OrganizationID:     r.OrganizationID,
		OrganizationReview: &result,
	}

	// If decision altered posting status or capacity, attach updated posting to the same immutable event
	if decision.DecisionKind != "no_change" {
		postingRec, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", policy.TargetPositionID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		updatedPosting := *postingRec.Fact.Posting
		updatedPosting.Status = decision.PostingStatus
		if decision.EffectiveCapacity > 0 {
			updatedPosting.Capacity = decision.EffectiveCapacity
		}
		fact.Posting = &updatedPosting
	}

	projection := func() error {
		evidenceJSON, err := json.Marshal(evidence)
		if err != nil {
			return err
		}
		decisionJSON, err := json.Marshal(decision)
		if err != nil {
			return err
		}

		_, err = conn.ExecContext(ctx, `INSERT INTO organization_reviews(
				review_id, instance_id, branch_id, organization_id, policy_id,
				reviewed_world_time, evidence_payload, decision_kind, decision_payload, event_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			reviewID, b.InstanceID, b.BranchID, r.OrganizationID, policy.PolicyID,
			evidence.WorldTime, string(evidenceJSON), decision.DecisionKind, string(decisionJSON), c.EventID)
		if err != nil {
			return err
		}
		return queueOrganizationReview(ctx, conn, b, c.EventID, c.WorldTime, policy)
	}

	return fact, projection, nil
}

func organizationReviewID(b core.CareerBinding, organization, at, event string) (string, error) {
	hash, err := core.HashJSON([]any{b.InstanceID, b.BranchID, organization, at, event})
	if err != nil {
		return "", err
	}
	// HashJSON includes the "sha256:" algorithm prefix. Keep all 256 digest
	// bits; taking hash[:12] accidentally kept just five hex digits (20 bits).
	return "review_" + hash[len("sha256:"):], nil
}

// ReadOrganizationReviews returns past review history for an organization.
func (s *Store) ReadOrganizationReviews(ctx context.Context, principalID, instanceID, branchID, orgID string, limit int) ([]core.OrganizationReviewResult, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := authorizeCareerManager(ctx, conn, core.CareerBinding{PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID}, orgID); err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := conn.QueryContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='organization_review' AND json_extract(payload,'$.organization_id')=? ORDER BY event_sequence DESC LIMIT ?`,
		instanceID, branchID, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []core.OrganizationReviewResult
	for rows.Next() {
		var f CareerFact
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			return nil, err
		}
		if f.OrganizationReview == nil || f.OrganizationReview.OrganizationID != orgID {
			return nil, core.NewError(core.CodeProjectionDiverged, "review source differs")
		}
		results = append(results, *f.OrganizationReview)
	}
	return results, rows.Err()
}
