package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func prepareRPHealthCareerChoice(t *testing.T) (*Store, CareerRecord, RPConditionRecord) {
	t.Helper()
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "health-choice.db"))
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f5_operator','operator','F5 local operator','active')`); err != nil {
		t.Fatal(err)
	}
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"),
		OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	onset, err := s.StartRPConditionLocal(ctx, RPConditionOnsetRequest{Binding: careerTestBinding(t, s, "principal_f5_operator", "illness-onset"),
		ConditionKey: "illness-ada", EntityID: M2AgentAdaID, Kind: "minor_illness", Severity: 2})
	if err != nil {
		t.Fatal(err)
	}
	actor := readCareerTestContext(t, s, M2AgentAdaID)
	if actor.Life.Health == nil || actor.Life.Health.ConditionImpact != "consider_rest_or_leave" ||
		len(actor.Life.Health.Symptoms) != 1 || actor.Life.Health.Symptoms[0] != "malaise" {
		t.Fatal("employee did not perceive symptoms before work/leave choice", actor.Life.Health)
	}
	return s, accepted, onset
}

func TestRPHealthMinorIllnessWorkOrLeaveChoiceKeepsTruthPrivate(t *testing.T) {
	ctx := context.Background()
	t.Run("work", func(t *testing.T) {
		s, accepted, onset := prepareRPHealthCareerChoice(t)
		defer s.Close()
		ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
		view, err := s.ObserveRPSession(ctx, ada)
		if err != nil {
			t.Fatal(err)
		}
		wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
			TargetWorldTime: careerTime(1, 9, 0), Budget: 1000, ExpectedCursor: view.ObservationCursor,
			IdempotencyKey: "illness-go-work"})
		if err != nil || wait.Status != "completed" {
			t.Fatal("ill employee did not reach work", wait, err)
		}
		view, err = s.ObserveRPSession(ctx, ada)
		if err != nil {
			t.Fatal(err)
		}
		attempt, err := s.AttemptRPWorkTask(ctx, RPWorkTaskRequest{Binding: core.CareerBinding{PrincipalID: ada.PrincipalID,
			InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor,
			IdempotencyKey: "illness-work-sample"}, SessionID: ada.SessionID,
			ContractID: accepted.Fact.Employment.ContractID, TaskCode: "routine_check"})
		if err != nil || attempt.Fact.Outcome != "recheck_required" || attempt.Fact.ConditionImpact != "reduced_stamina" ||
			attempt.Fact.FatigueLevel != "" {
			t.Fatal("minor illness did not affect actual work task", attempt, err)
		}
		manager, err := s.ReadRPWorkTask(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, attempt.EventID)
		if err != nil || manager.Outcome != "recheck_required" {
			t.Fatal("manager lacks bounded task result", manager, err)
		}
		encoded, _ := json.Marshal(manager)
		if strings.Contains(string(encoded), onset.EventID) || strings.Contains(string(encoded), onset.Fact.ConditionID) ||
			strings.Contains(string(encoded), "minor_illness") || strings.Contains(string(encoded), "reduced_stamina") {
			t.Fatal("manager learned private illness cause from task result")
		}
		if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 1000); err != nil {
			t.Fatal(err)
		}
		attendance, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID,
			accepted.Fact.Employment.ContractID, 1)
		if err != nil || attendance.Attendance.Status != "complete" || attendance.BaseEarnedMinor != 12 {
			t.Fatal("illness task rewrote attendance/wage", attendance, err)
		}
	})
	t.Run("leave", func(t *testing.T) {
		s, accepted, onset := prepareRPHealthCareerChoice(t)
		defer s.Close()
		contract := accepted.Fact.Employment.ContractID
		reason := "I feel unwell and need a day to rest."
		request := core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "illness-leave"),
			LeaveID: "illness-leave-ada", ContractID: contract, StartDay: 1, EndDay: 2, Reason: reason}
		leave, err := s.RequestCareerLeave(ctx, request)
		if err != nil || leave.Fact.Leave.Status != "requested" || leave.Fact.Leave.Reason != reason {
			t.Fatal("employee did not make an actual leave request", leave, err)
		}
		manager, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, "leave", request.LeaveID)
		if err != nil || manager.Fact.Leave.Reason != reason {
			t.Fatal("manager did not receive employee's submitted reason", manager, err)
		}
		encoded, _ := json.Marshal(manager)
		if strings.Contains(string(encoded), onset.EventID) || strings.Contains(string(encoded), onset.Fact.ConditionID) ||
			strings.Contains(string(encoded), "minor_illness") || strings.Contains(string(encoded), "severity") {
			t.Fatal("leave request disclosed private condition truth")
		}
		if _, err := s.ReadCareerRecruitmentRecord(ctx, M2RPNPCPrincipal, M2DemoInstanceID, M2DemoBranchID, "leave", request.LeaveID); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatal("coworker read private leave reason", err)
		}
		review, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "approve-illness-leave"),
			LeaveID: request.LeaveID, Decision: "approve", Notice: "One day approved."})
		if err != nil || review.Fact.Leave.Status != "approved" {
			t.Fatal("manager did not approve actual leave", review, err)
		}
		if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 1000); err != nil {
			t.Fatal(err)
		}
		attendance, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, contract, 1)
		if err != nil || attendance.Attendance.Status != "approved_leave" || attendance.Attendance.LeaveEventID != review.EventID {
			t.Fatal("approved leave did not replace work schedule", attendance, err)
		}
		if _, err := s.ReadCareerAttendance(ctx, M2RPNPCPrincipal, M2DemoInstanceID, M2DemoBranchID, contract, 1); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatal("coworker read private attendance", err)
		}
		if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
			t.Fatal("illness leave changed sourced projections", diff, err)
		}
	})
}

