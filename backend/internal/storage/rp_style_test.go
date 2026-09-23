package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

// Only temporary test databases: reconstruct the older application schema,
// preserving authoritative world/speech/NPC rows for real forward migration.
func removeRPStyleSchemaForUpgradeTest(t *testing.T, ctx context.Context, s *Store) {
	t.Helper()
	for _, statement := range []string{"DROP TABLE rp_turn_styles", "DROP TABLE rp_style_bindings", "DROP TABLE rp_style_revisions"} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version=?`, RPStyleSchemaVersion); err != nil {
		t.Fatal(err)
	}
}

func TestRPStyleScopedSettingsPinnedRecoveryAndReadOnlyVariants(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "styles.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	session, read, initial := newRPWaitTestSession(t, ctx, s)
	set := func(scope, key string, patch core.RPStylePatch) RPStyleSetRequest {
		r := RPStyleSetRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Scope: scope, IdempotencyKey: key, Patch: patch}
		if scope == "world" {
			r.PrincipalID = "principal_creator"
		} else {
			r.SessionID = read.SessionID
		}
		if scope == "scene" {
			r.PlaceID = initial.PlaceID
		}
		return r
	}
	past, present, first, third, terse, detailed := "past", "present", "first_person", "third_person", "terse", "detailed"
	density, zero := 80, 0
	world, err := s.SetRPStyle(ctx, set("world", "world-style", core.RPStylePatch{POV: &third, Tense: &past, DescriptionDensity: &density}))
	if err != nil {
		t.Fatal(err)
	}
	worldResolved, err := s.ReadRPStyle(ctx, read)
	if err != nil || worldResolved.Profile.POV != third {
		t.Fatalf("world scope lost to implicit session default %+v %v", worldResolved, err)
	}
	sessionStyle := set("session", "session-style", core.RPStylePatch{POV: &first, Verbosity: &detailed})
	if _, err := s.SetRPStyle(ctx, sessionStyle); err != nil {
		t.Fatal(err)
	}
	sceneStyle := set("scene", "scene-style", core.RPStylePatch{DescriptionDensity: &zero})
	if _, err := s.SetRPStyle(ctx, sceneStyle); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.ReadRPStyle(ctx, read)
	if err != nil || resolved.Profile.POV != first || resolved.Profile.Tense != past || resolved.Profile.DescriptionDensity != 0 || len(resolved.Sources) != 3 || resolved.Sources[0] != world.RevisionID {
		t.Fatalf("wrong scope resolution %+v %v", resolved, err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, initial.ObservationCursor)
	s.afterRPTurnStage = func(stage string) error {
		if stage == "npc_effects_committed" {
			return errors.New("crash before narrative")
		}
		return nil
	}
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, Text: "我有一百万", IdempotencyKey: "styled-turn", NarrativeStyle: &core.RPStylePatch{POV: &third, Verbosity: &terse}}
	decisionOnly := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		raw, _ := json.Marshal(input)
		if strings.Contains(string(raw), "prose_instructions") || strings.Contains(string(raw), "narrative_pack_ref") || strings.Contains(string(raw), "dialogue_ratio") {
			t.Fatal("style entered decision context")
		}
		return core.DeterministicRPDecisionProvider{}.Propose(ctx, input)
	})
	if _, err := s.RunRPTurn(ctx, request, decisionOnly); err == nil {
		t.Fatal("crash not injected")
	}
	sessionStyle.IdempotencyKey = "changed-session"
	sessionStyle.ExpectedRevision = 1
	sessionStyle.Patch.POV = &first
	sessionStyle.Patch.Tense = &present
	if _, err := s.SetRPStyle(ctx, sessionStyle); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return core.DeterministicRPDecisionProvider{}.Propose(ctx, input)
	})
	result, err := s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey}, provider)
	if err != nil || calls != 0 || result.NarrativeStyle.POV != third || result.NarrativeStyle.Tense != past || result.NarrativeStyle.Verbosity != terse || !strings.Contains(result.NarrativeLines[0], "Lin说") {
		t.Fatalf("recovery changed pinned style/effects %+v calls%d %v", result, calls, err)
	}
	before := countsForStyleTest(t, ctx, s)
	one, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: result.TurnRunID})
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: result.TurnRunID, StyleOverride: &core.RPStylePatch{POV: &first, Verbosity: &detailed, DescriptionDensity: &density}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one.View.EventIDs, two.View.EventIDs) || reflect.DeepEqual(one.View.Lines, two.View.Lines) || before != countsForStyleTest(t, ctx, s) {
		t.Fatal("narrative variant changed facts or not presentation")
	}
	for _, v := range []RPNarrativeReadResult{one, two} {
		if !strings.Contains(v.View.Lines[0], "我有一百万") {
			t.Fatal("accepted quote rewritten")
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_turn_styles SET profile_json='{}' WHERE turn_run_id=?`, result.TurnRunID); err == nil {
		t.Fatal("pinned style mutated")
	}
}

