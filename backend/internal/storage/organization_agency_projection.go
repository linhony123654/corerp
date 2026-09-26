package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

type organizationPolicyRow struct {
	Policy core.OrganizationAgencyPolicy `json:"policy"`
	Source string                        `json:"source"`
}

// Only immutable Career Events determine policy and review projections. In
// particular a modified policy table must never authorize a different decision.
func organizationAgencyExpected(ctx context.Context, q replayQuerier, instance, branch string, head int64) (map[string]organizationPolicyRow, map[string]core.OrganizationReviewResult, error) {
	policies := map[string]organizationPolicyRow{}
	reviews := map[string]core.OrganizationReviewResult{}
	lastReview := map[string]time.Time{}
	latestSource := map[string]string{}
	sourceTimes := map[string]string{}
	type businessAudit struct {
		Review   core.OrganizationReviewResult
		Posting  *core.CareerPostingDefinition
		Position string
	}
	var audits []businessAudit
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,world_time,payload,actor_id FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind') IN ('agency_policy','organization_review') ORDER BY event_sequence`, instance, branch, head)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	invalid := func() (map[string]organizationPolicyRow, map[string]core.OrganizationReviewResult, error) {
		return nil, nil, core.NewError(core.CodeProjectionDiverged, "organization agency Event source differs")
	}
	for rows.Next() {
		var id, at, raw, actor string
		var sequence int64
		if err := rows.Scan(&id, &sequence, &at, &raw, &actor); err != nil {
			return nil, nil, err
		}
		var f CareerFact
		if json.Unmarshal([]byte(raw), &f) != nil || f.Version != "corerp.career.v1" || f.OrganizationID == "" {
			return invalid()
		}
		if f.Kind == "agency_policy" {
			if f.AgencyPolicy == nil || f.AgencyPolicy.Validate() != nil || f.RecordID != f.OrganizationID || f.AgencyPolicy.OrganizationID != f.OrganizationID {
				return invalid()
			}
			if prior, exists := policies[f.OrganizationID]; exists && prior.Policy.PolicyID != f.AgencyPolicy.PolicyID {
				return invalid()
			}
			policies[f.OrganizationID] = organizationPolicyRow{*f.AgencyPolicy, id}
			latestSource[f.OrganizationID] = id
			sourceTimes[id] = at
			continue
		}
		r := f.OrganizationReview
		p, exists := policies[f.OrganizationID]
		if r == nil || !exists || p.Policy.Status != "active" || r.PolicyID != p.Policy.PolicyID || r.PolicySourceEventID != p.Source || r.DecisionPrincipalID == "" ||
			r.OrganizationID != f.OrganizationID || r.ReviewID == "" || r.ReviewID != f.RecordID || r.EventID != id || r.EventSequence != sequence || r.WorldTime != at || r.Replay ||
			r.Evidence.OrganizationID != f.OrganizationID || r.Evidence.WorldTime != at || r.Evidence.WagePayablesMinor < 0 ||
			r.Evidence.CashBalanceMinor < 0 || r.Evidence.ActiveEmployees < 0 || r.Evidence.CurrentCapacity < 1 || r.Evidence.CurrentCapacity > 100 ||
			(r.Evidence.PostingStatus != "active" && r.Evidence.PostingStatus != "frozen") {
			return invalid()
		}
		if r.Evidence.NetHeadroomMinor != r.Evidence.CashBalanceMinor-r.Evidence.WagePayablesMinor || r.Decision != core.EvaluateOrganizationPolicy(p.Policy, r.Evidence) {
			return invalid()
		}
		audits = append(audits, businessAudit{*r, f.Posting, p.Policy.TargetPositionID})
		if actor == "system" {
			item, _, err := organizationReviewItem(r.ScheduleSourceEventID, sourceTimes[r.ScheduleSourceEventID], p.Policy)
			if err != nil || !p.Policy.AutomaticReview || r.DecisionPrincipalID != p.Policy.ManagerPrincipalID || r.ScheduleSourceEventID != latestSource[f.OrganizationID] || id != "event_"+item.SchedulerItemID || at != item.WorldTime {
				return invalid()
			}
		} else if r.DecisionPrincipalID != actor || r.ScheduleSourceEventID != "" {
			return invalid()
		}
		when, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return invalid()
		}
		if previous, exists := lastReview[f.OrganizationID]; exists && when.Before(previous.Add(time.Duration(p.Policy.ReviewFrequencyHours)*time.Hour)) {
			return invalid()
		}
		lastReview[f.OrganizationID] = when
		if _, duplicate := reviews[r.ReviewID]; duplicate {
			return invalid()
		}
		if r.Decision.DecisionKind == "no_change" {
			if f.Posting != nil {
				return invalid()
			}
		} else if f.Posting == nil || f.Posting.OrganizationID != f.OrganizationID || f.Posting.PositionID != p.Policy.TargetPositionID || f.Posting.Status != r.Decision.PostingStatus || f.Posting.Capacity != r.Decision.EffectiveCapacity {
			return invalid()
		}
		reviews[r.ReviewID] = *r
		latestSource[f.OrganizationID] = id
		sourceTimes[id] = at
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	// Release the source cursor before querying through a single-connection DB.
	for _, audit := range audits {
		if err := verifyOrganizationReviewBusinessSource(ctx, q, instance, branch, audit.Review.EventSequence-1, audit.Review, audit.Posting, audit.Position); err != nil {
			return nil, nil, err
		}
	}
	return policies, reviews, nil
}

func organizationAgencyProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, head int64) ([]ProjectionDifference, error) {
	policies, reviews, err := organizationAgencyExpected(ctx, q, instance, branch, head)
	if err != nil {
		return nil, err
	}
	want, got := map[string]string{}, map[string]string{}
	encode := func(v any) (string, error) { b, err := core.CanonicalJSON(v); return string(b), err }
	for id, p := range policies {
		raw, err := encode(p)
		if err != nil {
			return nil, err
		}
		want["policy:"+id] = raw
	}
	for id, r := range reviews {
		raw, err := encode(r)
		if err != nil {
			return nil, err
		}
		want["review:"+id] = raw
	}
	rows, err := q.QueryContext(ctx, `SELECT policy_id,organization_id,manager_principal_id,review_frequency_hours,reserve_target_minor,hiring_threshold_minor,freeze_threshold_minor,target_position_id,default_capacity,status,definition_event_id,automatic_review FROM organization_agency_policies WHERE instance_id=? AND branch_id=?`, instance, branch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var row organizationPolicyRow
		p := &row.Policy
		if err := rows.Scan(&p.PolicyID, &p.OrganizationID, &p.ManagerPrincipalID, &p.ReviewFrequencyHours, &p.ReserveTargetMinor, &p.HiringThresholdMinor, &p.FreezeThresholdMinor, &p.TargetPositionID, &p.DefaultCapacity, &p.Status, &row.Source, &p.AutomaticReview); err != nil {
			rows.Close()
			return nil, err
		}
		raw, err := encode(row)
		if err != nil {
			rows.Close()
			return nil, err
		}
		got["policy:"+p.OrganizationID] = raw
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, `SELECT r.review_id,r.organization_id,r.policy_id,r.reviewed_world_time,r.evidence_payload,r.decision_kind,r.decision_payload,r.event_id,COALESCE(e.event_sequence,0) FROM organization_reviews r LEFT JOIN events e ON e.event_id=r.event_id AND e.instance_id=r.instance_id AND e.branch_id=r.branch_id WHERE r.instance_id=? AND r.branch_id=?`, instance, branch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r core.OrganizationReviewResult
		var evidence, decision, kind string
		if err := rows.Scan(&r.ReviewID, &r.OrganizationID, &r.PolicyID, &r.WorldTime, &evidence, &kind, &decision, &r.EventID, &r.EventSequence); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(evidence), &r.Evidence); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(decision), &r.Decision); err != nil {
			rows.Close()
			return nil, err
		}
		if source, exists := reviews[r.ReviewID]; exists {
			r.PolicySourceEventID = source.PolicySourceEventID
			r.DecisionPrincipalID = source.DecisionPrincipalID
			r.ScheduleSourceEventID = source.ScheduleSourceEventID
		}
		raw, err := encode(r)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if kind != r.Decision.DecisionKind {
			raw += " decision_kind=" + kind
		}
		got["review:"+r.ReviewID] = raw
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for key := range want {
		keys[key] = true
	}
	for key := range got {
		keys[key] = true
	}
	var ordered []string
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var differences []ProjectionDifference
	for _, key := range ordered {
		w, ok := want[key]
		if !ok {
			w = "absent"
		}
		g, ok := got[key]
		if !ok {
			g = "missing"
		}
		if w != g {
			differences = append(differences, ProjectionDifference{Projection: "organization_agency", Key: key, ExpectedText: w, ActualText: g})
		}
	}
	queues, err := organizationReviewQueueDifferences(ctx, q, instance, branch, head)
	if err != nil {
		return nil, err
	}
	return append(differences, queues...), nil
}

func repairOrganizationAgencyProjections(ctx context.Context, conn *sql.Conn, instance, branch string, head int64) error {
	policies, reviews, err := organizationAgencyExpected(ctx, conn, instance, branch, head)
	if err != nil {
		return err
	}
	// These are disposable projections; the immutable source Events stay intact.
	if _, err := conn.ExecContext(ctx, `DELETE FROM organization_reviews WHERE instance_id=? AND branch_id=?`, instance, branch); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM organization_agency_policies WHERE instance_id=? AND branch_id=?`, instance, branch); err != nil {
		return err
	}
	for _, row := range policies {
		p := row.Policy
		if _, err := conn.ExecContext(ctx, `INSERT INTO organization_agency_policies(policy_id,instance_id,branch_id,organization_id,manager_principal_id,review_frequency_hours,reserve_target_minor,hiring_threshold_minor,freeze_threshold_minor,target_position_id,default_capacity,status,definition_event_id,automatic_review) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.PolicyID, instance, branch, p.OrganizationID, p.ManagerPrincipalID, p.ReviewFrequencyHours, p.ReserveTargetMinor, p.HiringThresholdMinor, p.FreezeThresholdMinor, p.TargetPositionID, p.DefaultCapacity, p.Status, row.Source, p.AutomaticReview); err != nil {
			return err
		}
	}
	for _, r := range reviews {
		evidence, err := json.Marshal(r.Evidence)
		if err != nil {
			return err
		}
		decision, err := json.Marshal(r.Decision)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO organization_reviews(review_id,instance_id,branch_id,organization_id,policy_id,reviewed_world_time,evidence_payload,decision_kind,decision_payload,event_id) VALUES (?,?,?,?,?,?,?,?,?,?)`, r.ReviewID, instance, branch, r.OrganizationID, r.PolicyID, r.WorldTime, string(evidence), r.Decision.DecisionKind, string(decision), r.EventID); err != nil {
			return err
		}
	}
	return repairOrganizationReviewQueues(ctx, conn, instance, branch, head)
}
