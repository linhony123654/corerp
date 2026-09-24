package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func studioTestPackage(kind string) core.StudioPackageBundle {
	m := core.PackageManifest{SchemaVersion: core.StudioPackageManifestVersion, ID: "test." + kind, Kind: kind, Version: "1.0.0", EngineAPI: core.StudioPackageManifestVersion, Requires: []core.PackageDependency{}, Optional: []core.PackageDependency{}, SchemaHash: core.StudioPackageSchemaHash}
	c := core.StudioPackageContent{Version: core.StudioPackageContentVersion}
	if kind == "system" {
		c.SystemRules = &core.StudioSystemRules{NPCDailyActionBudget: 4}
		m.Capabilities = []string{"rules.npc.daily_budget"}
		m.ContentFiles = []string{"system.json"}
	} else {
		style := core.DefaultRPStyle()
		style.Verbosity = "terse"
		c.NarrativeStyle = &style
		m.Capabilities = []string{"narrative.style"}
		m.ContentFiles = []string{"narrative.json"}
	}
	m.ContentHash, _ = core.HashJSON(c)
	return core.StudioPackageBundle{Manifest: m, Content: c}
}

func TestStudioPackageInstallationScopedAtomicImmutableAndRecoverable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "packages.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	access := StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "package-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}
	grant, err := s.ConfigureStudioAccessLocal(ctx, access)
	if err != nil {
		t.Fatal(err)
	}
	spec := core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "包测试世界", StartWorldTime: core.StudioWorldStart, Population: 3, OpeningMoneyMinor: 11, OpeningStockMinor: 5, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "square"}}}
	genesis := StudioGenesisRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: "package-world", IdempotencyKey: "package-world", Spec: spec}
	for _, world := range []string{"package-world", "other-package-world"} {
		g := genesis
		g.InstanceID = world
		g.IdempotencyKey = world
		if _, err := s.PrepareStudioWorld(ctx, g); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PrepareStudioParticipants(ctx, g); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PrepareStudioSpatial(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	system := studioTestPackage("system")
	narrative := studioTestPackage("narrative")
	narrative.Manifest.Requires = []core.PackageDependency{{ID: system.Manifest.ID, Version: system.Manifest.Version}}
	r := StudioPackageInstallRequest{Genesis: genesis, Binding: core.CareerBinding{PrincipalID: genesis.PrincipalID, InstanceID: genesis.InstanceID, BranchID: "br_main", ExpectedHead: 4, IdempotencyKey: "install-system"}, Bundle: system}
	missing := r
	missing.Bundle = narrative
	if _, err := s.InstallStudioPackage(ctx, missing); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("missing exact dependency", err)
	}
	wrong := r
	wrong.Binding.InstanceID = "other-package-world"
	if _, err := s.InstallStudioPackage(ctx, wrong); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("foreign binding", err)
	}
	tampered := r
	tampered.Bundle.Content.SystemRules = &core.StudioSystemRules{NPCDailyActionBudget: 9}
	if _, err := s.InstallStudioPackage(ctx, tampered); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("tampered content", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "package rollback") }
	if _, err := s.InstallStudioPackage(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal(err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{genesis.InstanceID}, 4)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='StudioPackageInstalled'`, []any{genesis.InstanceID}, 0)
	first, err := s.InstallStudioPackage(ctx, r)
	if err != nil || first.Replayed || first.EventSequence != 5 {
		t.Fatal(first, err)
	}
	narrativeRequest := r
	narrativeRequest.Bundle = narrative
	narrativeRequest.Binding.ExpectedHead = 5
	narrativeRequest.Binding.IdempotencyKey = "install-narrative"
	second, err := s.InstallStudioPackage(ctx, narrativeRequest)
	if err != nil || second.EventSequence != 6 {
		t.Fatal(second, err)
	}
	mismatch := r
	mismatch.Bundle.Manifest.Version = "1.0.1"
	if _, err := s.InstallStudioPackage(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed exact retry", err)
	}
	duplicate := r
	duplicate.Binding.ExpectedHead = 6
	duplicate.Binding.IdempotencyKey = "duplicate"
	if _, err := s.InstallStudioPackage(ctx, duplicate); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("duplicate identity", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, grant.EventSequence)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM world_instances WHERE instance_id=? AND lifecycle_state='paused'`, []any{genesis.InstanceID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE instance_id=?`, []any{genesis.InstanceID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id IN (?,?)`, []any{first.EventID, second.EventID}, 0)
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload='{}' WHERE event_id=?`, first.EventID); err == nil {
		t.Fatal("installed content was mutable")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE event_id=?`, first.EventID); err == nil {
		t.Fatal("installed content was deletable")
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO events SELECT * FROM events WHERE event_id=?`, first.EventID); err == nil {
		t.Fatal("installed content replace bypass")
	}
	if diffs, err := s.CompareProjections(ctx, genesis.InstanceID, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("installed replay", diffs, err)
	}
	if err := s.RebuildProjections(ctx, genesis.InstanceID, "br_main"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := readStudioInstalledPackages(ctx, s.db, genesis.InstanceID, "br_main")
	if err != nil || len(installed) != 2 || installed[system.Manifest.ID].Fact.Bundle.Content.SystemRules.NPCDailyActionBudget != 4 || installed[narrative.Manifest.ID].Fact.Bundle.Content.NarrativeStyle.Verbosity != "terse" {
		t.Fatal("saved package content", installed, err)
	}
	other, err := readStudioInstalledPackages(ctx, s.db, "other-package-world", "br_main")
	if err != nil || len(other) != 0 {
		t.Fatal("cross-world package inheritance", other, err)
	}
	retry, err := s.InstallStudioPackage(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != first.EventID {
		t.Fatal("reopen retry", retry, err)
	}
	access.Status = "revoked"
	access.Binding.ExpectedHead = grant.EventSequence
	access.Binding.IdempotencyKey = "revoke-package"
	if _, err := s.ConfigureStudioAccessLocal(ctx, access); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallStudioPackage(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("revoked retry", err)
	}
}

func TestStudioPackageMigration027UpgradeAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s := openBootstrappedStore(t, ctx, path)
	embedded, err := migrationFiles.ReadFile("migrations/028_studio_package_content.sql")
	if err != nil {
		t.Fatal(err)
	}
	documented, err := os.ReadFile("../../../docs/rp8/schema-028-package-content.sql")
	if err != nil || !bytes.Equal(embedded, documented) {
		t.Fatal("migration contract drift", err)
	}
	var before int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// Recreate the immediately preceding schema in this disposable database.
	for _, q := range []string{`DROP TRIGGER studio_package_event_no_update`, `DROP TRIGGER studio_package_event_no_delete`, `DROP TRIGGER studio_package_event_no_replace`, `DELETE FROM schema_meta WHERE schema_version='corerp-rp8-package-content-028-2026-09-24'`} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		s, err = Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{StudioPackageSchemaVersion}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events`, nil, before)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name IN ('studio_package_event_no_update','studio_package_event_no_delete','studio_package_event_no_replace')`, nil, 3)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
