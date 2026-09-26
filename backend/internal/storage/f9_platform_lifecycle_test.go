package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestF9PlatformDatasetSwapWithoutHostChanges(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "dataset-swap.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()

	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{
		Purpose: "create_world",
		Binding: core.CareerBinding{
			PrincipalID:    "principal_operator",
			InstanceID:     M2DemoInstanceID,
			BranchID:       M2DemoBranchID,
			ExpectedHead:   setup.EventSequence,
			IdempotencyKey: "f9-swap-grant",
		},
		TargetPrincipalID: "principal_creator",
		Status:            "active",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Author a completely different, second dataset for retail content (Bookstore retail)
	swapContent := core.StudioPackageBundle{
		Manifest: core.PackageManifest{
			SchemaVersion: "m0-draft-2026-09-22",
			ID:            "content.bookstore.service",
			Kind:          "content",
			Version:       "1.0.0",
			EngineAPI:     "m0-draft-2026-09-22",
			Requires:      []core.PackageDependency{},
			Optional:      []core.PackageDependency{},
			Capabilities:  []string{"content.career.retail"},
			SchemaHash:    "sha256:0b7d979a3384df06726118c293ec3533e501cd0740f0819067e8e6191c2667a0",
			ContentFiles:  []string{"content.json"},
		},
		Content: core.StudioPackageContent{
			Version: "corerp.studio-package.v1",
			RetailCareer: &core.StudioRetailCareerCatalog{
				Version:        "corerp.retail-career.v1",
				OrganizationID: "actor_m2_coop_employer",
				Jobs: []core.StudioRetailCareerJob{
					{
						PositionID:         "position_bookstore_curator",
						Title:              "Bookstore curator",
						OccupationID:       "cultural_service",
						Grade:              "entry",
						WageReferenceMinor: 22,
						RequiredCredential: core.CredentialRequirement{
							Code:     "bookstore_curation",
							IssuerID: "actor_m2_coop_employer",
						},
						Training: core.StudioRetailCareerTraining{
							ProgramID:      "program_bookstore_curation",
							MinimumMinutes: 45,
							ExercisePrompt: "Catalog modern narrative publications and assist patrons with shelf search.",
						},
						WorkStartHour: 9,
						WorkEndHour:   18,
						NextGrade:     "lead",
					},
				},
			},
		},
	}
	hashContent, err := core.HashJSON(swapContent.Content)
	if err != nil {
		t.Fatal(err)
	}
	swapContent.Manifest.ContentHash = hashContent
	if err := swapContent.Validate(); err != nil {
		t.Fatalf("swap content validation failed: %v", err)
	}

	// 2. Author a completely different, second dataset for narrative style (Chronicle style)
	swapNarrative := core.StudioPackageBundle{
		Manifest: core.PackageManifest{
			SchemaVersion: "m0-draft-2026-09-22",
			ID:            "narrative.city.chronicle",
			Kind:          "narrative",
			Version:       "1.0.0",
			EngineAPI:     "m0-draft-2026-09-22",
			Requires:      []core.PackageDependency{},
			Optional:      []core.PackageDependency{},
			Capabilities:  []string{"narrative.style"},
			SchemaHash:    "sha256:0b7d979a3384df06726118c293ec3533e501cd0740f0819067e8e6191c2667a0",
			ContentFiles:  []string{"narrative.json"},
		},
		Content: core.StudioPackageContent{
			Version: "corerp.studio-package.v1",
			NarrativeStyle: &core.RPStyleProfile{
				Version:              "corerp.style.v1",
				POV:                  "second_person",
				Tense:                "present",
				Verbosity:            "terse",
				NarrativeDensity:     "concise",
				DialogueRatio:        25,
				DescriptionDensity:   60,
				InnerMonologuePolicy: "observed_only",
				ProseInstructions:    "Describe the street actions concisely in second person present tense.",
				ForbiddenPatterns:    []string{},
				NarrativePackRef:     "builtin/plain@1",
			},
		},
	}
	hashNarrative, err := core.HashJSON(swapNarrative.Content)
	if err != nil {
		t.Fatal(err)
	}
	swapNarrative.Manifest.ContentHash = hashNarrative
	if err := swapNarrative.Validate(); err != nil {
		t.Fatalf("swap narrative validation failed: %v", err)
	}

	world := "f9-dataset-swap"
	fixture := studioCreateFixture(world)
	g := StudioGenesisRequest{
		PrincipalID:         fixture.PrincipalID,
		AuthorityInstanceID: fixture.AuthorityInstanceID,
		AuthorityBranchID:   fixture.AuthorityBranchID,
		InstanceID:          world,
		IdempotencyKey:      world,
		Spec:                fixture.Spec,
	}
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

	system := studioTestPackage("system")
	b := core.CareerBinding{PrincipalID: g.PrincipalID, InstanceID: world, BranchID: "br_main"}

	// Install the swapped datasets without any Go/Vue host changes
	for i, bundle := range []core.StudioPackageBundle{system, swapNarrative, swapContent} {
		b.ExpectedHead = int64(4 + i)
		b.IdempotencyKey = "install-swap-" + bundle.Manifest.Kind
		if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: bundle}); err != nil {
			t.Fatalf("failed to install swapped package %s: %v", bundle.Manifest.ID, err)
		}
	}

	// Activate the swapped datasets
	b.ExpectedHead, b.IdempotencyKey = 7, "activate-swap"
	activated, err := s.ActivateStudioPackages(ctx, StudioPackageActivationRequest{
		Genesis:            g,
		Binding:            b,
		SystemPackageID:    system.Manifest.ID,
		NarrativePackageID: swapNarrative.Manifest.ID,
		ContentPackageIDs:  []string{swapContent.Manifest.ID},
	})
	if err != nil {
		t.Fatalf("failed to activate swapped packages: %v", err)
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active, err := readStudioActivePackages(ctx, conn, world, "br_main")
	conn.Close()
	if err != nil || active == nil {
		t.Fatalf("failed to read active swapped packages: %v", err)
	}

	if active.Narrative.Manifest.ID != swapNarrative.Manifest.ID || active.Narrative.Content.NarrativeStyle.POV != "second_person" {
		t.Fatalf("swapped narrative style did not activate: %+v", active.Narrative)
	}
	if len(active.Content) != 1 || active.Content[0].Manifest.ID != swapContent.Manifest.ID || active.Content[0].Content.RetailCareer.Jobs[0].PositionID != "position_bookstore_curator" {
		t.Fatalf("swapped retail content did not activate: %+v", active.Content)
	}

	// Verify idempotency of duplicate install
	b.ExpectedHead = 6
	b.IdempotencyKey = "install-swap-content"
	dup, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: swapContent})
	if err != nil || !dup.Replayed {
		t.Fatalf("duplicate install of swap content must replay idempotently: %+v, err=%v", dup, err)
	}

	_ = activated
}

