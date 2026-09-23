package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPPlaySetupMaterializesPlayerAndThirdNPCInExistingWorld(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-play-setup.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := store.BootstrapRPPlayDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if setup.EventSequence != 7 || setup.PlayerID != M2RPPlayerID || setup.NPCCount != 3 || setup.PlaceCount != 5 {
		t.Fatalf("unexpected RP fixture: %+v", setup)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_profiles`, nil, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_places`, nil, 5)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM materialized_entities WHERE status = 'active'`, nil, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_positions WHERE place_id = ?`, []any{M2AgentCafeID}, 2)
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("RP fixture diverged from existing event replay: %v, %v", differences, err)
	}
	snapshot, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, setup.EventSequence)
	if err != nil {
		t.Fatal(err)
	}
	continued, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, setup.EventSequence)
	if err != nil || continued.StateHash != snapshot.StateHash {
		t.Fatalf("RP fixture snapshot replay diverged: %+v, %v", continued, err)
	}
	opened, err := store.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "play-setup-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: opened.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if view.PlaceID != M2AgentCafeID || view.WorldTime != m2RPSetupTime || len(view.PresentEntities) != 1 || view.PresentEntities[0].EntityID != M2RPNPCID {
		t.Fatalf("actual RP player did not observe co-located NPC: %+v", view)
	}
	if _, err := store.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		EntityID: M2RPNPCID, POV: "second_person", IdempotencyKey: "steal-npc",
	}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("player controlled NPC without grant: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := reopened.BootstrapRPPlayDemo(ctx)
	if err != nil || !replay.Replayed || replay.EventSequence != setup.EventSequence {
		t.Fatalf("RP bootstrap retry changed world: %+v, %v", replay, err)
	}
	view, err = reopened.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: opened.SessionID})
	if err != nil || view.PlaceID != M2AgentCafeID || len(view.PresentEntities) != 1 {
		t.Fatalf("RP fixture did not survive restart: %+v, %v", view, err)
	}
}
