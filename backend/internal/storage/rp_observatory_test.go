package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObservatoryScopesReplayAndProjectionHealth(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "observatory.db"))
	defer s.Close()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "observatory-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	const world = "observatory-world"
	create := studioCreateFixture(world)
	create.Spec.Population = 3
	create.Spec.OpeningStockMinor = 3
	create.Spec.People = []core.StudioWorldPerson{
		{Key: "lin", Name: "Lin", Place: "home", Player: true},
		{Key: "bo", Name: "Bo", Place: "home"},
		{Key: "cai", Name: "Cai", Place: "home"},
	}
	create.Spec.Acquaintances = [][2]string{{"lin", "bo"}}
	create.SystemPackage.Content.SystemRules.RPExecutionMode = "orchestrated"
	create.SystemPackage.Content.SystemRules.MaxActiveResponders = 1
	create.SystemPackage.Manifest.ContentHash, _ = core.HashJSON(create.SystemPackage.Content)
	if _, err := s.CreateStudioWorld(ctx, create); err != nil {
		t.Fatal(err)
	}
	player, _ := core.StudioWorldObjectID(world, "entity", "lin")
	bo, _ := core.StudioWorldObjectID(world, "entity", "bo")
	cai, _ := core.StudioWorldObjectID(world, "entity", "cai")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: "observatory-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	provider := rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		if input.NPCEntityID == bo {
			return core.RPDecisionProposal{Action: "respond", Text: "我在。"}, nil
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "observatory-first", Text: "Bo，请回答。"}, provider); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "observatory-second", Text: "继续。"}, provider); err != nil {
		t.Fatal(err)
	}
	request := RPObservatoryRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Limit: 1}
	latest, err := s.ReadRPObservatory(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest.Traces) != 1 || latest.NextBeforeSequence == 0 || latest.ProjectionHealth.Status != "healthy" || latest.ProjectionHealth.DifferenceCount != 0 {
		t.Fatalf("wrong latest observatory page: %+v", latest)
	}
	request.BeforeSequence = latest.NextBeforeSequence
	older, err := s.ReadRPObservatory(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Traces) != 1 || older.Traces[0].PlayerAction != "Bo，请回答。" || older.Traces[0].ExecutionMode != "orchestrated" || older.Traces[0].ResponderLimit != 1 {
		t.Fatalf("wrong historical observatory page: %+v", older)
	}
	trace := older.Traces[0]
	if trace.TraceID == "" || !strings.HasPrefix(trace.TraceID, "trace_") || len(trace.Activations) != 2 || len(trace.CommittedActions) != 1 || trace.CommittedActions[0].DisplayName != "Bo" || trace.CommittedActions[0].Detail != "我在。" {
		t.Fatalf("observatory omitted public turn evidence: %+v", trace)
	}
	unknownSeen := false
	for _, activation := range trace.Activations {
		if !strings.HasPrefix(activation.ActorRef, "actor_") {
			t.Fatalf("canonical actor reference escaped: %+v", activation)
		}
		if activation.DisplayName == "陌生人" {
			unknownSeen = true
		}
	}
	if !unknownSeen {
		t.Fatalf("unfamiliar listener identity leaked: %+v", trace.Activations)
	}
	encoded, err := json.Marshal(older)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{player, bo, cai, "event_", "turn_rp_", "principal_"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("observatory leaked canonical/private token %q: %s", secret, encoded)
		}
	}
	if _, err := s.ReadRPObservatory(ctx, RPObservatoryRequest{PrincipalID: "principal_creator", SessionID: session.SessionID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("other principal read player observatory: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_positions SET activity_code='damaged' WHERE agent_id=?`, player); err != nil {
		t.Fatal(err)
	}
	degraded, err := s.ReadRPObservatory(ctx, RPObservatoryRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if degraded.ProjectionHealth.Status != "degraded" || degraded.ProjectionHealth.DifferenceCount == 0 {
		t.Fatalf("projection damage was not reduced to safe aggregate health: %+v", degraded.ProjectionHealth)
	}
}
