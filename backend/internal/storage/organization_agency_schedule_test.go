package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestOrganizationAgencyScheduledReviewsRecoverAndRespectPolicy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "scheduled-agency.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	org := careerTestOrg(t, s)
	if _, err := s.DefineCareerOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	post := careerTestPosting(t, s)
	if _, err := s.PostCareerPosition(ctx, post); err != nil {
		t.Fatal(err)
	}
	p := core.OrganizationAgencyPolicy{PolicyID: "automatic_coop", OrganizationID: org.Organization.OrganizationID, ManagerPrincipalID: M2AgentBoPrincipal, ReviewFrequencyHours: 24, AutomaticReview: true, ReserveTargetMinor: 990, HiringThresholdMinor: 10, TargetPositionID: post.Posting.PositionID, DefaultCapacity: 2, Status: "active"}
	b := careerTestBinding(t, s, M2AgentBoPrincipal, "automatic-policy")
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: b, Policy: p}); err != nil {
		t.Fatal(err)
	}
	var firstDue string
	if err := s.db.QueryRow(`SELECT world_time FROM scheduler_items WHERE phase_id=? AND status='pending'`, organizationReviewPhase).Scan(&firstDue); err != nil {
		t.Fatal(err)
	}
	partial, err := s.RunAgentLife(ctx, firstDue, 1)
	if err != nil || partial.ProcessedItems != 1 || partial.PendingDue == 0 {
		t.Fatalf("bounded run: %+v %v", partial, err)
	}
	if _, err := s.RunAgentLife(ctx, firstDue, 1000); err != nil {
		t.Fatal(err)
	}
	history, err := s.ReadOrganizationReviews(ctx, M2AgentBoPrincipal, b.InstanceID, b.BranchID, p.OrganizationID, 10)
	if err != nil || len(history) != 1 || history[0].Decision.DecisionKind != "expand_capacity" || history[0].ScheduleSourceEventID == "" {
		t.Fatalf("automatic expansion: %+v %v", history, err)
	}
	first := history[0]
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	managerView := RPSession{SessionID: "scheduled_manager_handle_fixture", InstanceID: b.InstanceID, BranchID: b.BranchID, ControlledEntityID: M2AgentBoID}
	sources, err := rpNoticePublicationSourcesForSession(ctx, conn, managerView, "organization_announcement")
	if err != nil || len(sources.Sources) != 1 {
		conn.Close()
		t.Fatalf("scheduled publication discovery: %+v %v", sources, err)
	}
	resolved, err := resolveRPNoticePublicationSource(ctx, conn, managerView, "organization_announcement", sources.Sources[0].SourceHandle)
	if err != nil || resolved != first.EventID {
		conn.Close()
		t.Fatalf("scheduled source handle did not resolve: %s %v", resolved, err)
	}
	managerView.SessionID = "another_manager_session"
	if _, err := resolveRPNoticePublicationSource(ctx, conn, managerView, "organization_announcement", sources.Sources[0].SourceHandle); !core.HasCode(err, core.CodeNotFound) {
		conn.Close()
		t.Fatalf("publication handle escaped session: %v", err)
	}
	conn.Close()
	encoded, _ := json.Marshal(sources)
	if strings.Contains(string(encoded), first.EventID) || strings.Contains(string(encoded), "cash_balance_minor") {
		t.Fatal("scheduled publication source leaked private evidence")
	}
	notice, err := s.PublishRPOrganizationNotice(ctx, RPOrganizationNoticePublishRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "automatic-notice"), MessageID: "automatic_expansion_notice", CareerEventID: first.EventID, SpeakerID: M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := rpInformationExpected(ctx, s.db, b.InstanceID, b.BranchID, notice.EventSequence); err != nil {
		t.Fatalf("scheduled review notice source: %v", err)
	}
	assertClean := func() {
		t.Helper()
		d, err := s.CompareProjections(ctx, b.InstanceID, b.BranchID)
		if err != nil || len(d) != 0 {
			t.Fatalf("scheduled source parity: %+v %v", d, err)
		}
	}
	assertClean()
	var nextID, nextDue string
	if err := s.db.QueryRow(`SELECT scheduler_item_id,world_time FROM scheduler_items WHERE phase_id=? AND status='pending'`, organizationReviewPhase).Scan(&nextID, &nextDue); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE scheduler_items SET declared_priority=1 WHERE scheduler_item_id=?`, nextID); err != nil {
		t.Fatal(err)
	}
	d, err := s.CompareProjections(ctx, b.InstanceID, b.BranchID)
	if err != nil || len(d) != 1 || d[0].Projection != "organization_review_queue" {
		t.Fatalf("queue corruption: %+v %v", d, err)
	}
	if err := s.RebuildProjections(ctx, b.InstanceID, b.BranchID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, nextDue, 1000); err != nil {
		t.Fatal(err)
	}
	history, err = s.ReadOrganizationReviews(ctx, M2AgentBoPrincipal, b.InstanceID, b.BranchID, p.OrganizationID, 10)
	if err != nil || len(history) != 2 || history[0].Decision.DecisionKind != "freeze_recruitment" || history[0].Evidence.CashBalanceMinor >= first.Evidence.CashBalanceMinor {
		t.Fatalf("real wages did not drive scheduled freeze: %+v %v", history, err)
	}
	if _, err := s.RunAgentLife(ctx, nextDue, 1000); err != nil {
		t.Fatal(err)
	}
	again, err := s.ReadOrganizationReviews(ctx, M2AgentBoPrincipal, b.InstanceID, b.BranchID, p.OrganizationID, 10)
	if err != nil || len(again) != 2 {
		t.Fatalf("duplicate scheduled review: %+v %v", again, err)
	}
	assertClean()
	if err := s.db.QueryRow(`SELECT world_time FROM scheduler_items WHERE phase_id=? AND status='pending'`, organizationReviewPhase).Scan(&nextDue); err != nil {
		t.Fatal(err)
	}
	p.Status = "suspended"
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "suspend-policy"), Policy: p}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, nextDue, 1000); err != nil {
		t.Fatal(err)
	}
	again, err = s.ReadOrganizationReviews(ctx, M2AgentBoPrincipal, b.InstanceID, b.BranchID, p.OrganizationID, 10)
	if err != nil || len(again) != 2 {
		t.Fatalf("suspended policy reviewed: %+v %v", again, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='OrganizationReviewSkipped'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=? AND status='pending'`, []any{organizationReviewPhase}, 0)
	assertClean()
}

