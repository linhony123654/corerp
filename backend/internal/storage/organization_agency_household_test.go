package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestOrganizationAgencyVacancyChangesHouseholdAndInformation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "organization-household.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	// Real funded world without Bo's optional demo routine, which would overlap
	// the independent employment created below.
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	for _, command := range []core.MaterializeCohortCommand{
		m2AgentMaterialization("rp_cai", M2RPNPCID, "Cai", 1, 400, 3, 0, 60, 0, "2026-09-22T07:01:00Z"),
		m2AgentMaterialization("rp_lin", M2RPPlayerID, "Lin", 1, 300, 3, 0, 45, 0, "2026-09-22T07:02:00Z"),
	} {
		command.ExpectedHead = careerTestBinding(t, s, "principal_creator", "cursor").ExpectedHead
		if _, err := s.MaterializeCohort(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.setupRPParticipantsAt(ctx, rpLifeSetupTime); err != nil {
		t.Fatal(err)
	}
	if _, err := s.setupRPTravelAt(ctx, rpLifeSetupTime); err != nil {
		t.Fatal(err)
	}
	org := careerTestOrg(t, s)
	org.Organization.ManagerPrincipalID = M2RPPlayerPrincipal
	if _, err := s.DefineCareerOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	post := careerTestPosting(t, s)
	post.Binding.PrincipalID = M2RPPlayerPrincipal
	if _, err := s.PostCareerPosition(ctx, post); err != nil {
		t.Fatal(err)
	}
	prepare := func(id, principal string) {
		t.Helper()
		if _, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: careerTestBinding(t, s, principal, "apply_"+id), ApplicationID: "app_" + id, PositionID: post.Posting.PositionID, CandidateID: id, Statement: "Apply for cooperative work."}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "invite_"+id), InterviewID: "interview_" + id, ApplicationID: "app_" + id, Question: "Explain safe work."}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, principal, "answer_"+id), InterviewID: "interview_" + id, Answer: "Inspect equipment and exits."}); err != nil {
			t.Fatal(err)
		}
		evaluation := careerTestEvaluation(t, s, "evaluation_"+id, true)
		evaluation.Binding.PrincipalID, evaluation.InterviewID = M2RPPlayerPrincipal, "interview_"+id
		if _, err := s.EvaluateCareerApplication(ctx, evaluation); err != nil {
			t.Fatal(err)
		}
	}
	offer := func(id string) error {
		r := careerTestOffer(t, s, "offer_"+id, "evaluation_"+id)
		r.Binding.PrincipalID, r.StartsOnDay = M2RPPlayerPrincipal, 2
		_, err := s.OfferCareerEmployment(ctx, r)
		return err
	}
	accept := func(id, principal string) {
		t.Helper()
		if _, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, principal, "accept_"+id), OfferID: "offer_" + id, AfterWorkPlaceID: M2AgentCafeID}); err != nil {
			t.Fatal(err)
		}
	}
	prepare(M2AgentAdaID, M2AgentAdaPrincipal)
	if err := offer(M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	accept(M2AgentAdaID, M2AgentAdaPrincipal)
	prepare(M2AgentBoID, M2AgentBoPrincipal)
	if err := offer(M2AgentBoID); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("full posting allowed second offer: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f8_operator','operator','F8 fixture operator','active')`); err != nil {
		t.Fatal(err)
	}
	home, err := s.FoundRPHouseholdLocal(ctx, RPHouseholdFoundRequest{Binding: careerTestBinding(t, s, "principal_f8_operator", "home"), HouseholdKey: "agency-home", DisplayName: "Agency household", ResidencePlaceID: M2AgentCafeID, AdultEntityIDs: [2]string{M2AgentAdaID, M2AgentBoID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgreeRPHouseholdRentLocal(ctx, RPHouseholdRentAgreementRequest{Binding: careerTestBinding(t, s, "principal_f8_operator", "rent"), HouseholdID: home.Fact.HouseholdID, AgreementKey: "agency-rent", LandlordName: "Agency landlord", RentMinor: 500, PeriodDays: 30, GraceDays: 3, Shares: [2]RPHouseholdRentShare{{M2AgentAdaID, 250}, {M2AgentBoID, 250}}}); err != nil {
		t.Fatal(err)
	}
	before := readCareerTestContext(t, s, M2AgentAdaID).Life.HouseholdPressure
	if before == nil || before.PressureLevel != "at_risk" || before.ExpectedIncomeMinor != 360 || before.CoverageGapMinor != 140 {
		t.Fatalf("one income should leave rent pressure: %+v", before)
	}
	p := core.OrganizationAgencyPolicy{PolicyID: "household_expansion", OrganizationID: org.Organization.OrganizationID, ManagerPrincipalID: M2RPPlayerPrincipal, ReviewFrequencyHours: 1, AutomaticReview: true, HiringThresholdMinor: 10, TargetPositionID: post.Posting.PositionID, DefaultCapacity: 2, Status: "active"}
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "policy"), Policy: p}); err != nil {
		t.Fatal(err)
	}
	var due string
	if err := s.db.QueryRow(`SELECT world_time FROM scheduler_items WHERE phase_id=? AND status='pending'`, organizationReviewPhase).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, due, 1000); err != nil {
		t.Fatal(err)
	}
	reviews, err := s.ReadOrganizationReviews(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, p.OrganizationID, 10)
	if err != nil || len(reviews) != 1 || reviews[0].Decision.DecisionKind != "expand_capacity" || reviews[0].ScheduleSourceEventID == "" {
		t.Fatalf("scheduled vacancy: %+v %v", reviews, err)
	}
	if err := offer(M2AgentBoID); err != nil {
		t.Fatalf("new slot must admit the existing application: %v", err)
	}
	accept(M2AgentBoID, M2AgentBoPrincipal)
	after := readCareerTestContext(t, s, M2AgentAdaID).Life.HouseholdPressure
	if after == nil || after.PressureLevel != "covered" || after.CoverageGapMinor != 0 || after.ExpectedIncomeMinor != 720 {
		t.Fatalf("second hire did not improve forecast: before=%+v after=%+v", before, after)
	}
	// Stop recurring reviews before advancing to actual employment, then publish
	// the original sourced expansion, not an invented narrative announcement.
	p.AutomaticReview, p.Status = false, "suspended"
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "suspend"), Policy: p}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 8, 0), 1000); err != nil {
		t.Fatal(err)
	}
	publication := RPOrganizationNoticePublishRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "notice"), MessageID: "expansion_notice", CareerEventID: reviews[0].EventID, SpeakerID: M2RPPlayerID}
	if _, err := s.PublishRPOrganizationNotice(ctx, publication); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{M2AgentAdaID, M2AgentBoID, M2RPNPCID} {
		read := allowFixtureControl(t, ctx, s, id)
		list, err := s.ReadRPOrganizationNotices(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		if got := readCareerTestContext(t, s, id).Life.Information; len(got) != 0 {
			t.Fatal("discovery taught notice content before access", id, got)
		}
		if id == M2RPNPCID {
			if len(list.Notices) != 0 {
				t.Fatal("outsider discovered private expansion")
			}
		} else if len(list.Notices) != 1 {
			t.Fatalf("employee cannot discover expansion: %+v", list)
		}
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		b := core.CareerBinding{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "access_" + id}
		_, err = s.AccessRPOrganizationNotice(ctx, RPOrganizationNoticeAccessRequest{Binding: b, SessionID: read.SessionID, MessageID: publication.MessageID})
		if id == M2RPNPCID {
			if !core.HasCode(err, core.CodeNotFound) {
				t.Fatalf("outsider access: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		view, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		b.ExpectedHead, b.IdempotencyKey = view.ObservationCursor, "stance_"+id
		if _, err := s.RecordRPInformationStance(ctx, RPInformationStanceRequest{Binding: b, SessionID: read.SessionID, MessageID: publication.MessageID, Stance: "believe"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.HouseholdPressure; got == nil || got.PressureLevel != "covered" {
		t.Fatalf("restarted household: %+v", got)
	}
	if got := readCareerTestContext(t, s, M2AgentBoID).Life.Information; len(got) != 1 || got[0].Stance != "believe" {
		t.Fatalf("restarted employee knowledge: %+v", got)
	}
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 0 {
		t.Fatal("outsider learned private notice", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("cross-system source recovery: %+v %v", diff, err)
	}
}
