package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerKnownCoworkerRelationshipChangesActualOvertime(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "coworker.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	// Compose the funded life world without opting into the optional fixed
	// thirty-day demo routine. Career contracts will supply the recurring work.
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
	post.Posting.Capacity = 2
	if _, err := s.PostCareerPosition(ctx, post); err != nil {
		t.Fatal(err)
	}
	jobs := map[string]CareerRecord{}
	for _, person := range []struct{ id, principal string }{{M2AgentAdaID, M2AgentAdaPrincipal}, {M2AgentBoID, M2AgentBoPrincipal}} {
		id, principal := person.id, person.principal
		if _, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: careerTestBinding(t, s, principal, "apply_"+id), ApplicationID: "app_" + id, PositionID: post.Posting.PositionID, CandidateID: id, Statement: "Apply."}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "invite_"+id), InterviewID: "interview_" + id, ApplicationID: "app_" + id, Question: "Explain safety checks."}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, principal, "answer_"+id), InterviewID: "interview_" + id, Answer: "Inspect equipment and exits."}); err != nil {
			t.Fatal(err)
		}
		eval := careerTestEvaluation(t, s, "eval_"+id, true)
		eval.Binding.PrincipalID, eval.InterviewID = M2RPPlayerPrincipal, "interview_"+id
		if _, err := s.EvaluateCareerApplication(ctx, eval); err != nil {
			t.Fatal(err)
		}
		offer := careerTestOffer(t, s, "offer_"+id, eval.EvaluationID)
		offer.Binding.PrincipalID = M2RPPlayerPrincipal
		// Day-one Bo already has a different workplace appointment. Preserve
		// that commitment and begin both new contracts on the following day.
		offer.StartsOnDay = 2
		if _, err := s.OfferCareerEmployment(ctx, offer); err != nil {
			t.Fatal(err)
		}
		job, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, principal, "accept_"+id), OfferID: offer.OfferID, AfterWorkPlaceID: M2AgentCafeID})
		if err != nil {
			t.Fatal(err)
		}
		jobs[id] = job
	}
	// Announcing while Ada is at work and Lin is elsewhere must not teach Ada.
	if _, err := s.RunAgentLife(ctx, careerTime(2, 8, 0), 200); err != nil {
		t.Fatal(err)
	}
	announce := func(id string) CareerRecord {
		t.Helper()
		news, err := s.SpeakCareerAnnouncement(ctx, core.CareerAnnouncementRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, id), AnnouncementID: id, ContractID: jobs[M2AgentBoID].Fact.Employment.ContractID, SpeakerID: M2RPPlayerID})
		if err != nil {
			t.Fatal(err)
		}
		return news
	}
	announce("unheard")
	if _, err := s.RunAgentLife(ctx, careerTime(2, 12, 0), 100); err != nil {
		t.Fatal(err)
	}
	read := allowFixtureControl(t, ctx, s, M2AgentBoID)
	if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2AgentAdaID, "greet", "peer-greet")); err != nil {
		t.Fatal(err)
	}
	// Gift gives a real sourced positive relationship, without announcing a job.
	gift := socialRequest(t, ctx, s, read, M2AgentAdaID, "gift", "peer-gift")
	gift.AmountMinor = 1
	if _, err := s.SocialRP(ctx, gift); err != nil {
		t.Fatal(err)
	}
	var acceptedRequest core.CareerOvertimeResponseRequest
	choose := func(id string, day int) CareerRecord {
		t.Helper()
		offer := careerTestOvertime(t, s, jobs[M2AgentAdaID].Fact.Employment.ContractID, id)
		offer.Binding.PrincipalID, offer.Day = M2RPPlayerPrincipal, day
		if _, err := s.OfferCareerOvertime(ctx, offer); err != nil {
			t.Fatal(err)
		}
		r := core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "consider_"+id), OvertimeID: id, Decision: "consider", Reason: "Consider my own experience."}
		if id == "known" {
			acceptedRequest = r
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "coworker choice interrupted") }
			if _, err := s.RespondCareerOvertime(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("choice rollback: %v", err)
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=?`, []any{M2AgentAdaID, careerTime(day, 13, 0)}, 0)
		}
		got, err := s.RespondCareerOvertime(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := choose("unknown", 2); got.Fact.Overtime.Status != "declined" {
		t.Fatal("unheard coworker identity influenced choice")
	}
	news := announce("heard")
	got := choose("known", 2)
	if got.Fact.Overtime.Status != "accepted" || got.Fact.Overtime.Choice.ReasonCode != "comfortable_with_known_coworker" || got.Fact.Overtime.Choice.CounterpartyEntityID != M2AgentBoID || got.Fact.Overtime.Choice.SourceEventIDs[0] != news.EventID {
		t.Fatalf("known coworker choice: %+v", got.Fact.Overtime)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2AgentAdaID, "insult", fmt.Sprintf("peer-conflict-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	if got := choose("strained", 3); got.Fact.Overtime.Status != "declined" || got.Fact.Overtime.Choice.ReasonCode != "avoid_strained_workplace_relationship" {
		t.Fatalf("strained coworker choice: %+v", got.Fact.Overtime)
	}
	view, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, "overtime", "known")
	if err != nil || view.Fact.Overtime.Choice != nil {
		t.Fatalf("private employee choice: %+v %v", view, err)
	}
	if _, err := s.CloseRPSession(ctx, read); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := s.RespondCareerOvertime(ctx, acceptedRequest); err != nil || !retry.Replayed || retry.EventID != got.EventID {
		t.Fatalf("historical choice changed: %+v %v", retry, err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(4, 0, 1), 1000); err != nil {
		t.Fatal(err)
	}
	contract := jobs[M2AgentAdaID].Fact.Employment.ContractID
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=2`, []any{contract}, 26)
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=3`, []any{contract}, 12)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("recovery: %+v %v", diffs, err)
	}
}
