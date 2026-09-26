package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestOrganizationAgencyBusinessEvidenceIgnoresMissingAndForgedProjections(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "agency-business.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	p := core.OrganizationAgencyPolicy{PolicyID: "business_sources", OrganizationID: "actor_m2_coop_employer", ManagerPrincipalID: M2AgentBoPrincipal, ReviewFrequencyHours: 1, ReserveTargetMinor: 1010, TargetPositionID: "position_coop_assistant", DefaultCapacity: 2, Status: "active"}
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "policy"), Policy: p}); err != nil {
		t.Fatal(err)
	}
	read := func(at string) (int64, int) {
		t.Helper()
		head := careerTestBinding(t, s, M2AgentBoPrincipal, "cursor").ExpectedHead
		payable, staff, err := organizationBusinessSnapshot(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, p.OrganizationID, M2DemoCurrencyID, at, head)
		if err != nil {
			t.Fatal(err)
		}
		return payable, staff
	}
	if _, err := s.RunAgentLife(ctx, m2EconomyAccrualTime, 1000); err != nil {
		t.Fatal(err)
	}
	if payable, staff := read(m2EconomyAccrualTime); payable != 180 || staff != 19 {
		t.Fatalf("cohort plus independent employment: payable=%d staff=%d", payable, staff)
	}
	// Corrupt the obligation's paid amount; journal authority still says 180.
	if _, err := s.db.Exec(`UPDATE m2_economic_obligations SET amount_paid_minor=amount_due_minor,status='paid' WHERE kind='wage'`); err != nil {
		t.Fatal(err)
	}
	if payable, _ := read(m2EconomyAccrualTime); payable != 180 {
		t.Fatalf("forged aggregate payment hid liabilities: %d", payable)
	}
	// Restore only this injected corruption so the real payment can execute.
	if _, err := s.db.Exec(`UPDATE m2_economic_obligations SET amount_paid_minor=0,status='accrued' WHERE kind='wage'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 0), 1000); err != nil {
		t.Fatal(err)
	}
	if payable, staff := read(careerTime(2, 0, 0)); payable != 12 || staff != 19 {
		t.Fatalf("settled cohort and newly accrued individual: payable=%d staff=%d", payable, staff)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations WHERE contract_id=?`, []any{accepted.Fact.Employment.ContractID}, 1)
	if _, err := s.db.Exec(`DELETE FROM wage_obligations WHERE contract_id=?`, accepted.Fact.Employment.ContractID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE employment_contracts SET status='ended' WHERE contract_id=?`, accepted.Fact.Employment.ContractID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE m2_cohort_contracts SET participant_count=1 WHERE contract_id=?`, m2EconomyContractID); err != nil {
		t.Fatal(err)
	}
	review, err := s.ConductOrganizationReview(ctx, core.OrganizationReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "review"), OrganizationID: p.OrganizationID, PolicyID: p.PolicyID})
	if err != nil {
		t.Fatal(err)
	}
	if review.Evidence.WagePayablesMinor != 12 || review.Evidence.ActiveEmployees != 19 || review.Evidence.NetHeadroomMinor != 1008 || review.Decision.DecisionKind != "freeze_recruitment" {
		t.Fatalf("derived-table corruption changed authorized review: %+v", review)
	}
}

func TestOrganizationAgencyHistoricalEvidenceCannotBeSelfConsistentlyForged(t *testing.T) {
	for _, mutation := range []struct{ name, expression string }{
		{"staff", `json_set(payload,'$.organization_review.evidence.active_employees',999)`},
		{"payable", `json_set(payload,'$.organization_review.evidence.wage_payables_minor',12,'$.organization_review.evidence.net_headroom_minor',1188)`},
		{"job_terms", `json_set(payload,'$.posting.daily_wage_minor',999)`},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			ctx := context.Background()
			s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "historical-business.db"))
			defer s.Close()
			if _, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s)); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PostCareerPosition(ctx, careerTestPosting(t, s)); err != nil {
				t.Fatal(err)
			}
			p := core.OrganizationAgencyPolicy{PolicyID: "history_source", OrganizationID: "actor_m2_coop_employer", ManagerPrincipalID: M2AgentBoPrincipal, ReviewFrequencyHours: 1, TargetPositionID: "position_coop_assistant", DefaultCapacity: 2, Status: "active"}
			if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "policy"), Policy: p}); err != nil {
				t.Fatal(err)
			}
			r, err := s.ConductOrganizationReview(ctx, core.OrganizationReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "review"), OrganizationID: p.OrganizationID, PolicyID: p.PolicyID})
			if err != nil {
				t.Fatal(err)
			}
			if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
				t.Fatalf("valid history: %+v %v", diff, err)
			}
			// Test-only source mutation: the declared decision remains compatible
			// with the forged input; historical business authority must catch it.
			if _, err := s.db.Exec(`UPDATE events SET payload=`+mutation.expression+` WHERE event_id=?`, r.EventID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatalf("forged source accepted: %v", err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatalf("rebuild blessed forged source: %v", err)
			}
		})
	}
}

func TestOrganizationAgencyWorkforceUsesEffectiveStartsAndExits(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "agency-staff.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	assertStaff := func(want int) {
		t.Helper()
		var at string
		if err := s.db.QueryRow(`SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		_, staff, err := organizationBusinessSnapshot(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, "actor_m2_coop_employer", M2DemoCurrencyID, at, careerTestBinding(t, s, M2AgentBoPrincipal, "cursor").ExpectedHead)
		if err != nil || staff != want {
			t.Fatalf("effective workforce=%d want=%d error=%v", staff, want, err)
		}
	}
	assertStaff(18) // Accepted but not yet started must not inflate headcount.
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 1000); err != nil {
		t.Fatal(err)
	}
	assertStaff(19)
	if _, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "exit"), ContractID: accepted.Fact.Employment.ContractID, Kind: "resignation", EffectiveFromDay: 2, Notice: "Leaving tomorrow."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestCareerAggregateExit(ctx, core.CareerAggregateExitRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "aggregate-exit"), CandidateID: M2RPNPCID, ContractID: m2EconomyContractID, FinalEarnedDay: 2, Notice: "Leaving after earned work."}); err != nil {
		t.Fatal(err)
	}
	assertStaff(19) // Neither notice is an activated exit.
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 1000); err != nil {
		t.Fatal(err)
	}
	assertStaff(18)
	if _, err := s.RunAgentLife(ctx, careerTime(2, 7, 3), 1000); err != nil {
		t.Fatal(err)
	}
	assertStaff(17)
}
