package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/narrative"
)

func TestRPNarrativeCompositionReceiptSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "composition.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	instructions := "只调整已确认事实的排版。"
	if _, err := s.SetRPStyle(ctx, RPStyleSetRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Scope: "session", SessionID: read.SessionID, IdempotencyKey: "composition-warning-style", Patch: core.RPStylePatch{ProseInstructions: &instructions}}); err != nil {
		t.Fatal(err)
	}
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "composition", Text: "你好。"}, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.readRPNarrativeInput(ctx, read.SessionID, turn.PlayerTurnID, turn.PlayerEventID)
	if err != nil || len(input.Facts) < 2 {
		t.Fatal("need multiple committed facts", input, err)
	}
	style, err := s.loadRPTurnStyle(ctx, turn.TurnRunID)
	if err != nil {
		t.Fatal(err)
	}
	input.Style = style.Profile
	base, err := (core.DeterministicRPNarrativeProvider{}).Render(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	canonicalView := base
	canonicalView.CompositionVersion = rpNarrativeCompositionVersion
	for _, id := range base.EventIDs {
		canonicalView.FactGroups = append(canonicalView.FactGroups, []string{id})
	}
	if err := s.saveRPOfficialNarrative(ctx, turn.TurnRunID, "full_prose", canonicalView); err != nil {
		t.Fatal(err)
	}
	// One paragraph owns multiple atoms; group attribution is not line-index
	// attribution, and authored cue provenance is not another fact atom.
	view := core.RPNarrativeView{Lines: []string{strings.Join(base.Lines, "")}, EventIDs: base.EventIDs, CompositionVersion: rpNarrativeCompositionVersion, FactGroups: [][]string{append([]string(nil), base.EventIDs...)}}
	input.PublicPresentations = []core.RPPublicPresentation{{SourceEventID: "authored-presentation-only"}}
	pov := "second_person"
	request := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, StyleOverride: &core.RPStylePatch{POV: &pov}}
	var canonical string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_json FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSelectedRPNarrative(ctx, request, input, &view, core.RPProviderMetadata{Kind: "full_prose", Model: "fixture"}); err != nil {
		t.Fatal(err)
	}
	var version, groupsJSON, sourcesJSON, factIDsJSON, kind string
	if err := s.db.QueryRowContext(ctx, `SELECT composition_version,fact_groups_json,source_event_ids_json,fact_event_ids_json,provider_kind FROM rp_narrative_renders WHERE render_id=?`, view.RenderID).Scan(&version, &groupsJSON, &sourcesJSON, &factIDsJSON, &kind); err != nil {
		t.Fatal(err)
	}
	var groups [][]string
	if err := json.Unmarshal([]byte(groupsJSON), &groups); err != nil || version != view.CompositionVersion || kind != "full_prose" || !reflect.DeepEqual(groups, view.FactGroups) || strings.Contains(groupsJSON, "authored-presentation-only") || !strings.Contains(sourcesJSON, "authored-presentation-only") {
		t.Fatal("receipt lost composition or mixed provenance", version, groupsJSON, sourcesJSON, kind, err)
	}
	var factIDs []string
	if err := json.Unmarshal([]byte(factIDsJSON), &factIDs); err != nil || !reflect.DeepEqual(factIDs, view.EventIDs) || strings.Contains(factIDsJSON, "authored-presentation-only") {
		t.Fatal("independent source sequence mixed facts and provenance", factIDsJSON, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := s.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, RenderID: view.RenderID})
	if err != nil || selected.CompositionVersion != view.CompositionVersion || !reflect.DeepEqual(selected.FactGroups, view.FactGroups) || !reflect.DeepEqual(selected.Lines, view.Lines) {
		t.Fatal("selected receipt did not reload", selected, err)
	}
	observed, err := s.ObserveRPSession(ctx, read)
	if err != nil || len(observed.RecentTurns) == 0 {
		t.Fatal(observed, err)
	}
	recent := observed.RecentTurns[len(observed.RecentTurns)-1]
	if recent.RenderID != view.RenderID || recent.CompositionVersion != view.CompositionVersion || !reflect.DeepEqual(recent.FactGroups, view.FactGroups) || !reflect.DeepEqual(recent.NarrativeLines, view.Lines) {
		t.Fatal("selected history lost composition", recent)
	}
	var canonicalAfter string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_json FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&canonicalAfter); err != nil || canonicalAfter != canonical {
		t.Fatal("canonical narrative changed", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, turn.SettledSequence)
	var chunks []core.RPNarrativeChunk
	provider := narrativeProviderFixture{call: func(context.Context, core.RPNarrativeInput, func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
		t.Fatal("saved canonical narrative invoked a provider")
		return core.RPNarrativeView{}, nil
	}}
	service, err := NewRPServiceWithNarrative(s, core.DeterministicRPDecisionProvider{}, "deterministic", provider)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := service.StreamRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}, func(chunk core.RPNarrativeChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil || reloaded.View.CompositionVersion != canonicalView.CompositionVersion || !reflect.DeepEqual(reloaded.View.FactGroups, canonicalView.FactGroups) || !reflect.DeepEqual(reloaded.View.Lines, canonicalView.Lines) || len(chunks) != len(canonicalView.Lines) {
		t.Fatal("canonical composition did not reload", reloaded, chunks, err)
	}
	if !reflect.DeepEqual(reloaded.View.Warnings, []string{narrative.CompositionCapabilityWarning}) {
		t.Fatal("saved composition lost supported/unsupported capability disclosure", reloaded.View.Warnings)
	}
	for i, chunk := range chunks {
		if !reflect.DeepEqual(chunk.EventIDs, canonicalView.FactGroups[i]) || chunk.EventID != "" {
			t.Fatal("saved stream lost actual group attribution", chunk)
		}
	}

	// Invalid receipts must leave both immutable render and selection intact.
	cases := map[string]core.RPNarrativeView{}
	copyView := func() core.RPNarrativeView {
		v := view
		v.FactGroups = [][]string{append([]string(nil), view.FactGroups[0]...)}
		return v
	}
	v := copyView()
	v.FactGroups[0] = v.FactGroups[0][:len(v.FactGroups[0])-1]
	cases["missing"] = v
	v = copyView()
	v.FactGroups[0][0], v.FactGroups[0][1] = v.FactGroups[0][1], v.FactGroups[0][0]
	cases["reordered"] = v
	v = copyView()
	v.FactGroups[0] = append(v.FactGroups[0], v.FactGroups[0][0])
	cases["duplicated"] = v
	v = copyView()
	v.FactGroups[0][0] = "authored-presentation-only"
	cases["provenance as fact"] = v
	v = copyView()
	v.FactGroups = [][]string{{}}
	cases["empty group"] = v
	v = copyView()
	v.CompositionVersion = ""
	cases["unversioned groups"] = v
	v = copyView()
	v.CompositionVersion = "unknown-version"
	cases["unknown version"] = v
	v = copyView()
	v.Lines = append(v.Lines, "unattributed line")
	cases["unattributed line"] = v
	for name, invalid := range cases {
		t.Run(name, func(t *testing.T) {
			if err := s.saveSelectedRPNarrative(ctx, request, input, &invalid, core.RPProviderMetadata{Kind: "full_prose"}); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatal("invalid receipt accepted", err)
			}
		})
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_narrative_renders WHERE turn_run_id=?`, []any{turn.TurnRunID}, 1)
	var selectedID string
	if err := s.db.QueryRowContext(ctx, `SELECT render_id FROM rp_narrative_selections WHERE turn_run_id=?`, turn.TurnRunID).Scan(&selectedID); err != nil || selectedID != view.RenderID {
		t.Fatal("invalid save changed selection", selectedID, err)
	}
	canonicalResult, err := s.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil || canonicalResult.CompositionVersion != canonicalView.CompositionVersion || !reflect.DeepEqual(canonicalResult.FactGroups, canonicalView.FactGroups) || !reflect.DeepEqual(canonicalResult.Lines, canonicalView.Lines) {
		t.Fatal("canonical restore lost metadata", canonicalResult, err)
	}
	observed, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	recent = observed.RecentTurns[len(observed.RecentTurns)-1]
	if recent.RenderID != "" || recent.CompositionVersion != canonicalView.CompositionVersion || !reflect.DeepEqual(recent.FactGroups, canonicalView.FactGroups) {
		t.Fatal("canonical history lost metadata", recent)
	}
	run, err := s.loadRPTurnRun(ctx, turn.TurnRunID)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.loadRPTurnResult(ctx, run, true)
	if err != nil || !replayed.Replayed || replayed.CompositionVersion != canonicalView.CompositionVersion || !reflect.DeepEqual(replayed.FactGroups, canonicalView.FactGroups) {
		t.Fatal("turn replay lost composition", replayed, err)
	}
	if !reflect.DeepEqual(replayed.NarrativeWarnings, []string{"deterministic renderer does not interpret free-form prose instructions", narrative.CompositionCapabilityWarning}) {
		t.Fatal("turn replay lost composition capability disclosure", replayed.NarrativeWarnings)
	}
	loaded, err := s.loadRPTurnResult(ctx, run, false)
	if err != nil || !reflect.DeepEqual(loaded.NarrativeWarnings, replayed.NarrativeWarnings) {
		t.Fatal("settled turn read lost composition capability disclosure", loaded, err)
	}
}

func TestRPNarrativeCompositionMigrationPreservesLegacySelectedProse(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-composition.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "legacy-composition", Text: "你好。"}, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	pov := "first_person"
	variant, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, StyleOverride: &core.RPStylePatch{POV: &pov}})
	if err != nil {
		t.Fatal(err)
	}
	// Return a populated fixture to the pre-077 table shape; Open must apply
	// the additive migration without rewriting immutable legacy render rows.
	for _, statement := range []string{
		`ALTER TABLE rp_narrative_renders DROP COLUMN fact_event_ids_json`,
		`ALTER TABLE rp_narrative_renders DROP COLUMN fact_groups_json`,
		`ALTER TABLE rp_narrative_renders DROP COLUMN composition_version`,
		`ALTER TABLE rp_turn_runs DROP COLUMN narrative_fact_event_ids_json`,
		`ALTER TABLE rp_turn_runs DROP COLUMN narrative_fact_groups_json`,
		`ALTER TABLE rp_turn_runs DROP COLUMN narrative_composition_version`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-narrative-composition-077-2026-10-01'`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal("077 upgrade", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPNarrativeCompositionSchemaVersion}, 1)
	selected, err := s.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, RenderID: variant.View.RenderID})
	if err != nil || selected.CompositionVersion != "" || len(selected.FactGroups) != 0 || !reflect.DeepEqual(selected.Lines, variant.View.Lines) {
		t.Fatal("legacy prose was reclassified or lost", selected, err)
	}
	observed, err := s.ObserveRPSession(ctx, read)
	if err != nil || len(observed.RecentTurns) == 0 {
		t.Fatal(observed, err)
	}
	recent := observed.RecentTurns[len(observed.RecentTurns)-1]
	if recent.RenderID != variant.View.RenderID || recent.CompositionVersion != "" || len(recent.FactGroups) != 0 || !reflect.DeepEqual(recent.NarrativeLines, variant.View.Lines) {
		t.Fatal("legacy selected history changed", recent)
	}
	canonical, err := s.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil || canonical.CompositionVersion != "" || len(canonical.FactGroups) != 0 || !reflect.DeepEqual(canonical.Lines, turn.NarrativeLines) {
		t.Fatal("canonical restore acquired composition metadata", canonical, err)
	}
	input, err := s.readRPNarrativeInput(ctx, read.SessionID, turn.PlayerTurnID, turn.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(canonical.Lines)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := savedRPOfficialNarrative(ctx, string(encoded), "", "", "[]", "[]", input, nil)
	if err != nil || legacy.CompositionVersion != "" || len(legacy.Warnings) != 0 {
		t.Fatal("legacy prose acquired composition capability guarantee", legacy, err)
	}
	run, err := s.loadRPTurnRun(ctx, turn.TurnRunID)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.loadRPTurnResult(ctx, run, true)
	if err != nil || replayed.CompositionVersion != "" {
		t.Fatal("legacy turn replay changed", replayed, err)
	}
	for _, warning := range replayed.NarrativeWarnings {
		if warning == narrative.CompositionCapabilityWarning {
			t.Fatal("legacy turn replay acquired a composition guarantee")
		}
	}
}

