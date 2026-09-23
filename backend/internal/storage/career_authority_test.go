package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func careerRoleProbe(t *testing.T, s *Store, key string) core.CareerPostingRequest {
	t.Helper()
	r := careerTestPosting(t, s)
	r.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, key)
	r.Posting.PositionID = key
	return r
}

func TestCareerRoleAuthorityPromotionDemotionAndRecovery(t *testing.T) {
	for _, independent := range []bool{false, true} {
		name := "role-only"
		if independent {
			name = "independent-appointment"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "role.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			initial, proposal := prepareCareerPositionOffer(t, s, "senior")
			post := careerTestPosting(t, s)
			post.Binding.IdempotencyKey = "manager-post"
			post.Posting.PositionID, post.Posting.Grade, post.Posting.DailyWageMinor = "manager-position", "senior", 30
			post.Posting.RequiredQualifications = []string{"team_coordination"}
			post.Posting.Capabilities = []string{core.CareerPositionManageCapability}
			if _, err := s.PostCareerPosition(ctx, post); err != nil {
				t.Fatal(err)
			}
			proposal.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "manager-offer")
			proposal.PositionID = post.Posting.PositionID
			if _, err := s.OfferCareerPositionChange(ctx, proposal); err != nil {
				t.Fatal(err)
			}
			accepted, err := s.AcceptCareerPositionChange(ctx, core.CareerPositionAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "manager-accept"), ChangeID: proposal.ChangeID})
			if err != nil {
				t.Fatal(err)
			}
			if len(accepted.Fact.Employment.Capabilities) != 1 {
				t.Fatal("accepted terms lost role authority")
			}
			if _, err := s.PostCareerPosition(ctx, careerRoleProbe(t, s, "too-early")); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("early authority: %v", err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(2, 12, 0), 100); err != nil {
				t.Fatal(err)
			}
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "role activation rollback") }
			if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 0), 100); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("rollback: %v", err)
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE capability_id=? AND principal_id=?`, []any{core.CareerPositionManageCapability, M2AgentAdaPrincipal}, 0)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(4, 0, 1), 100); err != nil {
				t.Fatal(err)
			}
			if own := readCareerTestContext(t, s, M2AgentAdaID); len(own.Life.Employment[0].PositionCapabilities) != 1 {
				t.Fatal("own effective role capability missing")
			}
			if _, err := s.PostCareerPosition(ctx, careerRoleProbe(t, s, "actual-management")); err != nil {
				t.Fatalf("promoted manager cannot post: %v", err)
			}
			wrong := careerRoleProbe(t, s, "other-organization")
			wrong.Posting.OrganizationID = "another-organization"
			if _, err := s.PostCareerPosition(ctx, wrong); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("cross-org privilege: %v", err)
			}
			creatorOnly := careerTestOrg(t, s)
			creatorOnly.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "creator-probe")
			if _, err := s.DefineCareerOrganization(ctx, creatorOnly); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("role gained creator authority: %v", err)
			}
			var grantID string
			if err := s.db.QueryRow(`SELECT grant_id FROM capability_grants WHERE capability_id=? AND principal_id=?`, core.CareerPositionManageCapability, M2AgentAdaPrincipal).Scan(&grantID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`UPDATE capability_grants SET status='revoked',field_scope='["wrong"]' WHERE grant_id=?`, grantID); err != nil {
				t.Fatal(err)
			}
			diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
			if err != nil || len(diffs) != 1 || diffs[0].Projection != "career_role_grant" {
				t.Fatalf("role corruption: %+v %v", diffs, err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PostCareerPosition(ctx, careerRoleProbe(t, s, "repaired-management")); err != nil {
				t.Fatalf("repaired authority: %v", err)
			}
			// Wrong identity on an otherwise identical tuple produces one extra
			// row and one missing row; rebuild removes extras before restoring.
			if _, err := s.db.Exec(`UPDATE capability_grants SET grant_id='unexpected_role_projection' WHERE grant_id=?`, grantID); err != nil {
				t.Fatal(err)
			}
			if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 2 {
				t.Fatalf("missing/extra role projection: %+v %v", diffs, err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE grant_id=? AND status='active'`, []any{grantID}, 1)
			if independent {
				// Fixture: independently appointed authority must remain separate
				// from position authority, including while both are active.
				if _, err := s.db.Exec(`INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) SELECT 'independent_ada',?,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id FROM capability_grants WHERE principal_id=? AND capability_id=?`, M2AgentAdaPrincipal, M2AgentBoPrincipal, careerManageCapability); err != nil {
					t.Fatal(err)
				}
				if _, err := s.PostCareerPosition(ctx, careerRoleProbe(t, s, "dual-authority")); err != nil {
					t.Fatalf("two legitimate grant origins rejected: %v", err)
				}
			}
			report, err := s.ReadCareerAttendance(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, initial.Fact.Employment.ContractID, 3)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.RecordCareerPerformance(ctx, core.CareerPerformanceRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "demotion-review"), ReviewID: "demotion-review", ContractID: initial.Fact.Employment.ContractID, EvidenceEventIDs: []string{report.EventID}, Assessment: "needs_improvement", Reason: "Role no longer suitable"}); err != nil {
				t.Fatal(err)
			}
			back := core.CareerPositionOfferRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "demotion"), ChangeID: "demotion", ReviewID: "demotion-review", PositionID: initial.Fact.Employment.PositionID, EffectiveFromDay: 5, Assessments: []core.CareerQualificationAssessment{{Code: "safety_training", Passed: true, Reason: "Still qualified for prior role"}}, Notice: "Return to nonmanager role"}
			if _, err := s.OfferCareerPositionChange(ctx, back); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AcceptCareerPositionChange(ctx, core.CareerPositionAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "demotion-accept"), ChangeID: back.ChangeID}); err != nil {
				t.Fatal(err)
			}
			// Missing projection must not erase the historical need to revoke.
			if _, err := s.db.Exec(`DELETE FROM capability_grants WHERE grant_id=?`, grantID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(5, 0, 1), 100); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE grant_id=? AND status='revoked'`, []any{grantID}, 1)
			if own := readCareerTestContext(t, s, M2AgentAdaID); len(own.Life.Employment[0].PositionCapabilities) != 0 {
				t.Fatal("old position capability survived in own context")
			}
			if _, err := s.db.Exec(`UPDATE capability_grants SET status='active' WHERE grant_id=?`, grantID); err != nil {
				t.Fatal(err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			_, err = s.PostCareerPosition(ctx, careerRoleProbe(t, s, "after-demotion"))
			if independent && err != nil {
				t.Fatalf("independent authority lost: %v", err)
			}
			if !independent && !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("role privilege survived demotion: %v", err)
			}
			if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
				t.Fatalf("post-rebuild roles: %+v %v", diffs, err)
			}
		})
	}
}

func TestCareerRoleAuthorityInitialHiringStartsAtBoundary(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "initial-role.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 12, core.CareerPositionManageCapability)
	if _, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PostCareerPosition(ctx, careerRoleProbe(t, s, "early")); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("onboarding privilege: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 0, 0), 100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PostCareerPosition(ctx, careerRoleProbe(t, s, "started")); err != nil {
		t.Fatalf("initial manager inactive: %v", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("initial grant recovery: %+v %v", diffs, err)
	}
}
