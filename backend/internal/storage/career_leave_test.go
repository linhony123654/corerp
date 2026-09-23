package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerLeaveCancelsAdoptedWorkPreservesPayAndRecovers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "leave.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	job := accepted.Fact.Employment
	if len(accepted.Fact.AdoptedWorkScheduleIDs) < 3 {
		t.Fatal("acceptance did not explicitly adopt future work")
	}
	r := core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "leave"), LeaveID: "leave_ada", ContractID: job.ContractID, StartDay: 1, EndDay: 3, Reason: "Private family appointment"}
	spoof := r
	spoof.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.RequestCareerLeave(ctx, spoof); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager requested leave on employee's behalf: %v", err)
	}
	requested, err := s.RequestCareerLeave(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	overlap := r
	overlap.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "overlap")
	overlap.LeaveID = "overlap"
	if _, err := s.RequestCareerLeave(ctx, overlap); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("overlap accepted: %v", err)
	}
	approve := core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "approve"), LeaveID: r.LeaveID, Decision: "approve", Notice: "Two days approved with guaranteed base pay."}
	wrong := approve
	wrong.Binding.PrincipalID = M2AgentAdaPrincipal
	if _, err := s.ReviewCareerLeave(ctx, wrong); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("self-approved leave: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "interrupt leave approval") }
	if _, err := s.ReviewCareerLeave(ctx, approve); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("approval interruption: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND activity_code='work' AND world_time>=? AND world_time<? AND status='active'`, []any{job.EmployeeID, careerTime(1, 0, 0), careerTime(3, 0, 0)}, 2)
	approved, err := s.ReviewCareerLeave(ctx, approve)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Fact.Leave.Status != "approved" || len(approved.Fact.Leave.CancelledSchedules) != 2 || approved.Fact.Leave.RequestEventID != requested.EventID {
		t.Fatalf("wrong leave effects: %+v", approved)
	}
	for _, schedule := range approved.Fact.Leave.CancelledSchedules {
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE schedule_id=? AND status='cancelled'`, []any{schedule.ScheduleID}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id=? AND status='cancelled'`, []any{schedule.SchedulerItemID}, 1)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND activity_code<>'work' AND world_time>=? AND world_time<? AND status='active'`, []any{job.EmployeeID, careerTime(1, 0, 0), careerTime(3, 0, 0)}, 2)
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, "leave", r.LeaveID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("private leave leaked: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := s.RequestCareerLeave(ctx, r); err != nil || !retry.Replayed || retry.EventID != requested.EventID {
		t.Fatalf("request retry changed approved state: %+v %v", retry, err)
	}
	if retry, err := s.ReviewCareerLeave(ctx, approve); err != nil || !retry.Replayed || retry.EventID != approved.EventID {
		t.Fatalf("approval retry: %+v %v", retry, err)
	}
	if current, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, "leave", r.LeaveID); err != nil || !reflect.DeepEqual(current, approved) {
		t.Fatalf("leave recovery: %+v %v", current, err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=? AND activity_code='work'`, []any{job.EmployeeID, job.WorkplaceID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND activity_code='work' AND world_time>=? AND world_time<?`, []any{job.EmployeeID, careerTime(1, 0, 0), careerTime(3, 0, 0)}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND activity_code='work' AND world_time>=? AND world_time<? AND status='active'`, []any{job.EmployeeID, careerTime(1, 0, 0), careerTime(3, 0, 0)}, 0)
	attendance, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1)
	if err != nil || attendance.Attendance.Status != "approved_leave" || attendance.Attendance.ExpectedSeconds != 0 || attendance.Attendance.RecordedSeconds != 0 || attendance.Attendance.LeaveEventID != approved.EventID {
		t.Fatalf("approved leave misrepresented as absence: %+v %v", attendance, err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, 12)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id IN (?,?)`, []any{requested.EventID, approved.EventID}, 0)
	own := readCareerTestContext(t, s, job.EmployeeID)
	found := false
	for _, memory := range own.Life.SalientMemories {
		if memory.Kind == "own_leave_decision" && memory.SourceEventID == approved.EventID {
			found = true
		}
	}
	if !found {
		t.Fatal("own approved leave missing from memory")
	}
	other := readCareerTestContext(t, s, M2AgentBoID)
	encoded, _ := json.Marshal(other)
	if strings.Contains(string(encoded), r.Reason) || strings.Contains(string(encoded), approve.Notice) {
		t.Fatal("another NPC automatically knew personal leave")
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=? AND activity_code='work'`, []any{job.EmployeeID, job.WorkplaceID}, 1)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("leave replay: %+v %v", differences, err)
	}
	late := r
	late.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "late")
	late.LeaveID, late.StartDay, late.EndDay = "late", 3, 4
	if _, err := s.RequestCareerLeave(ctx, late); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("retroactive leave accepted: %v", err)
	}
}

func TestCareerLeaveRejectionDoesNotCancelOrReserveWork(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "leave-reject.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	r := core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "request"), LeaveID: "leave", ContractID: accepted.Fact.Employment.ContractID, StartDay: 1, EndDay: 2, Reason: "Personal plans"}
	if _, err := s.RequestCareerLeave(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "reject"), LeaveID: r.LeaveID, Decision: "reject", Notice: "Coverage is not available."}); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND activity_code='work' AND world_time=? AND status='active'`, []any{M2AgentAdaID, careerTime(1, 8, 0)}, 1)
	r.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "new-request")
	r.LeaveID = "new-leave"
	r.EndDay = 3
	if _, err := s.RequestCareerLeave(ctx, r); err != nil {
		t.Fatalf("rejected historical request reserved days: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "late-review"), LeaveID: r.LeaveID, Decision: "approve", Notice: "Too late"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("approval after shift started accepted: %v", err)
	}
	if _, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "close-expired-request"), LeaveID: r.LeaveID, Decision: "reject", Notice: "The original start passed without approval; please reapply for future days."}); err != nil {
		t.Fatal(err)
	}
	r.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "future-request")
	r.LeaveID, r.StartDay = "future-request", 2
	if _, err := s.RequestCareerLeave(ctx, r); err != nil {
		t.Fatalf("expired request permanently reserved future days: %v", err)
	}
}
