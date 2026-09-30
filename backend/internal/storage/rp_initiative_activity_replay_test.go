package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInitiativeActivityEventRebuildsAndReplays(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "initiative-activity-replay.db")
	s := openBootstrappedStore(t, ctx, path)
	world := "initiative-activity-world"
	read := openConcurrentActivitySession(t, ctx, s, world)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T01:15:00Z", Budget: 100, IdempotencyKey: "initiative-activity-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("activity trigger: %+v %v", wait, err)
	}
	npc, err := core.StudioWorldObjectID(world, "entity", "cai")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: npc, TriggerEventID: wait.EventID}
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "act", ActivityCode: "sweep_court"}, nil
	})
	result, err := s.RunRPInitiative(ctx, request, provider)
	if err != nil || result.Action != "act" || result.EventID == "" {
		t.Fatalf("initiative did not start activity: %+v %v", result, err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND event_type='AgentActivityStarted'`, result.EventID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var activity rpActivityStartedEvent
	if err := json.Unmarshal([]byte(raw), &activity); err != nil || activity.ActorID != npc || activity.DurationMinutes < 1 || activity.ToPlaceID == "" || activity.StartedWorldTime != wait.CurrentWorldTime {
		t.Fatalf("event cannot reconstruct activity: %+v %v", activity, err)
	}
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatalf("initiative activity projection mismatch: %v %v", differences, err)
	}
	if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	replayed, err := s.RunRPInitiative(ctx, request, nil)
	if err != nil || !replayed.Replayed || replayed.EventID != result.EventID {
		t.Fatalf("initiative replay differs: %+v %v", replayed, err)
	}
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatalf("reopened world diverged: %v %v", differences, err)
	}
}
