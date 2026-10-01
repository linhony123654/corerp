package httpapi

import (
	"context"
	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type interruptionHTTPProviderFunc func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error)

func (f interruptionHTTPProviderFunc) Propose(ctx context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
	return f(ctx, in)
}

func TestRPActionInterruptionHTTPGenericOutcomeAndReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "http-interruption.db")
	store, handler := openHTTPTestServer(t, ctx, path)
	defer store.Close()
	session := openAuthoredRPModelHTTPSession(t, ctx, store, handler)
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, row := range []struct{ id, kind string }{{"principal_http_interrupt_operator", "operator"}, {"principal_http_interrupt_service", "service"}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	response := performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	initial := decodeData[storage.RPObservation](t, response)
	if len(initial.PresentEntities) != 1 {
		t.Fatal("fixture target count", initial.PresentEntities)
	}
	target := initial.PresentEntities[0].EntityID
	var rawTarget string
	if err := db.QueryRowContext(ctx, `SELECT agent_id FROM agent_profiles WHERE instance_id=? AND branch_id=? AND agent_id<>? AND status='active'`, session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(&rawTarget); err != nil {
		t.Fatal(err)
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, session.InstanceID, session.BranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_http_interrupt_operator", InstanceID: session.InstanceID, BranchID: session.BranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := store.EnrollRPExternalControllerLocal(ctx, storage.RPExternalControllerEnrollmentRequest{Binding: binding("http-interrupt-enroll"), EntityID: rawTarget, ControllerPrincipalID: "principal_http_interrupt_service", ControllerInstanceID: "http-private-controller"}); err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	initial = decodeData[storage.RPObservation](t, response)
	calls := 0
	var fence string
	service, err := storage.NewRPService(store, interruptionHTTPProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		assigned, err := store.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{Binding: binding("http-interrupt-assign"), EntityID: rawTarget, ExpectedGeneration: 0})
		fence = assigned.EventID
		return core.RPDecisionProposal{Action: "respond", Text: "不应提交。"}, err
	}), "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	handler.(*Server).service = service
	request := core.RPNonverbalRequest{SessionID: session.SessionID, Action: "nod", TargetEntityID: target, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "http-interrupt-action"}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	out := decodeData[storage.RPNonverbalResult](t, response)
	if calls != 1 || out.Turn == nil || out.Turn.Status != "settled" || out.Turn.Interruption == nil || out.Turn.Interruption.Code != "world_changed" {
		t.Fatal("HTTP interrupted outcome", out, calls)
	}
	for _, secret := range []string{fence, "RPExternalControllerAssigned", "http-private-controller", "principal_http_interrupt_service", "不应提交。", "interrupted_listener_ids"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("HTTP exposes private fence %q", secret)
		}
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	saved := decodeData[storage.RPNonverbalResult](t, response)
	if calls != 1 || saved.Turn == nil || !saved.Turn.Replayed || saved.EventID != out.EventID || saved.Turn.Interruption == nil {
		t.Fatal("HTTP replay repeated provider", saved, calls)
	}
}