func TestF9PlatformUpgradeFailurePreservesActiveLock(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade-failure.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()

	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{
		Purpose: "create_world",
		Binding: core.CareerBinding{
			PrincipalID:    "principal_operator",
			InstanceID:     M2DemoInstanceID,
			BranchID:       M2DemoBranchID,
			ExpectedHead:   setup.EventSequence,
			IdempotencyKey: "f9-upgrade-grant",
		},
		TargetPrincipalID: "principal_creator",
		Status:            "active",
	})
	if err != nil {
		t.Fatal(err)
	}

	world := "f9-upgrade-failure"
	fixture := studioCreateFixture(world)
	g := StudioGenesisRequest{
		PrincipalID:         fixture.PrincipalID,
		AuthorityInstanceID: fixture.AuthorityInstanceID,
		AuthorityBranchID:   fixture.AuthorityBranchID,
		InstanceID:          world,
		IdempotencyKey:      world,
		Spec:                fixture.Spec,
	}
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

	system, narrative := studioTestPackage("system"), studioTestPackage("narrative")
	b := core.CareerBinding{PrincipalID: g.PrincipalID, InstanceID: world, BranchID: "br_main"}
	for i, bundle := range []core.StudioPackageBundle{system, narrative} {
		b.ExpectedHead = int64(4 + i)
		b.IdempotencyKey = "install-" + bundle.Manifest.Kind
		if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: bundle}); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Initial valid activation succeeds
	b.ExpectedHead, b.IdempotencyKey = 6, "activate-initial"
	initActivated, err := s.ActivateStudioPackages(ctx, StudioPackageActivationRequest{
		Genesis:            g,
		Binding:            b,
		SystemPackageID:    system.Manifest.ID,
		NarrativePackageID: narrative.Manifest.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Attempt illegal second activation (upgrade on active world) -> must be rejected
	b.ExpectedHead, b.IdempotencyKey = 7, "illegal-hot-upgrade"
	_, err = s.ActivateStudioPackages(ctx, StudioPackageActivationRequest{
		Genesis:            g,
		Binding:            b,
		SystemPackageID:    system.Manifest.ID,
		NarrativePackageID: narrative.Manifest.ID,
	})
	if err == nil || !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("expected CodeBranchConflict for hot upgrade attempt, got: %v", err)
	}

	// 3. Verify original active lock remains completely unchanged
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active, err := readStudioActivePackages(ctx, conn, world, "br_main")
	conn.Close()
	if err != nil || active == nil {
		t.Fatal(err)
	}
	if active.Lock.ActivationEventID != initActivated.EventID || active.Lock.System.ID != system.Manifest.ID {
		t.Fatalf("active lock corrupted by failed upgrade attempt: %+v", active.Lock)
	}
}

