package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerOrganizationLeaveConsiderationActualEffects(t *testing.T) {
	for _, capacity := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "organization-leave.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			// Existing authored world composition, without the optional fixed
			// thirty-day routine: accepted careers supply later work schedules.
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
				command.ExpectedHead = careerTestBinding(t, s, "principal_creator", "head").ExpectedHead
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
			if capacity > 0 {
				org.Organization.LeaveReviewPolicy = &core.CareerLeaveReviewPolicy{MaxConcurrentEmployees: capacity}
			}
			defined, err := s.DefineCareerOrganization(ctx, org)
			if err != nil {
				t.Fatal(err)
			}
			post := careerTestPosting(t, s)
			post.Binding.PrincipalID, post.Posting.Capacity = M2RPPlayerPrincipal, 2
			if _, err := s.PostCareerPosition(ctx, post); err != nil {
				t.Fatal(err)
			}
			jobs := map[string]string{}
			people := []struct{ actor, principal string }{{M2AgentAdaID, M2AgentAdaPrincipal}, {M2AgentBoID, M2AgentBoPrincipal}}
			for _, p := range people {
				if _, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: careerTestBinding(t, s, p.principal, "apply-"+p.actor), ApplicationID: "application-" + p.actor, PositionID: post.Posting.PositionID, CandidateID: p.actor, Statement: "Apply"}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "invite-"+p.actor), InterviewID: "interview-" + p.actor, ApplicationID: "application-" + p.actor, Question: "Explain safety checks"}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, p.principal, "answer-"+p.actor), InterviewID: "interview-" + p.actor, Answer: "Check exits and equipment"}); err != nil {
					t.Fatal(err)
				}
				eval := careerTestEvaluation(t, s, "evaluation-"+p.actor, true)
				eval.Binding.PrincipalID, eval.InterviewID = M2RPPlayerPrincipal, "interview-"+p.actor
				if _, err := s.EvaluateCareerApplication(ctx, eval); err != nil {
					t.Fatal(err)
				}
				offer := careerTestOffer(t, s, "offer-"+p.actor, eval.EvaluationID)
				offer.Binding.PrincipalID, offer.StartsOnDay = M2RPPlayerPrincipal, 2
				if _, err := s.OfferCareerEmployment(ctx, offer); err != nil {
					t.Fatal(err)
				}
				job, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, p.principal, "accept-"+p.actor), OfferID: offer.OfferID, AfterWorkPlaceID: M2AgentCafeID})
				if err != nil {
					t.Fatal(err)
				}
				jobs[p.actor] = job.Fact.Employment.ContractID
			}
			request := func(p int, id string, first, end int) core.CareerLeaveReviewRequest {
				t.Helper()
				person := people[p]
				if _, err := s.RequestCareerLeave(ctx, core.CareerLeaveRequest{Binding: careerTestBinding(t, s, person.principal, id), LeaveID: id, ContractID: jobs[person.actor], StartDay: first, EndDay: end, Reason: "Private plans"}); err != nil {
					t.Fatal(err)
				}
				return core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "review-"+id), LeaveID: id, Decision: "consider", Notice: "Reviewed using the declared organization policy"}
			}
			ada := request(0, "ada-leave", 2, 4)
			if capacity == 0 {
				if _, err := s.ReviewCareerLeave(ctx, ada); !core.HasCode(err, core.CodeBranchConflict) {
					t.Fatalf("unconfigured consideration: %v", err)
				}
				return
			}
			wrong := ada
			wrong.Binding.PrincipalID = M2AgentAdaPrincipal
			if _, err := s.ReviewCareerLeave(ctx, wrong); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("self-review: %v", err)
			}
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "review rollback") }
			if _, err := s.ReviewCareerLeave(ctx, ada); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("rollback: %v", err)
			}
			s.beforeCommit = nil
			if current, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, "leave", ada.LeaveID); err != nil || current.Fact.Leave.Status != "requested" {
				t.Fatalf("rollback changed request: %+v %v", current, err)
			}
			approved, err := s.ReviewCareerLeave(ctx, ada)
			if err != nil || approved.Fact.Leave.Status != "approved" || approved.Fact.Leave.ReviewAssessment.PolicySourceEventID != defined.EventID {
				t.Fatalf("first approval: %+v %v", approved, err)
			}
			bo := request(1, "bo-leave", 3, 4)
			second, err := s.ReviewCareerLeave(ctx, bo)
			if err != nil {
				t.Fatal(err)
			}
			want := "rejected"
			if capacity == 2 {
				want = "approved"
			}
			assessment := second.Fact.Leave.ReviewAssessment
			if second.Fact.Leave.Status != want || assessment.PeakOtherEmployees != 1 || len(assessment.ApprovalSourceEventIDs) != 1 || assessment.ApprovalSourceEventIDs[0] != approved.EventID {
				t.Fatalf("sourced capacity decision: %+v", second)
			}
			for _, p := range people {
				id := ada.LeaveID
				source := approved.EventID
				if p.actor == M2AgentBoID {
					id = bo.LeaveID
					source = second.EventID
				}
				own, err := s.ReadCareerRecruitmentRecord(ctx, p.principal, M2DemoInstanceID, M2DemoBranchID, "leave", id)
				if err != nil || own.Fact.Leave.ReviewAssessment != nil {
					t.Fatalf("manager evidence exposed to employee: %+v %v", own, err)
				}
				input := readCareerTestContext(t, s, p.actor)
				found := false
				for _, memory := range input.Life.SalientMemories {
					if memory.Kind == "own_leave_decision" && memory.SourceEventID == source && core.RPWorkChangeMemory(memory, p.actor) {
						found = true
					}
				}
				if !found {
					t.Fatal("organization decision missing from own eligible work-change memory")
				}
			}
			boundary := request(1, "bo-boundary", 4, 5)
			next, err := s.ReviewCareerLeave(ctx, boundary)
			if err != nil || next.Fact.Leave.Status != "approved" || next.Fact.Leave.ReviewAssessment.PeakOtherEmployees != 0 {
				t.Fatalf("half-open window: %+v %v", next, err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if retry, err := s.ReviewCareerLeave(ctx, bo); err != nil || !retry.Replayed || retry.EventID != second.EventID || retry.Fact.Leave.Status != want {
				t.Fatalf("reconsidered old decision: %+v %v", retry, err)
			}
			bo.Binding = careerTestBinding(t, s, M2RPPlayerPrincipal, "new-key")
			if _, err := s.ReviewCareerLeave(ctx, bo); !core.HasCode(err, core.CodeBranchConflict) {
				t.Fatalf("fresh key reopened decision: %v", err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(5, 0, 1), 1000); err != nil {
				t.Fatal(err)
			}
			for _, p := range people {
				attendance, err := s.ReadCareerAttendance(ctx, p.principal, M2DemoInstanceID, M2DemoBranchID, jobs[p.actor], 3)
				if err != nil {
					t.Fatal(err)
				}
				if p.actor == M2AgentAdaID || capacity == 2 {
					if attendance.Attendance.Status != "approved_leave" || attendance.Attendance.RecordedSeconds != 0 {
						t.Fatalf("approved leave worked: %+v", attendance)
					}
				} else if attendance.Attendance.RecordedSeconds == 0 {
					t.Fatalf("rejection cancelled work: %+v", attendance)
				}
				assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=3`, []any{jobs[p.actor]}, 12)
			}
			if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
				t.Fatalf("review recovery: %+v %v", diff, err)
			}
		})
	}
}