func TestOrganizationAgencyScheduledReviewRollbackAndAuthorityLoss(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agency-rollback.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	org := careerTestOrg(t, s)
	if _, err := s.DefineCareerOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	post := careerTestPosting(t, s)
	if _, err := s.PostCareerPosition(ctx, post); err != nil {
		t.Fatal(err)
	}
	p := core.OrganizationAgencyPolicy{PolicyID: "rollback_policy", OrganizationID: org.Organization.OrganizationID, ManagerPrincipalID: M2AgentBoPrincipal, ReviewFrequencyHours: 1, AutomaticReview: true, TargetPositionID: post.Posting.PositionID, DefaultCapacity: 2, Status: "active"}
	b := careerTestBinding(t, s, M2AgentBoPrincipal, "rollback-policy")
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: b, Policy: p}); err != nil {
		t.Fatal(err)
	}
	var due string
	if err := s.db.QueryRow(`SELECT world_time FROM scheduler_items WHERE phase_id=? AND status='pending'`, organizationReviewPhase).Scan(&due); err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, due)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, at.Add(-time.Second).Format(time.RFC3339), 1000); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "organization review commit lost") }
	_, err = s.RunAgentLife(ctx, due, 1000)
	s.beforeCommit = nil
	if !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected transactional failure: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='organization_review'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=? AND status='pending'`, []any{organizationReviewPhase}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, due, 1000); err != nil {
		t.Fatal(err)
	}
	history, err := s.ReadOrganizationReviews(ctx, M2AgentBoPrincipal, b.InstanceID, b.BranchID, p.OrganizationID, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("restart did not settle once: %+v %v", history, err)
	}
	d, err := s.CompareProjections(ctx, b.InstanceID, b.BranchID)
	if err != nil || len(d) != 0 {
		t.Fatalf("recovered scheduled parity: %+v %v", d, err)
	}
	if err := s.db.QueryRow(`SELECT world_time FROM scheduler_items WHERE phase_id=? AND status='pending'`, organizationReviewPhase).Scan(&due); err != nil {
		t.Fatal(err)
	}
	// Test-only grant revocation verifies authorization at execution, not just
	// when the original policy was accepted.
	if _, err := s.db.Exec(`UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND subject_id=? AND capability_id=?`, M2AgentBoPrincipal, p.OrganizationID, careerManageCapability); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, due, 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='organization_review'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='OrganizationReviewSkipped' AND json_extract(payload,'$.reason')='manager_authority_unavailable'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=? AND status='pending'`, []any{organizationReviewPhase}, 0)
}
