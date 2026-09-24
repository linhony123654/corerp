package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"corerp.local/backend/internal/core"
)

func studioCreateFixture(world string) StudioCreateRequest {
	return StudioCreateRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: world, IdempotencyKey: world, PlayerPrincipalID: M2RPPlayerPrincipal, SystemPackage: studioTestPackage("system"), NarrativePackage: studioTestPackage("narrative"),
		Spec: core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "创建流程世界", StartWorldTime: core.StudioWorldStart, Population: 2, OpeningMoneyMinor: 20, OpeningStockMinor: 2, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}}}}
}

func TestStudioCreateRecoversEveryStageAndPinsWholePlan(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "create.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "create-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	for stage := 1; stage <= 7; stage++ {
		t.Run(fmt.Sprintf("fail-stage-%d", stage), func(t *testing.T) {
			r := studioCreateFixture(fmt.Sprintf("resume-world-%d", stage))
			commits := 0
			s.beforeCommit = func() error {
				commits++
				if commits == stage {
					return core.NewError(core.CodeInjectedFailure, "stage interrupted")
				}
				return nil
			}
			if _, err := s.CreateStudioWorld(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatal("stage failure", stage, err)
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM world_instances WHERE instance_id=? AND lifecycle_state='active'`, []any{r.InstanceID}, 0)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE instance_id=?`, []any{r.InstanceID}, 0)
			if stage > 1 {
				changed := r
				changed.SystemPackage.Content.SystemRules = &core.StudioSystemRules{NPCDailyActionBudget: 9}
				changed.SystemPackage.Manifest.ContentHash, _ = core.HashJSON(changed.SystemPackage.Content)
				if _, err := s.CreateStudioWorld(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
					t.Fatal("changed plan accepted after partial creation", err)
				}
			}
			// Release and reopen between the fault and continuation.
			s.Close()
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			out, err := s.CreateStudioWorld(ctx, r)
			if err != nil || out.Replayed || out.Status != "ready" || out.EventSequence != 8 {
				t.Fatal("failed to continue staged creation", out, err)
			}
			retry, err := s.CreateStudioWorld(ctx, r)
			if err != nil || !retry.Replayed || retry.ReadyEventID != out.ReadyEventID {
				t.Fatal("complete retry", retry, err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=?`, []any{r.InstanceID}, 8)
			if diffs, err := s.CompareProjections(ctx, r.InstanceID, "br_main"); err != nil || len(diffs) != 0 {
				t.Fatal("workflow replay", diffs, err)
			}
		})
	}
	invalid := studioCreateFixture("invalid-preflight")
	invalid.PlayerPrincipalID = "principal_operator"
	if _, err := s.CreateStudioWorld(ctx, invalid); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM world_instances WHERE instance_id=?`, []any{invalid.InstanceID}, 0)
	// Required dependency ordering is derived, not hardcoded system-first.
	reverse := studioCreateFixture("reverse-dependency")
	reverse.SystemPackage.Manifest.Requires = []core.PackageDependency{{ID: reverse.NarrativePackage.Manifest.ID, Version: reverse.NarrativePackage.Manifest.Version}}
	if _, err := s.CreateStudioWorld(ctx, reverse); err != nil {
		t.Fatal("reverse dependency", err)
	}
	// Concurrent exact calls share all existing receipts; no world-level clone.
	concurrent := studioCreateFixture("concurrent-world")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.CreateStudioWorld(ctx, concurrent); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent exact retry", err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=?`, []any{concurrent.InstanceID}, 8)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, grant.EventSequence)
}
