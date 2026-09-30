package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func f9LifeJournalBundle(t *testing.T) core.StudioPackageBundle {
	t.Helper()
	base := "../../../docs/f9/life-journal"
	manifest, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(base, "narrative.json"))
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
		t.Fatalf("authored Narrative Pack invalid: %v", err)
	}
	return bundle
}

func TestF9LifeJournalInstalledActivatedAndRegeneratedWithoutWorldMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "life-journal.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "f9-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	r := studioCreateFixture("f9-life-journal")
	r.NarrativePackage = f9LifeJournalBundle(t)
	created, err := s.CreateStudioWorld(ctx, r)
	if err != nil || created.Status != "ready" {
		t.Fatal(created, err)
	}
	if retry, err := s.CreateStudioWorld(ctx, r); err != nil || !retry.Replayed || retry.ReadyEventID != created.ReadyEventID {
		t.Fatal("exact create retry", retry, err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: r.InstanceID, BranchID: "br_main", EntityID: created.EntityID, POV: "second_person", IdempotencyKey: "f9-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	style, err := s.ReadRPStyle(ctx, read)
	if err != nil || !reflect.DeepEqual(style.Profile, *r.NarrativePackage.Content.NarrativeStyle) || len(style.Sources) != 1 {
		t.Fatal("authored profile not active in player session", style, err)
	}
	initial, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "f9-turn", Text: "今天我记下了自己的第一句话。"})
	if err != nil || turn.Status != "settled" {
		t.Fatal("real committed turn", turn, err)
	}
	narrative := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}
	journal, err := service.ReadRPNarrative(ctx, narrative)
	if err != nil || len(journal.View.Lines) == 0 || journal.Style.Profile.POV != "first_person" || journal.Style.Profile.NarrativeDensity != "long" {
		t.Fatal("journal rendering", journal, err)
	}
	otherPOV, otherDensity := "second_person", "standard"
	narrative.StyleOverride = &core.RPStylePatch{POV: &otherPOV, NarrativeDensity: &otherDensity}
	variant, err := service.ReadRPNarrative(ctx, narrative)
	if err != nil || reflect.DeepEqual(journal.View.Lines, variant.View.Lines) || !reflect.DeepEqual(journal.View.EventIDs, variant.View.EventIDs) {
		t.Fatal("same sourced result must permit different presentation", variant, err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{r.InstanceID}, turn.SettledSequence)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	service, err = NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := service.ObserveRPSession(ctx, read)
	if err != nil || len(reloaded.RecentTurns) == 0 || !reflect.DeepEqual(variant.View.Lines, reloaded.RecentTurns[len(reloaded.RecentTurns)-1].NarrativeLines) || variant.View.RenderID != reloaded.RecentTurns[len(reloaded.RecentTurns)-1].RenderID {
		t.Fatal("restarted selected presentation", reloaded.RecentTurns, err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{r.InstanceID}, turn.SettledSequence)
	if diffs, err := s.CompareProjections(ctx, r.InstanceID, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("restarted world projection", diffs, err)
	}
}