func TestRPNarrativeCompositionCorruptionRejectedAfterRestart(t *testing.T) {
	for _, receipt := range []string{"variant", "canonical"} {
		for _, corruption := range []string{"unknown", "duplicate", "reordered"} {
			t.Run(receipt+"/"+corruption, func(t *testing.T) {
				ctx := context.Background()
				path := filepath.Join(t.TempDir(), "corrupt-composition.db")
				s, err := Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = s.Close() }()
				_, read, initial := newRPWaitTestSession(t, ctx, s)
				turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "corrupt-composition", Text: "你好。"}, core.DeterministicRPDecisionProvider{})
				if err != nil {
					t.Fatal(err)
				}
				input, err := s.readRPNarrativeInput(ctx, read.SessionID, turn.PlayerTurnID, turn.PlayerEventID)
				if err != nil || len(input.Facts) < 2 {
					t.Fatal("need multiple fact sources", err)
				}
				style, err := s.loadRPTurnStyle(ctx, turn.TurnRunID)
				if err != nil {
					t.Fatal(err)
				}
				input.Style = style.Profile
				view, err := (core.DeterministicRPNarrativeProvider{}).Render(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				view.CompositionVersion = rpNarrativeCompositionVersion
				for _, id := range view.EventIDs {
					view.FactGroups = append(view.FactGroups, []string{id})
				}
				if err := s.saveRPOfficialNarrative(ctx, turn.TurnRunID, "full_prose", view); err != nil {
					t.Fatal(err)
				}
				pov := "second_person"
				request := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, StyleOverride: &core.RPStylePatch{POV: &pov}}
				if err := s.saveSelectedRPNarrative(ctx, request, input, &view, core.RPProviderMetadata{Kind: "full_prose"}); err != nil {
					t.Fatal(err)
				}
				corrupt := make([][]string, len(view.FactGroups))
				for i, group := range view.FactGroups {
					corrupt[i] = append([]string(nil), group...)
				}
				switch corruption {
				case "unknown":
					corrupt[0][0] = "authored-style-source-is-not-a-fact"
				case "duplicate":
					corrupt[1][0] = corrupt[0][0]
				case "reordered":
					corrupt[0][0], corrupt[1][0] = corrupt[1][0], corrupt[0][0]
				}
				encoded, err := json.Marshal(corrupt)
				if err != nil {
					t.Fatal(err)
				}
				var independentBefore string
				readSources := `SELECT narrative_fact_event_ids_json FROM rp_turn_runs WHERE turn_run_id=?`
				updateGroups := `UPDATE rp_turn_runs SET narrative_fact_groups_json=? WHERE turn_run_id=?`
				identity := turn.TurnRunID
				if receipt == "variant" {
					// Simulate out-of-band restore/corruption, not a supported write.
					if _, err := s.db.ExecContext(ctx, `DROP TRIGGER rp_narrative_renders_no_update`); err != nil {
						t.Fatal(err)
					}
					readSources = `SELECT fact_event_ids_json FROM rp_narrative_renders WHERE render_id=?`
					updateGroups = `UPDATE rp_narrative_renders SET fact_groups_json=? WHERE render_id=?`
					identity = view.RenderID
				}
				if err := s.db.QueryRowContext(ctx, readSources, identity).Scan(&independentBefore); err != nil {
					t.Fatal(err)
				}
				if _, err := s.db.ExecContext(ctx, updateGroups, string(encoded), identity); err != nil {
					t.Fatal(err)
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				s, err = Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				var independentAfter string
				if err := s.db.QueryRowContext(ctx, readSources, identity).Scan(&independentAfter); err != nil || independentAfter != independentBefore {
					t.Fatal("corruption altered independent source sequence", err)
				}
				if _, err := s.ObserveRPSession(ctx, read); !core.HasCode(err, core.CodeProjectionDiverged) {
					t.Fatal("Observe accepted corrupted groups", err)
				}
				selectRequest := RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}
				if receipt == "variant" {
					selectRequest.RenderID = view.RenderID
				}
				if _, err := s.SelectRPNarrative(ctx, selectRequest); !core.HasCode(err, core.CodeProjectionDiverged) {
					t.Fatal("Select accepted corrupted groups", err)
				}
				if receipt == "canonical" {
					run, err := s.loadRPTurnRun(ctx, turn.TurnRunID)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := s.loadRPTurnResult(ctx, run, true); !core.HasCode(err, core.CodeProjectionDiverged) {
						t.Fatal("turn replay accepted corrupted groups", err)
					}
					if _, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}); !core.HasCode(err, core.CodeProjectionDiverged) {
						t.Fatal("canonical read accepted corrupted groups", err)
					}
				}
			})
		}
	}
}
