package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func f9RetailBundle(t *testing.T) core.StudioPackageBundle {
	t.Helper()
	base := "../../../docs/f9/retail-career"
	manifest, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(base, "content.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle core.StudioPackageBundle
	if err := json.Unmarshal(manifest, &bundle.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &bundle.Content); err != nil {
		t.Fatal(err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("author content hash/capability: %v", err)
	}
	return bundle
}

func TestF9RetailContentInstallationActivationPinsAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "retail-content.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "f9-content-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	world := "f9-retail-content"
	fixture := studioCreateFixture(world)
	g := StudioGenesisRequest{PrincipalID: fixture.PrincipalID, AuthorityInstanceID: fixture.AuthorityInstanceID, AuthorityBranchID: fixture.AuthorityBranchID, InstanceID: world, IdempotencyKey: world, Spec: fixture.Spec}
	for _, prepare := range []func(context.Context, StudioGenesisRequest) error{
		func(ctx context.Context, g StudioGenesisRequest) error {
			_, err := s.PrepareStudioWorld(ctx, g)
			return err
		},
		func(ctx context.Context, g StudioGenesisRequest) error {
			_, err := s.PrepareStudioParticipants(ctx, g)
			return err
		},
		func(ctx context.Context, g StudioGenesisRequest) error {
			_, err := s.PrepareStudioSpatial(ctx, g)
			return err
		},
	} {
		if err := prepare(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	content := f9RetailBundle(t)
	system, narrative := studioTestPackage("system"), studioTestPackage("narrative")
	b := core.CareerBinding{PrincipalID: g.PrincipalID, InstanceID: world, BranchID: "br_main", ExpectedHead: 4}
	for i, bundle := range []core.StudioPackageBundle{system, narrative, content} {
		b.ExpectedHead = int64(4 + i)
		b.IdempotencyKey = "f9-install-" + bundle.Manifest.Kind
		if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: bundle}); err != nil {
			t.Fatal(err)
		}
	}
	b.ExpectedHead, b.IdempotencyKey = 7, "f9-activate"
	a := StudioPackageActivationRequest{Genesis: g, Binding: b, SystemPackageID: system.Manifest.ID, NarrativePackageID: narrative.Manifest.ID, ContentPackageIDs: []string{content.Manifest.ID}}
	missing := a
	missing.ContentPackageIDs = []string{"missing.retail"}
	if _, err := s.ActivateStudioPackages(ctx, missing); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("missing selected content accepted", err)
	}
	duplicate := a
	duplicate.ContentPackageIDs = []string{content.Manifest.ID, content.Manifest.ID}
	if _, err := s.ActivateStudioPackages(ctx, duplicate); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("duplicate selected content accepted", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{world}, 7)
	activated, err := s.ActivateStudioPackages(ctx, a)
	if err != nil || len(activated.Fact.Lock.Content) != 1 {
		t.Fatal("content activation", activated, err)
	}
	if retry, err := s.ActivateStudioPackages(ctx, a); err != nil || !retry.Replayed || retry.EventID != activated.EventID {
		t.Fatal("activation retry", retry, err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active, err := readStudioActivePackages(ctx, conn, world, "br_main")
	conn.Close()
	if err != nil || active == nil || len(active.Content) != 1 || active.Content[0].Manifest.ID != content.Manifest.ID || active.Content[0].Content.RetailCareer.Jobs[0].WageReferenceMinor != 16 {
		t.Fatal("pinned retail source unavailable", active, err)
	}
	b.ExpectedHead, b.IdempotencyKey = 8, "f9-save"
	ready, err := s.SaveStudioWorld(ctx, StudioWorldSaveRequest{Genesis: g, Binding: b, PlayerPrincipalID: M2RPPlayerPrincipal})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: world, BranchID: "br_main", ExpectedHead: ready.EventSequence, IdempotencyKey: "f9-retail-inspect"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	inspection := StudioEventRequest{PrincipalID: "principal_creator", InstanceID: world, BranchID: "br_main", EventID: ready.EventID}
	evidence, err := s.ReadStudioEvent(ctx, inspection)
	if err != nil || evidence.Rule.Packages == nil || len(evidence.Rule.Packages.Content) != 1 || evidence.Rule.Packages.Content[0].Manifest.ContentHash != content.Manifest.ContentHash || evidence.Rule.Packages.Lock.Content[0].InstallEventID == "" {
		t.Fatal("creator cannot inspect historical content provenance", evidence, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	conn, err = s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active, err = readStudioActivePackages(ctx, conn, world, "br_main")
	conn.Close()
	if err != nil || active == nil || len(active.Content) != 1 || active.Content[0].Manifest.ContentHash != content.Manifest.ContentHash {
		t.Fatal("restarted content pin", active, err)
	}
	evidence, err = s.ReadStudioEvent(ctx, inspection)
	if err != nil || evidence.Rule.Packages == nil || len(evidence.Rule.Packages.Content) != 1 || evidence.Rule.Packages.Content[0].Manifest.ContentHash != content.Manifest.ContentHash {
		t.Fatal("restarted historical content provenance", evidence, err)
	}
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("content activation replay", diffs, err)
	}
}
