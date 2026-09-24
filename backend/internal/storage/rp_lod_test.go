package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPHotRosterUsesActualDecisionsAndRecovers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hot-roster.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.RunAgentLife(ctx, careerTime(1, 12, 0), 1000); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "hot-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	waitRequest := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 12, 15), Budget: 1000, IdempotencyKey: "hot-wait"}
	wait, err := s.WaitRP(ctx, waitRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(wait.InitiativeNPCIDs) < 2 {
		t.Fatalf("fixture needs multiple actual scene NPCs: %+v", wait)
	}
	actor := wait.InitiativeNPCIDs[0]
	calls := 0
	provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if in.NPCEntityID != actor || in.Life == nil {
			t.Fatal("HOT lost full own context")
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: actor, TriggerEventID: wait.EventID}
	decision, err := s.RunRPInitiative(ctx, request, provider)
	if err != nil || decision.EventID == "" || calls != 1 {
		t.Fatalf("actual decision: %+v %v calls=%d", decision, err, calls)
	}
	roster := func() []string {
		t.Helper()
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readRPHotInitiativeRoster(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID, view.PlaceID, M2RPPlayerID)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	after := roster()
	if len(after) != len(wait.InitiativeNPCIDs) || after[len(after)-1] != actor {
		t.Fatalf("oldest-first roster did not rotate: %v -> %v", wait.InitiativeNPCIDs, after)
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
	if !reflect.DeepEqual(after, roster()) {
		t.Fatal("history-derived priority changed after recovery")
	}
	retry, err := s.WaitRP(ctx, waitRequest)
	if err != nil || !retry.Replayed || !reflect.DeepEqual(retry.InitiativeNPCIDs, wait.InitiativeNPCIDs) {
		t.Fatalf("retry recalculated pinned HOT roster: %+v %v", retry, err)
	}
	if _, err := s.RunRPInitiative(ctx, request, provider); err != nil || calls != 1 {
		t.Fatalf("recovery repeated provider: %v calls=%d", err, calls)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("HOT recovery: %+v %v", differences, err)
	}
}