func TestRPHealthSevereInjuryLimitsTaskUntilRecovery(t *testing.T) {
	ctx := context.Background()
	s, accepted, _ := prepareRPHealthCareerChoice(t)
	defer s.Close()
	injury, err := s.StartRPConditionLocal(ctx, RPConditionOnsetRequest{Binding: careerTestBinding(t, s, "principal_f5_operator", "injury-onset"),
		ConditionKey: "injury-ada", EntityID: M2AgentAdaID, Kind: "minor_injury", Severity: 3})
	if err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	view, err := s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
		TargetWorldTime: careerTime(1, 9, 0), Budget: 1000, ExpectedCursor: view.ObservationCursor,
		IdempotencyKey: "injury-work-time"}); err != nil {
		t.Fatal(err)
	}
	attempt := func(key string) RPWorkTaskRequest {
		view, err := s.ObserveRPSession(ctx, ada)
		if err != nil {
			t.Fatal(err)
		}
		return RPWorkTaskRequest{Binding: core.CareerBinding{PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: key},
			SessionID: ada.SessionID, ContractID: accepted.Fact.Employment.ContractID, TaskCode: "routine_check"}
	}
	if _, err := s.AttemptRPWorkTask(ctx, attempt("blocked-by-injury")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("severe injury did not limit work task", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPWorkTaskAttempted'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected injured task left an Event", count, err)
	}
	if _, err := s.ChangeRPConditionLocal(ctx, RPConditionChangeRequest{Binding: careerTestBinding(t, s, "principal_f5_operator", "injury-resolve"),
		ConditionID: injury.Fact.ConditionID, Status: "resolved", Severity: 0, Reason: "Mobility restored"}); err != nil {
		t.Fatal(err)
	}
	result, err := s.AttemptRPWorkTask(ctx, attempt("after-injury-recovery"))
	if err != nil || result.Fact.Outcome != "recheck_required" || result.Fact.ConditionImpact != "reduced_stamina" {
		t.Fatal("recovered injury did not restore task while illness still affects outcome", result, err)
	}
}

func TestRPHealthVoluntarySymptomSpeechGrantsOnlyHeardClaim(t *testing.T) {
	ctx := context.Background()
	s, _, onset := prepareRPHealthCareerChoice(t)
	defer s.Close()
	lin, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "health-listener"})
	if err != nil {
		t.Fatal(err)
	}
	linRead := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: lin.SessionID}
	adaRead := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	linView, err := s.ObserveRPSession(ctx, linRead)
	if err != nil {
		t.Fatal(err)
	}
	adaView, err := s.ObserveRPSession(ctx, adaRead)
	if err != nil {
		t.Fatal(err)
	}
	if linView.PlaceID != adaView.PlaceID {
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: linRead.PrincipalID, SessionID: linRead.SessionID,
			FromPlaceID: linView.PlaceID, ToPlaceID: adaView.PlaceID, ExpectedCursor: linView.ObservationCursor,
			IdempotencyKey: "lin-visits-ada-health"}); err != nil {
			t.Fatal(err)
		}
	}
	adaView, err = s.ObserveRPSession(ctx, adaRead)
	if err != nil {
		t.Fatal(err)
	}
	spoken := "我今天觉得身体不舒服，想休息一下。"
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID,
		ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "ada-volunteers-symptom", Text: spoken})
	if err != nil {
		t.Fatal(err)
	}
	heard := false
	for _, listener := range speech.ListenerIDs {
		heard = heard || listener == M2RPPlayerID
	}
	if !heard {
		t.Fatal("co-located Lin did not hear voluntary statement", speech)
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if err != nil {
		t.Fatal(err)
	}
	contextView, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: linRead.PrincipalID,
		SessionID: linRead.SessionID, SubjectEntityID: alias})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range contextView.Facts {
		found = found || fact.Kind == "speaker_said" && fact.Text == spoken
	}
	if !found {
		t.Fatal("client context did not preserve heard symptom claim", contextView.Facts)
	}
	encoded, _ := json.Marshal(contextView)
	if strings.Contains(string(encoded), onset.EventID) || strings.Contains(string(encoded), onset.Fact.ConditionID) ||
		strings.Contains(string(encoded), "minor_illness") || strings.Contains(string(encoded), "severity") {
		t.Fatal("heard statement turned into private condition truth")
	}
	eventsView, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: linRead.PrincipalID,
		SessionID: linRead.SessionID, After: onset.EventSequence - 1, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(eventsView)
	if strings.Contains(string(encoded), onset.EventID) || strings.Contains(string(encoded), onset.Fact.ConditionID) ||
		strings.Contains(string(encoded), "RPConditionStarted") {
		t.Fatal("external event feed disclosed private health Event")
	}
	var remote int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`,
		M2AgentBoID, speech.EventID).Scan(&remote); err != nil || remote != 0 {
		t.Fatal("remote manager learned an unheard statement", remote, err)
	}
}
