package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPControllerReplacementRequiresReleaseAndFencesOldPrincipal(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "controller-replacement.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _, _ = newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{
		{"principal_replace_operator", "operator"},
		{"principal_replace_old", "service"},
		{"principal_replace_new", "service"},
		{"principal_replace_other", "service"},
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_replace_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("replace-enroll-old"), EntityID: M2AgentBoID, ControllerPrincipalID: "principal_replace_old", ControllerInstanceID: "controller-replace-old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("replace-enroll-other"), EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_replace_other", ControllerInstanceID: "controller-replace-other"}); err != nil {
		t.Fatal(err)
	}
	firstRequest := RPExternalControllerAssignmentRequest{Binding: binding("replace-assign-old"), EntityID: M2AgentBoID, ExpectedGeneration: 0}
	first, err := s.AssignRPExternalControllerLocal(ctx, firstRequest)
	if err != nil || first.Fact.Generation != 1 {
		t.Fatal("first assignment", first, err)
	}
	oldOpen := core.RPSessionOpenRequest{PrincipalID: "principal_replace_old", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "replace-old-session"}
	old, err := s.OpenRPSession(ctx, oldOpen)
	if err != nil {
		t.Fatal(err)
	}
	oldRead := core.RPSessionReadRequest{PrincipalID: oldOpen.PrincipalID, SessionID: old.SessionID}
	view, err := s.ObserveRPSession(ctx, oldRead)
	if err != nil {
		t.Fatal(err)
	}
	oldSpeech := core.RPSpeechRequest{PrincipalID: oldOpen.PrincipalID, SessionID: old.SessionID, Text: "旧控制者已接受的话。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "replace-old-speech"}
	spoken, err := s.SpeakRP(ctx, oldSpeech)
	if err != nil {
		t.Fatal(err)
	}
	oldMoveView, err := s.ObserveRPSession(ctx, oldRead)
	if err != nil {
		t.Fatal(err)
	}
	oldMove := core.RPMoveRequest{PrincipalID: oldOpen.PrincipalID, SessionID: old.SessionID, FromPlaceID: oldMoveView.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: oldMoveView.ObservationCursor, IdempotencyKey: "replace-old-move"}
	moved, err := s.MoveRP(ctx, oldMove)
	if err != nil {
		t.Fatal(err)
	}
	oldMapView, err := s.ObserveRPSession(ctx, oldRead)
	if err != nil {
		t.Fatal(err)
	}
	oldMap := RPMapSurveyRequest{PrincipalID: oldOpen.PrincipalID, SessionID: old.SessionID, ExpectedCursor: oldMapView.ObservationCursor, IdempotencyKey: "replace-old-map"}
	mapReceipt, err := s.SurveyRPMap(ctx, oldMap)
	if err != nil {
		t.Fatal(err)
	}
	pov := "first_person"
	oldStyle := RPStyleSetRequest{PrincipalID: oldOpen.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Scope: "session", SessionID: old.SessionID, ExpectedRevision: 0, IdempotencyKey: "replace-old-style", Patch: core.RPStylePatch{POV: &pov}}
	style, err := s.SetRPStyle(ctx, oldStyle)
	if err != nil {
		t.Fatal(err)
	}
	oldMode := RPInteractionModeSetRequest{PrincipalID: oldOpen.PrincipalID, SessionID: old.SessionID, Mode: "SCENE", ExpectedRevision: 0, IdempotencyKey: "replace-old-mode"}
	mode, err := s.SetRPInteractionMode(ctx, oldMode)
	if err != nil {
		t.Fatal(err)
	}
	replace := RPExternalControllerReplacementRequest{Binding: binding("replace-to-new"), EntityID: M2AgentBoID, ExpectedGeneration: 2, ControllerPrincipalID: "principal_replace_new", ControllerInstanceID: "controller-replace-new"}
	if _, err := s.ReplaceRPExternalControllerLocal(ctx, replace); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("replaced active controller", err)
	}
	if _, err := s.ReleaseRPExternalControllerLocal(ctx, RPExternalControllerReleaseRequest{Binding: binding("replace-release-old"), EntityID: M2AgentBoID, ExpectedGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceRPExternalControllerLocal(ctx, RPExternalControllerReplacementRequest{Binding: binding("replace-stale-generation"), EntityID: M2AgentBoID, ExpectedGeneration: 3, ControllerPrincipalID: "principal_replace_new", ControllerInstanceID: "controller-replace-new"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("stale replacement generation", err)
	}
	if _, err := s.ReplaceRPExternalControllerLocal(ctx, RPExternalControllerReplacementRequest{Binding: binding("replace-duplicate-slot"), EntityID: M2AgentBoID, ExpectedGeneration: 2, ControllerPrincipalID: "principal_replace_other", ControllerInstanceID: "controller-replace-new"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("reused other principal", err)
	}
	if _, err := s.ReplaceRPExternalControllerLocal(ctx, RPExternalControllerReplacementRequest{Binding: binding("replace-same-instance"), EntityID: M2AgentBoID, ExpectedGeneration: 2, ControllerPrincipalID: "principal_replace_new", ControllerInstanceID: "controller-replace-old"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("reused old controller instance", err)
	}
	replace.Binding = binding("replace-to-new")
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "replace rollback") }
	if _, err := s.ReplaceRPExternalControllerLocal(ctx, replace); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("replacement did not roll back", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_external_controller_enrollments WHERE entity_id=? AND principal_id=?`, []any{M2AgentBoID, oldOpen.PrincipalID}, 1)
	replaced, err := s.ReplaceRPExternalControllerLocal(ctx, replace)
	if err != nil || replaced.Fact.OldControllerPrincipalID != oldOpen.PrincipalID || replaced.Fact.ReleasedGeneration != 2 {
		t.Fatal("replace", replaced, err)
	}
	if again, err := s.ReplaceRPExternalControllerLocal(ctx, replace); err != nil || !again.Replayed || again.EventID != replaced.EventID {
		t.Fatal("replacement exact retry", again, err)
	}
	assertM2Value(t, ctx, s, `SELECT generation FROM rp_controller_authorities WHERE entity_id=? AND status='released'`, []any{M2AgentBoID}, 2)
	if _, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_replace_new", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "replace-before-assign"}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("new principal opened before assignment", err)
	}
	if again, err := s.AssignRPExternalControllerLocal(ctx, firstRequest); err != nil || !again.Replayed || again.EventID != first.EventID {
		t.Fatal("historical assignment receipt", again, err)
	}
	next, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("replace-assign-new"), EntityID: M2AgentBoID, ExpectedGeneration: 2})
	if err != nil || next.Fact.Generation != 3 || next.Fact.ControllerPrincipalID != "principal_replace_new" {
		t.Fatal("new principal assignment", next, err)
	}
	newOpen := core.RPSessionOpenRequest{PrincipalID: "principal_replace_new", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "replace-new-session"}
	current, err := s.OpenRPSession(ctx, newOpen)
	if err != nil || current.ControlGeneration != 3 || current.ControllerInstanceID != "controller-replace-new" {
		t.Fatal("new principal session", current, err)
	}
	currentRead := core.RPSessionReadRequest{PrincipalID: newOpen.PrincipalID, SessionID: current.SessionID}
	currentView, err := s.ObserveRPSession(ctx, currentRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: newOpen.PrincipalID, SessionID: current.SessionID, Text: "新控制者的话。", ExpectedCursor: currentView.ObservationCursor, IdempotencyKey: "replace-new-speech"}); err != nil {
		t.Fatal("new principal cannot act", err)
	}
	if again, err := s.SpeakRP(ctx, oldSpeech); err != nil || !again.Replayed || again.EventID != spoken.EventID {
		t.Fatal("old accepted speech lost", again, err)
	}
	if again, err := s.MoveRP(ctx, oldMove); err != nil || !again.Replayed || again.EventID != moved.EventID {
		t.Fatal("old accepted move lost", again, err)
	}
	if outcome, err := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: oldOpen.PrincipalID, SessionID: old.SessionID, Operation: "move", IdempotencyKey: oldMove.IdempotencyKey}); err != nil || outcome.Status != "completed" {
		t.Fatal("historical move retirement receipt", outcome, err)
	}
	if replay, err := s.SurveyRPMap(ctx, oldMap); err != nil || !replay.Replayed || replay.EventID != mapReceipt.EventID {
		t.Fatal("historical map survey receipt", replay, err)
	}
	oldMap.IdempotencyKey = "replace-new-old-map"
	if _, err := s.SurveyRPMap(ctx, oldMap); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old controller made a new map survey", err)
	}
	if replay, err := s.SetRPStyle(ctx, oldStyle); err != nil || !replay.Replayed || replay.RevisionID != style.RevisionID {
		t.Fatal("historical style revision receipt", replay, err)
	}
	if replay, err := s.SetRPInteractionMode(ctx, oldMode); err != nil || !replay.Replayed || replay.Revision != mode.Revision {
		t.Fatal("historical mode revision receipt", replay, err)
	}
	oldStyle.IdempotencyKey = "replace-new-old-style"
	oldStyle.ExpectedRevision = style.Revision
	if _, err := s.SetRPStyle(ctx, oldStyle); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old controller created a new style revision", err)
	}
	oldMode.IdempotencyKey = "replace-new-old-mode"
	oldMode.ExpectedRevision = mode.Revision
	if _, err := s.SetRPInteractionMode(ctx, oldMode); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old controller changed a new mode revision", err)
	}
	if _, err := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: oldOpen.PrincipalID, SessionID: old.SessionID, Operation: "move", IdempotencyKey: "replace-new-old-retirement"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old controller retired a new move key", err)
	}
	oldSpeech.IdempotencyKey = "replace-stale-old-proposal"
	if _, err := s.SpeakRP(ctx, oldSpeech); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old principal made fresh proposal", err)
	}
	if _, err := s.ReadRPSession(ctx, oldRead); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old principal observed through stale session", err)
	}
	for _, principal := range []string{oldOpen.PrincipalID, newOpen.PrincipalID} {
		bindings, err := s.DiscoverRPBindings(ctx, RPDiscoverRequest{PrincipalID: principal})
		if err != nil {
			t.Fatal(err)
		}
		if len(bindings.Bindings) != map[bool]int{true: 1, false: 0}[principal == newOpen.PrincipalID] {
			t.Fatal("discovery after rotation", principal, bindings)
		}
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rotation projection", diff, err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal("reopen rotated world", err)
	}
	if live, err := reopened.ReadRPSession(ctx, currentRead); err != nil || live.ControlGeneration != 3 {
		t.Fatal("new controller after reopen", live, err)
	}
	if again, err := reopened.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: oldOpen.PrincipalID, SessionID: old.SessionID, Text: "旧控制者已接受的话。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "replace-old-speech"}); err != nil || !again.Replayed || again.EventID != spoken.EventID {
		t.Fatal("old receipt after reopen", again, err)
	}
	if diff, err := reopened.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("reopened rotation projection", diff, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_external_controller_enrollments SET controller_instance_id='tampered' WHERE entity_id=?`, M2AgentBoID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("tampered replacement enrollment was not found", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild after replacement", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rebuilt rotation projection", diff, err)
	}
}

func TestRPControllerReplacementCannotInterruptAnotherOpenSharedRound(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "replacement-round.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	human, humanRead, _ := newRPWaitTestSession(t, ctx, s)
	for _, p := range []struct{ id, kind string }{{"principal_rotate_operator", "operator"}, {"principal_rotate_old", "service"}, {"principal_rotate_other", "service"}, {"principal_rotate_new", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, p.id, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_rotate_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	for _, item := range []struct{ entity, principal, instance string }{{M2AgentBoID, "principal_rotate_old", "rotate-old"}, {M2AgentAdaID, "principal_rotate_other", "rotate-other"}} {
		if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-" + item.instance), EntityID: item.entity, ControllerPrincipalID: item.principal, ControllerInstanceID: item.instance}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("assign-" + item.instance), EntityID: item.entity, ExpectedGeneration: 0}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ReleaseRPExternalControllerLocal(ctx, RPExternalControllerReleaseRequest{Binding: binding("release-old"), EntityID: M2AgentBoID, ExpectedGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	other, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_rotate_other", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "other-session"})
	if err != nil {
		t.Fatal(err)
	}
	otherRead := core.RPSessionReadRequest{PrincipalID: "principal_rotate_other", SessionID: other.SessionID}
	for _, read := range []core.RPSessionReadRequest{humanRead, otherRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("round-during-replacement"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{other.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	replace := RPExternalControllerReplacementRequest{Binding: binding("replace-during-round"), EntityID: M2AgentBoID, ExpectedGeneration: 2, ControllerPrincipalID: "principal_rotate_new", ControllerInstanceID: "rotate-new"}
	if _, err := s.ReplaceRPExternalControllerLocal(ctx, replace); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("replacement changed the baseline of another resident's open round", err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("assign-during-round"), EntityID: M2AgentBoID, ExpectedGeneration: 2}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("assignment changed an open round baseline", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: otherRead.PrincipalID, SessionID: otherRead.SessionID, RoundID: round.RoundID}); err != nil || receipt.Status != "open" || receipt.Submitted != 0 {
		t.Fatal("open round changed after rejected rotation", receipt, err)
	}
}
