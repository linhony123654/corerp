package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func prepareCareerAutoLeaveJob(t *testing.T, s *Store, delay int) CareerRecord {
	t.Helper()
	ctx := context.Background()
	org := careerTestOrg(t, s)
	org.Organization.LeaveReviewPolicy = &core.CareerLeaveReviewPolicy{MaxConcurrentEmployees: 1, AutoReviewDelayMinutes: delay}
	if _, err := s.DefineCareerOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	post := careerTestPosting(t, s)
	if _, err := s.PostCareerPosition(ctx, post); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "apply"), ApplicationID: "application_ada", PositionID: post.Posting.PositionID, CandidateID: M2AgentAdaID, Statement: "Apply"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "invite"), InterviewID: "interview_ada", ApplicationID: "application_ada", Question: "Explain safety checks"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "answer"), InterviewID: "interview_ada", Answer: "Check equipment and exits"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EvaluateCareerApplication(ctx, careerTestEvaluation(t, s, "eval", true)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "offer", "eval")); err != nil {
		t.Fatal(err)
	}
	job, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: "offer", AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestCareerLeaveAutoReviewWaitAuthorityAndRecovery(t *testing.T) {
	for _, mode := range []string{"automatic", "manual-first", "revoked"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "auto-leave.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			job := prepareCareerAutoLeaveJob(t, s, 5)
			r := core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "request"), LeaveID: "leave", ContractID: job.Fact.Employment.ContractID, StartDay: 1, EndDay: 2, Reason: "Private plans"}
			requested, err := s.RequestCareerLeave(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			plan := requested.Fact.Leave.AutoReview
			if plan == nil || plan.WorldTime != "2026-09-22T07:08:00Z" {
				t.Fatalf("missing sourced review plan: %+v", requested)
			}
			if _, err := s.RequestCareerLeave(ctx, r); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id=? AND status='pending'`, []any{plan.SchedulerItemID}, 1)
			if mode == "manual-first" {
				if _, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "manual"), LeaveID: "leave", Decision: "approve", Notice: "Approved before automatic review"}); err != nil {
					t.Fatal(err)
				}
			} else if mode == "revoked" {
				// Deliberately corrupt/revoke only fixture authority to verify the
				// due-time guard. No domain leave/position fact is fabricated.
				if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND capability_id=?`, M2AgentBoPrincipal, careerManageCapability); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET declared_priority=2 WHERE scheduler_item_id=?`, plan.SchedulerItemID); err != nil {
					t.Fatal(err)
				}
				if _, err := s.RunAgentLife(ctx, plan.WorldTime, 100); !core.HasCode(err, core.CodeProjectionDiverged) {
					t.Fatalf("corrupt review queue accepted: %v", err)
				}
				if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
					t.Fatalf("queue corruption not detected: %+v %v", diff, err)
				}
				if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
					t.Fatal(err)
				}
				s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "auto review rollback") }
				if _, err := s.RunAgentLife(ctx, plan.WorldTime, 100); !core.HasCode(err, core.CodeInjectedFailure) {
					t.Fatalf("rollback: %v", err)
				}
				s.beforeCommit = nil
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=?`, []any{"event_" + plan.SchedulerItemID}, 0)
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id=? AND status='pending'`, []any{plan.SchedulerItemID}, 1)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			wait := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: plan.WorldTime, Budget: 1000, IdempotencyKey: "auto-review-wait"}
			if _, err := s.WaitRP(ctx, wait); err != nil {
				t.Fatal(err)
			}
			own, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, "leave", "leave")
			if err != nil {
				t.Fatal(err)
			}
			want := "approved"
			if mode == "revoked" {
				want = "requested"
			}
			if own.Fact.Leave.Status != want || own.Fact.Leave.ReviewAssessment != nil {
				t.Fatalf("automatic decision/privacy: %+v", own)
			}
			kind := "RPCareerFactRecorded"
			if mode != "automatic" {
				kind = "CareerLeaveAutoReviewSkipped"
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND event_type=?`, []any{"event_" + plan.SchedulerItemID, kind}, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{"event_" + plan.SchedulerItemID}, 0)
			if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET status='pending' WHERE scheduler_item_id=?`, plan.SchedulerItemID); err != nil {
				t.Fatal(err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id=? AND status='completed'`, []any{plan.SchedulerItemID}, 1)
			if retry, err := s.WaitRP(ctx, wait); err != nil || !retry.Replayed {
				t.Fatalf("wait replay: %+v %v", retry, err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 1000); err != nil {
				t.Fatal(err)
			}
			attendance, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.Fact.Employment.ContractID, 1)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "revoked" {
				if attendance.Attendance.RecordedSeconds == 0 {
					t.Fatal("revoked manager changed work")
				}
			} else if attendance.Attendance.Status != "approved_leave" {
				t.Fatalf("automatic leave did not affect attendance: %+v", attendance)
			}
			assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.Fact.Employment.ContractID}, 12)
			if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
				t.Fatalf("auto review projection recovery: %+v %v", diff, err)
			}
		})
	}
}

func TestCareerLeaveLateRequestKeepsManualReview(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "late-review.db"))
	defer s.Close()
	job := prepareCareerAutoLeaveJob(t, s, 1440)
	// A one-day review delay cannot meet tomorrow's shift after today's work
	// has started; accepting the request must not queue a retroactive approval.
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "late-session"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(0, 8, 0), Budget: 1000, IdempotencyKey: "late-clock"}); err != nil {
		t.Fatal(err)
	}
	r := core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "late-request"), LeaveID: "late", ContractID: job.Fact.Employment.ContractID, StartDay: 1, EndDay: 2, Reason: "Late personal plans"}
	requested, err := s.RequestCareerLeave(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if requested.Fact.Leave.AutoReview != nil {
		t.Fatalf("review at first shift boundary queued: %+v", requested)
	}
	if requested.WorldTime != careerTime(0, 8, 0) {
		t.Fatalf("late request fixture clock: %s", requested.WorldTime)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=?`, []any{careerLeaveReviewPhase}, 0)
	approved, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "manual-late"), LeaveID: "late", Decision: "consider", Notice: "Timely manual review"})
	if err != nil || approved.Fact.Leave.Status != "approved" {
		t.Fatalf("manual review unavailable: %+v %v", approved, err)
	}
}
