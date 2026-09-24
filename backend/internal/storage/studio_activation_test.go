package storage

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestStudioActivationPinsConsumersIsolationAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "activation.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	access := StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "activation-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}
	grant, err := s.ConfigureStudioAccessLocal(ctx, access)
	if err != nil {
		t.Fatal(err)
	}
	var firstRequest StudioPackageActivationRequest
	var first privateFactRecord[StudioActivationFact]
	for index, world := range []string{"activation-first", "activation-second"} {
		g := StudioGenesisRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: world, IdempotencyKey: world, Spec: core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "新世界", StartWorldTime: core.StudioWorldStart, Population: 2, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}}}}
		if _, err := s.PrepareStudioWorld(ctx, g); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PrepareStudioParticipants(ctx, g); err != nil {
			t.Fatal(err)
		}
		spatial, err := s.PrepareStudioSpatial(ctx, g)
		if err != nil {
			t.Fatal(err)
		}
		binding := core.CareerBinding{PrincipalID: g.PrincipalID, InstanceID: world, BranchID: "br_main", ExpectedHead: 4, IdempotencyKey: "system"}
		system, narrative := studioTestPackage("system"), studioTestPackage("narrative")
		system.Content.SystemRules.NPCDailyActionBudget = 1 + index
		if index == 1 {
			narrative.Content.NarrativeStyle.Verbosity = "detailed"
		}
		system.Manifest.ContentHash, _ = core.HashJSON(system.Content)
		narrative.Manifest.ContentHash, _ = core.HashJSON(narrative.Content)
		if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: binding, Bundle: system}); err != nil {
			t.Fatal(err)
		}
		binding.ExpectedHead = 5
		binding.IdempotencyKey = "narrative"
		if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: binding, Bundle: narrative}); err != nil {
			t.Fatal(err)
		}
		binding.ExpectedHead = 6
		binding.IdempotencyKey = "activate"
		r := StudioPackageActivationRequest{Genesis: g, Binding: binding, SystemPackageID: system.Manifest.ID, NarrativePackageID: narrative.Manifest.ID}
		if index == 0 {
			missing := r
			missing.SystemPackageID = "not-installed"
			if _, err := s.ActivateStudioPackages(ctx, missing); !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatal("missing package", err)
			}
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "activation rollback") }
			if _, err := s.ActivateStudioPackages(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatal(err)
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rule_epochs WHERE instance_id=? AND end_sequence IS NULL AND epoch_id='epoch_0'`, []any{world}, 1)
			assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{world}, 6)
		}
		activated, err := s.ActivateStudioPackages(ctx, r)
		if err != nil || activated.EventSequence != 7 || activated.Fact.StartSequence != 8 {
			t.Fatal(activated, err)
		}
		if index == 0 {
			firstRequest, first = r, activated
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM event_batches WHERE epoch_id='epoch_0' AND batch_id=(SELECT batch_id FROM events WHERE event_id=?)`, []any{activated.EventID}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rule_epochs WHERE instance_id=? AND epoch_id='epoch_0' AND end_sequence=8`, []any{world}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE instance_id=?`, []any{world}, 0)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM world_instances WHERE instance_id=? AND lifecycle_state='paused'`, []any{world}, 1)
		npc, _ := core.StudioWorldObjectID(world, "entity", "cai")
		// One committed daily-action fixture exercises the existing initiative
		// budget predicate without claiming a production Wait/session workflow.
		binding.ExpectedHead = 7
		binding.IdempotencyKey = "test-budget-used"
		_, err = executePrivateFactCommand(s, ctx, binding, "RPNPCInitiative", struct {
			Actor string `json:"actor"`
		}{npc}, privateFactDomain{"test_budget", "TestBudgetConsumed", `{"authorization":"test-fixture"}`},
			func(conn *sql.Conn) error { return authorizeSavedStudioGenesis(ctx, conn, g) },
			func(conn *sql.Conn, c privateFactContext) (string, func() error, error) {
				return npc, func() error {
					return execAgentOne(ctx, conn, "test historical NPC actor", `UPDATE events SET actor_id=? WHERE event_id=?`, npc, c.EventID)
				}, nil
			})
		if err != nil {
			t.Fatal(err)
		}
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		style, err := resolveRPStyle(ctx, conn, RPSession{InstanceID: world, BranchID: "br_main", ControlledEntityID: spatial.Fact.PlayerEntityID, POV: "second_person"}, nil)
		if err != nil || style.Profile.Verbosity != narrative.Content.NarrativeStyle.Verbosity || len(style.Sources) != 1 || style.Sources[0] != activated.Fact.Lock.Narrative.InstallEventID {
			conn.Close()
			t.Fatal("installed narrative not consumed", style, err)
		}
		terse := "normal"
		overridden, err := resolveRPStyle(ctx, conn, RPSession{InstanceID: world, BranchID: "br_main", ControlledEntityID: spatial.Fact.PlayerEntityID, POV: "second_person"}, &core.RPStylePatch{Verbosity: &terse})
		if err != nil || overridden.Profile.Verbosity != "normal" {
			conn.Close()
			t.Fatal("existing style precedence lost", overridden, err)
		}
		eligible, err := rpInitiativeEligible(ctx, conn, core.RPDecisionInput{InstanceID: world, BranchID: "br_main", NPCEntityID: npc, WorldTime: "2026-09-22T02:00:00Z"})
		conn.Close()
		if err != nil || eligible != (index == 1) {
			t.Fatal("installed budget not consumed", eligible, err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM event_batches WHERE instance_id=? AND first_sequence=8 AND epoch_id=?`, []any{world, activated.Fact.EpochID}, 1)
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
			t.Fatal("activated replay", diffs, err)
		}
		if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
			t.Fatal(err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, grant.EventSequence)
	for _, q := range []string{`UPDATE events SET payload='{}' WHERE event_id=?`, `DELETE FROM events WHERE event_id=?`, `INSERT OR REPLACE INTO events SELECT * FROM events WHERE event_id=?`} {
		if _, err := s.db.ExecContext(ctx, q, first.EventID); err == nil {
			t.Fatal("activation mutable", q)
		}
	}
	// Corrupt the epoch projection: consumers must reject it, never fall back.
	if _, err := s.db.ExecContext(ctx, `UPDATE rule_epochs SET lock_document='{}' WHERE instance_id=? AND epoch_id=?`, firstRequest.Binding.InstanceID, first.Fact.EpochID); err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = readStudioActivePackages(ctx, conn, firstRequest.Binding.InstanceID, "br_main")
	conn.Close()
	if !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("invalid epoch silently accepted", err)
	}
	if diffs, err := s.CompareProjections(ctx, firstRequest.Binding.InstanceID, "br_main"); err != nil || len(diffs) == 0 {
		t.Fatal("epoch corruption missed", diffs, err)
	}
	if err := s.RebuildProjections(ctx, firstRequest.Binding.InstanceID, "br_main"); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, firstRequest.Binding.InstanceID, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("epoch repair", diffs, err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.ActivateStudioPackages(ctx, firstRequest)
	if err != nil || !retry.Replayed || retry.EventID != first.EventID {
		t.Fatal("reopen activation retry", retry, err)
	}
	conn, err = s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active, err := readStudioActivePackages(ctx, conn, firstRequest.Binding.InstanceID, "br_main")
	conn.Close()
	if err != nil || active == nil || active.System.Content.SystemRules.NPCDailyActionBudget != 1 {
		t.Fatal("reopen active content", active, err)
	}
	access.Status = "revoked"
	access.Binding.ExpectedHead = grant.EventSequence
	access.Binding.IdempotencyKey = "revoke-activation"
	if _, err := s.ConfigureStudioAccessLocal(ctx, access); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateStudioPackages(ctx, firstRequest); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("revoked activation retry", err)
	}
}

func TestStudioActivationMigration028Upgrade(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s := openBootstrappedStore(t, ctx, path)
	embedded, err := migrationFiles.ReadFile("migrations/029_studio_package_activation.sql")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile("../../../docs/rp8/schema-029-package-activation.sql")
	if err != nil || !bytes.Equal(embedded, doc) {
		t.Fatal("activation migration contract", err)
	}
	for _, q := range []string{`DROP TRIGGER studio_activation_event_no_update`, `DROP TRIGGER studio_activation_event_no_delete`, `DROP TRIGGER studio_activation_event_no_replace`, `DELETE FROM schema_meta WHERE schema_version='corerp-rp8-package-activation-029-2026-09-24'`} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	for i := 0; i < 2; i++ {
		s, err = Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{StudioActivationSchemaVersion}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name LIKE 'studio_activation_event_no_%'`, nil, 3)
		s.Close()
	}
}