func TestF9PlatformDisableUnloadWithHistoricalReferences(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "disable-historical.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()

	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{
		Purpose: "create_world",
		Binding: core.CareerBinding{
			PrincipalID:    "principal_operator",
			InstanceID:     M2DemoInstanceID,
			BranchID:       M2DemoBranchID,
			ExpectedHead:   setup.EventSequence,
			IdempotencyKey: "f9-hist-grant",
		},
		TargetPrincipalID: "principal_creator",
		Status:            "active",
	})
	if err != nil {
		t.Fatal(err)
	}

	world := "f9-historical-world"
	fixture := studioCreateFixture(world)
	g := StudioGenesisRequest{
		PrincipalID:         fixture.PrincipalID,
		AuthorityInstanceID: fixture.AuthorityInstanceID,
		AuthorityBranchID:   fixture.AuthorityBranchID,
		InstanceID:          world,
		IdempotencyKey:      world,
		Spec:                fixture.Spec,
	}
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

	system, narrative := studioTestPackage("system"), studioTestPackage("narrative")
	content := f9RetailBundle(t)
	b := core.CareerBinding{PrincipalID: g.PrincipalID, InstanceID: world, BranchID: "br_main"}
	for i, bundle := range []core.StudioPackageBundle{system, narrative, content} {
		b.ExpectedHead = int64(4 + i)
		b.IdempotencyKey = "install-hist-" + bundle.Manifest.Kind
		if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: bundle}); err != nil {
			t.Fatal(err)
		}
	}

	b.ExpectedHead, b.IdempotencyKey = 7, "activate-hist"
	activated, err := s.ActivateStudioPackages(ctx, StudioPackageActivationRequest{
		Genesis:            g,
		Binding:            b,
		SystemPackageID:    system.Manifest.ID,
		NarrativePackageID: narrative.Manifest.ID,
		ContentPackageIDs:  []string{content.Manifest.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	b.ExpectedHead, b.IdempotencyKey = 8, "save-hist"
	ready, err := s.SaveStudioWorld(ctx, StudioWorldSaveRequest{Genesis: g, Binding: b, PlayerPrincipalID: M2RPPlayerPrincipal})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{
		Binding: core.CareerBinding{
			PrincipalID:    "principal_operator",
			InstanceID:     world,
			BranchID:       "br_main",
			ExpectedHead:   ready.EventSequence,
			IdempotencyKey: "f9-inspect-grant",
		},
		TargetPrincipalID: "principal_creator",
		Status:            "active",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Inspect the event committed under this active epoch
	inspection := StudioEventRequest{PrincipalID: "principal_creator", InstanceID: world, BranchID: "br_main", EventID: ready.EventID}
	evidence1, err := s.ReadStudioEvent(ctx, inspection)
	if err != nil || evidence1.Rule.Packages == nil || len(evidence1.Rule.Packages.Content) != 1 {
		t.Fatalf("first inspection failed: %+v, err=%v", evidence1, err)
	}

	// Close the current rule epoch (simulating package epoch transition / unload for future actions)
	// and advance branch sequence
	if _, err := s.db.ExecContext(ctx, `UPDATE rule_epochs SET end_sequence=20 WHERE instance_id=? AND epoch_id=?`, world, activated.Fact.EpochID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rule_epochs(instance_id,branch_id,epoch_id,start_sequence,ruleset_hash,lock_document) VALUES (?,?,'epoch_future',20,?,'{}')`, world, "br_main", activated.Fact.RulesetHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE branches SET head_sequence=20 WHERE instance_id=? AND branch_id='br_main'`, world); err != nil {
		t.Fatal(err)
	}

	// Historical inspection of ready.EventID MUST still accurately return original pinned packages
	evidenceHistorical, err := s.ReadStudioEvent(ctx, inspection)
	if err != nil || evidenceHistorical.Rule.Packages == nil {
		t.Fatalf("historical event inspection lost package lock: %+v, err=%v", evidenceHistorical, err)
	}
	if !reflect.DeepEqual(evidenceHistorical.Rule.Packages.Lock, activated.Fact.Lock) {
		t.Fatalf("historical package lock changed after epoch transition: got %+v, want %+v", evidenceHistorical.Rule.Packages.Lock, activated.Fact.Lock)
	}
	if evidenceHistorical.Rule.Packages.Content[0].Manifest.ContentHash != content.Manifest.ContentHash {
		t.Fatalf("historical content pin changed: %+v", evidenceHistorical.Rule.Packages.Content[0])
	}
}