func countsForStyleTest(t *testing.T, ctx context.Context, s *Store) string {
	t.Helper()
	var counts string
	if err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_utterances)||':'||(SELECT COUNT(*) FROM agent_knowledge)||':'||(SELECT SUM(balance_minor) FROM account_balances)`).Scan(&counts); err != nil {
		t.Fatal(err)
	}
	return counts
}

func TestRPStyleUpgrade025PinsLegacyAndRetainsWorld(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "style-upgrade.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	r := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "升级以前", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "legacy-style"}
	before, err := s.PlayRPTurn(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "升级时中断", ExpectedCursor: before.SettledSequence, IdempotencyKey: "legacy-interrupted"}
	s.afterRPTurnStage = func(stage string) error {
		if stage == "npc_effects_committed" {
			return errors.New("interrupted legacy turn")
		}
		return nil
	}
	if _, err := s.PlayRPTurn(ctx, interrupted); err == nil {
		t.Fatal("legacy interruption not injected")
	}
	counts := countsForStyleTest(t, ctx, s)
	removeRPStyleSchemaForUpgradeTest(t, ctx, s)
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after, err := s.PlayRPTurn(ctx, r)
	if err != nil || !after.Replayed || !reflect.DeepEqual(before.NarrativeLines, after.NarrativeLines) || counts != countsForStyleTest(t, ctx, s) {
		t.Fatalf("025 upgrade mutated legacy turn %+v %v", after, err)
	}
	if !reflect.DeepEqual(after.NarrativeStyle, core.DefaultRPStyle()) {
		t.Fatal("legacy style not preserved")
	}
	resumed, err := s.PlayRPTurn(ctx, interrupted)
	if err != nil || resumed.Status != "settled" || !reflect.DeepEqual(resumed.NarrativeStyle, core.DefaultRPStyle()) || counts != countsForStyleTest(t, ctx, s) {
		t.Fatalf("legacy pending recovery changed facts %+v %v", resumed, err)
	}
}

func TestRPStyleRevisionAuthorizationRollbackAndSceneIsolation(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "style-boundaries.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	first := "first_person"
	r := RPStyleSetRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Scope: "scene", SessionID: read.SessionID, PlaceID: initial.PlaceID, IdempotencyKey: "scene", Patch: core.RPStylePatch{POV: &first}}
	s.beforeCommit = func() error { return errors.New("style rollback") }
	if _, err := s.SetRPStyle(ctx, r); err == nil {
		t.Fatal("style rollback not injected")
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_style_revisions`, nil, 0)
	if _, err := s.SetRPStyle(ctx, r); err != nil {
		t.Fatal(err)
	}
	stale := r
	stale.IdempotencyKey = "stale"
	if _, err := s.SetRPStyle(ctx, stale); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale style %v", err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", IdempotencyKey: "move-away"}); err != nil {
		t.Fatal(err)
	}
	if retry, err := s.SetRPStyle(ctx, r); err != nil || !retry.Replayed {
		t.Fatalf("exact scene retry after move failed %+v %v", retry, err)
	}
	resolved, err := s.ReadRPStyle(ctx, read)
	if err != nil || resolved.Profile.POV == first || len(resolved.Sources) != 0 {
		t.Fatalf("scene style leaked to other place %+v %v", resolved, err)
	}
	stale.ExpectedRevision = 1
	if _, err := s.SetRPStyle(ctx, stale); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("offsite scene mutation %v", err)
	}
	world := RPStyleSetRequest{PrincipalID: read.PrincipalID, InstanceID: r.InstanceID, BranchID: r.BranchID, Scope: "world", IdempotencyKey: "forbidden-world", Patch: r.Patch}
	if _, err := s.SetRPStyle(ctx, world); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("player changed world style %v", err)
	}
	world.PrincipalID = "principal_creator"
	world.InstanceID = "unknown-world"
	if _, err := s.SetRPStyle(ctx, world); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("creator crossed world scope %v", err)
	}
}
