package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerRelationshipChangesActualOvertimeChoice(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "career-choice.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
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
	if _, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "apply"), ApplicationID: "application_ada", PositionID: post.Posting.PositionID, CandidateID: M2AgentAdaID, Statement: "Apply for this job."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "invite"), InterviewID: "interview_ada", ApplicationID: "application_ada", Question: "Describe safety checks."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "answer"), InterviewID: "interview_ada", Answer: "Check equipment and exits."}); err != nil {
		t.Fatal(err)
	}
	eval := careerTestEvaluation(t, s, "evaluation", true)
	eval.Binding.PrincipalID = M2RPPlayerPrincipal
	if _, err := s.EvaluateCareerApplication(ctx, eval); err != nil {
		t.Fatal(err)
	}
	offer := careerTestOffer(t, s, "offer", "evaluation")
	offer.Binding.PrincipalID = M2RPPlayerPrincipal
	if _, err := s.OfferCareerEmployment(ctx, offer); err != nil {
		t.Fatal(err)
	}
	job, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-job"), OfferID: "offer", AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 12, 0), 100); err != nil {
		t.Fatal(err)
	}
	var trustedRequest core.CareerOvertimeResponseRequest
	choose := func(id string, day int) CareerRecord {
		t.Helper()
		r := careerTestOvertime(t, s, job.Fact.Employment.ContractID, id)
		r.Binding.PrincipalID = M2RPPlayerPrincipal
		r.Day = day
		if _, err := s.OfferCareerOvertime(ctx, r); err != nil {
			t.Fatal(err)
		}
		request := core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "consider_"+id), OvertimeID: id, Decision: "consider", Reason: "Use my own experience to consider optional work."}
		forged := request
		forged.Binding.PrincipalID = M2RPPlayerPrincipal
		if _, err := s.RespondCareerOvertime(ctx, forged); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("manager delegated employee decision: %v", err)
		}
		if id == "trusted" {
			trustedRequest = request
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "deliberated acceptance interrupted") }
			if _, err := s.RespondCareerOvertime(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("choice rollback: %v", err)
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=?`, []any{M2AgentAdaID, careerTime(1, 13, 0)}, 0)
		}
		got, err := s.RespondCareerOvertime(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	neutral := choose("neutral", 1)
	if neutral.Fact.Overtime.Status != "declined" {
		t.Fatalf("neutral choice: %+v", neutral)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "manager-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	gift := socialRequest(t, ctx, s, read, M2AgentAdaID, "gift", "manager-gift")
	gift.AmountMinor = 1
	if _, err := s.SocialRP(ctx, gift); err != nil {
		t.Fatal(err)
	}
	trusted := choose("trusted", 1)
	if trusted.Fact.Overtime.Status != "accepted" || trusted.Fact.Overtime.Choice == nil || trusted.Fact.Overtime.Choice.ReasonCode != "cooperate_with_trusted_manager" {
		t.Fatalf("trust choice: %+v", trusted)
	}
	view, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, "overtime", "trusted")
	if err != nil || view.Fact.Overtime.Choice != nil {
		t.Fatalf("manager saw private deliberation: %+v %v", view, err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2AgentAdaID, "insult", fmt.Sprintf("conflict_%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	conflicted := choose("conflicted", 2)
	if conflicted.Fact.Overtime.Status != "declined" || conflicted.Fact.Overtime.Choice.ReasonCode != "preserve_boundaries_with_manager" {
		t.Fatalf("conflict choice: %+v", conflicted)
	}
	if replay, err := s.RespondCareerOvertime(ctx, trustedRequest); err != nil || !replay.Replayed || replay.EventID != trusted.EventID || replay.Fact.Overtime.Status != "accepted" {
		t.Fatalf("changed relationship rewrote old decision: %+v %v", replay, err)
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
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.Fact.Employment.ContractID}, 26)
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=2`, []any{job.Fact.Employment.ContractID}, 12)
	view, err = s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, "overtime", "trusted")
	if err != nil || view.Fact.Overtime.Choice == nil {
		t.Fatalf("own deliberation lost after restart: %+v %v", view, err)
	}
}
