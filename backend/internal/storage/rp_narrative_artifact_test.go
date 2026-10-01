package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

type rpFullProseArtifactFixture struct{ narrativeProviderFixture }

func (rpFullProseArtifactFixture) NarrativeMode() string { return "full_prose" }

func TestRPNarrativeArtifactFullProseRefinesFrozenInputOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "refinement.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	full := true
	first, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "refine-first", Text: "只修饰原来这一回合。", NarrativeStyle: &core.RPStylePatch{FullProse: &full}}, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	// A later world head must never become this turn's optional narrator input.
	if _, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: first.SettledSequence, IdempotencyKey: "refine-later", Text: "这是后来发生的话。"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := rpFullProseArtifactFixture{narrativeProviderFixture{call: func(ctx context.Context, in core.RPNarrativeInput, emit func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
		calls++
		if in.SourceHead != first.SettledSequence {
			t.Fatal("refinement used current head", in.SourceHead)
		}
		for _, fact := range in.Facts {
			if strings.Contains(fact.Text, "后来发生") {
				t.Fatal("refinement included later fact", fact)
			}
		}
		probe, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		var head int64
		if err := s.db.QueryRowContext(probe, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal("provider retained transaction", err)
		}
		return (core.DeterministicRPNarrativeProvider{}).RenderStream(ctx, in, emit)
	}}}
	request := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: first.TurnRunID}
	refined, err := s.streamRPNarrativeWithProvider(ctx, request, nil, provider)
	if err != nil || calls != 1 {
		t.Fatal("opted-in refinement missing", calls, err)
	}
	var mode string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_presentation_mode FROM rp_turn_runs WHERE turn_run_id=?`, first.TurnRunID).Scan(&mode); err != nil || mode != "full_prose" {
		t.Fatal("refinement not persisted", mode, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := s.streamRPNarrativeWithProvider(ctx, request, nil, provider)
	if err != nil || calls != 1 || !reflect.DeepEqual(cached.View.Lines, refined.View.Lines) {
		t.Fatal("restart repeated refinement", calls, err)
	}
}

func TestRPNarrativeArtifactPrimaryCacheRestartAndCompanion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "artifact.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "artifact", Text: "你说的我听到了。"}
	decision := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "好，我再说明一次。", ExpressionCode: "nod", Private: &core.RPDecisionPrivate{Intent: "PRIVATE_ARTIFACT_SENTINEL"}}, nil
	})
	turn, err := s.RunRPTurn(ctx, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	if turn.CompositionVersion != core.RPFactCompositionVersionV2 {
		t.Fatal("primary is not v2", turn)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var artifact core.RPNarrativeArtifact
	if err := json.Unmarshal([]byte(raw), &artifact); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "PRIVATE_ARTIFACT_SENTINEL") || artifact.Input.SourceHead != turn.SettledSequence {
		t.Fatal("artifact leaked private data or lost head", raw)
	}
	facts := artifact.Input.Facts
	if len(facts) != 3 || facts[2].CompanionEventID != facts[1].EventID || !reflect.DeepEqual(turn.FactGroups[len(turn.FactGroups)-1], []string{facts[1].EventID, facts[2].EventID}) {
		t.Fatal("speech/expression lacks causal grouping", facts, turn.FactGroups)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := narrativeProviderFixture{call: func(context.Context, core.RPNarrativeInput, func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
		calls++
		t.Fatal("cached canonical called provider")
		return core.RPNarrativeView{}, nil
	}}
	var chunks []core.RPNarrativeChunk
	cached, err := s.streamRPNarrativeWithProvider(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}, func(chunk core.RPNarrativeChunk) error {
		// The read snapshot must be released before emitting.
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			return err
		}
		chunks = append(chunks, chunk)
		return nil
	}, provider)
	if err != nil || calls != 0 || !reflect.DeepEqual(cached.View.Lines, turn.NarrativeLines) || len(chunks) != len(turn.NarrativeLines) {
		t.Fatal("cached receipt changed", cached, err)
	}
	history, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	latest := history.RecentTurns[len(history.RecentTurns)-1]
	if !reflect.DeepEqual(latest.EventIDs, cached.View.EventIDs) || !reflect.DeepEqual(latest.FactGroups, turn.FactGroups) {
		t.Fatal("history lacks independent IDs", latest)
	}
	replay, err := s.RunRPTurn(ctx, request, decision)
	if err != nil || !replay.Replayed || !reflect.DeepEqual(replay.NarrativeLines, turn.NarrativeLines) {
		t.Fatal("replay changed artifact", replay, err)
	}
}

func TestRPNarrativeArtifactRecomputedForgeryRejected(t *testing.T) {
	mutations := map[string]func(*core.RPNarrativeInput){
		"expression_code": func(in *core.RPNarrativeInput) { in.Facts[len(in.Facts)-1].ExpressionCode = "smile" },
		"expression_target": func(in *core.RPNarrativeInput) {
			in.Facts[len(in.Facts)-1].TargetActorID = M2RPNPCID
			in.Facts[len(in.Facts)-1].TargetActorName = "Cai"
		},
		"companion_removed":  func(in *core.RPNarrativeInput) { in.Facts[len(in.Facts)-1].CompanionEventID = "" },
		"movement_direction": func(in *core.RPNarrativeInput) { in.Facts[len(in.Facts)-1].Action = "arrive" },

		"place": func(in *core.RPNarrativeInput) { in.Facts[0].PlaceName = "伪造地点" },
		"extra_activity": func(in *core.RPNarrativeInput) {
			in.Facts[0].ActivityCode = "forged"
			in.Facts[0].ActivityLabel = "伪造动作"
		},
		"labels": func(in *core.RPNarrativeInput) { in.ActivityLabels = map[string]string{"forged": "私密计划"} },
		"extra_object": func(in *core.RPNarrativeInput) {
			in.Facts[0].ObjectName = "伪造物品"
			in.Facts[0].ObjectState = "destroyed"
		},
		"extra_target": func(in *core.RPNarrativeInput) {
			in.Facts[0].TargetActorID = M2RPNPCID
			in.Facts[0].TargetActorName = "伪造目标"
		},
		"cue": func(in *core.RPNarrativeInput) {
			in.PublicPresentations = []core.RPPublicPresentation{{ActorID: M2RPNPCID, ActorName: "Cai", Text: "从容地隐藏私密计划", SourceEventID: in.Facts[0].EventID}}
		},
		"speech":        func(in *core.RPNarrativeInput) { in.Facts[0].Text = "未发生的话" },
		"omit_required": func(in *core.RPNarrativeInput) { in.Facts = in.Facts[:1] },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "forgery.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			_, read, initial := newRPWaitTestSession(t, ctx, s)
			turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "forgery", Text: "原话。"}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
				if name == "movement_direction" {
					return core.RPDecisionProposal{Action: "leave", DestinationPlaceID: "place_m2_home_ada"}, nil
				}
				return core.RPDecisionProposal{Action: "respond", Text: "原来的答复。", ExpressionCode: "nod"}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			var raw string
			if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var artifact core.RPNarrativeArtifact
			if err := json.Unmarshal([]byte(raw), &artifact); err != nil {
				t.Fatal(err)
			}
			mutate(&artifact.Input)
			plan, err := core.BuildDefaultRPComposition(artifact.Input)
			if err != nil {
				t.Fatal(err)
			}
			forged, err := core.RenderRPComposition(ctx, artifact.Input, plan, nil)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := encodeRPNarrativeArtifact(forged)
			if err != nil {
				t.Fatal(err)
			}
			lines, _ := json.Marshal(forged.Lines)
			groups, _ := json.Marshal(forged.FactGroups)
			ids, _ := json.Marshal(forged.EventIDs)
			// Forge every presentation field consistently; independent world sources stay intact.
			if _, err := s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET narrative_artifact_json=?,narrative_json=?,narrative_fact_groups_json=?,narrative_fact_event_ids_json=? WHERE turn_run_id=?`, encoded, string(lines), string(groups), string(ids), turn.TurnRunID); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatal("read accepted recomputed forgery", err)
			}
			if _, err := s.ObserveRPSession(ctx, read); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatal("history accepted recomputed forgery", err)
			}
			if _, err := s.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatal("selection accepted recomputed forgery", err)
			}
			run, err := s.loadRPTurnRun(ctx, turn.TurnRunID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.loadRPTurnResult(ctx, run, true); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatal("replay accepted recomputed forgery", err)
			}
		})
	}
}

func TestRPNarrativeArtifactPrimaryRollbackIsAtomic(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "rollback", Text: "回执必须完整。"}
	s.afterRPTurnStage = func(stage string) error {
		if stage == "npc_effects_committed" {
			return core.NewError(core.CodeInjectedFailure, "stop before receipt")
		}
		return nil
	}
	if _, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{}); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal(err)
	}
	s.afterRPTurnStage = nil
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT turn_run_id FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, read.SessionID, request.IdempotencyKey).Scan(&id); err != nil {
		t.Fatal(err)
	}
	run, err := s.loadRPTurnRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	style, err := s.loadRPTurnStyle(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.renderRPTurnStyled(ctx, run.SessionID, run.PlayerTurnID, run.PlayerEventID, style.Profile)
	if err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "receipt precommit") }
	if err := s.markRPTurnNarrativeReady(ctx, id, view); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("did not roll back receipt", err)
	}
	s.beforeCommit = nil
	run, err = s.loadRPTurnRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "npc_effects_committed" || run.CompositionVersion != "" || run.ArtifactJSON != "{}" || run.FactGroupsJSON != "[]" || run.FactEventIDsJSON != "[]" || run.SettledSequence.Int64 != 0 {
		t.Fatal("partial receipt survived rollback", run)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, view.Artifact.Input.SourceHead)
	turn, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || turn.CompositionVersion != core.RPFactCompositionVersionV2 {
		t.Fatal("receipt did not recover", turn, err)
	}
}

func TestRPNarrativeArtifactMigrationPreservesV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "migration078.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "migration078", Text: "历史回执。"}, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.readRPNarrativeInput(ctx, read.SessionID, turn.PlayerTurnID, turn.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	input.Style = core.DefaultRPStyle()
	view, err := (core.LiteralRPNarrativeProvider{}).Render(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	view.CompositionVersion = core.RPFactCompositionVersionV1
	for _, id := range view.EventIDs {
		view.FactGroups = append(view.FactGroups, []string{id})
	}
	clearRPV2CanonicalFixture(t, ctx, s, turn.TurnRunID)
	if err := s.saveRPOfficialNarrative(ctx, turn.TurnRunID, "full_prose", view); err != nil {
		t.Fatal(err)
	}
	request := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}
	if err := s.saveSelectedRPNarrative(ctx, request, input, &view, core.RPProviderMetadata{Kind: "fixture"}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`ALTER TABLE rp_turn_runs DROP COLUMN narrative_artifact_json`, `ALTER TABLE rp_narrative_renders DROP COLUMN artifact_json`, `DELETE FROM schema_meta WHERE schema_version='corerp-rp-narrative-artifacts-078-2026-10-01'`} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := s.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, RenderID: view.RenderID})
	if err != nil || selected.CompositionVersion != core.RPFactCompositionVersionV1 || !reflect.DeepEqual(selected.Lines, view.Lines) || !reflect.DeepEqual(selected.FactGroups, view.FactGroups) {
		t.Fatal("078 changed v1 receipt", selected, err)
	}
	var artifact string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&artifact); err != nil || artifact != "{}" {
		t.Fatal("078 fabricated v2 artifact", artifact, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPNarrativeArtifactsSchemaVersion}, 1)
}

func TestRPNarrativeArtifactHistoricalAliasSurvivesIntroduction(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "historical.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", IdempotencyKey: "historical-meet"}); err != nil {
		t.Fatal(err)
	}
	observed, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: observed.ObservationCursor, IdempotencyKey: "historical-turn", Text: "你好。"}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "你好，我听见了。"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range before.View.Artifact.Input.Facts {
		found = found || fact.ActorID == alias && fact.ActorName == "陌生人"
	}
	if !found {
		t.Fatal("fixture lacks frozen anonymous fact", before.View.Artifact.Input.Facts)
	}
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	ada, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	adaRead := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID}
	adaView, err := s.ObserveRPSession(ctx, adaRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "historical-introduction", Text: "我叫 Ada。", IntroduceSelf: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil || !reflect.DeepEqual(before.View.Lines, after.View.Lines) || !reflect.DeepEqual(before.View.Artifact.Input, after.View.Artifact.Input) {
		t.Fatal("introduction reinterpreted frozen output", after, err)
	}
}

func TestRPNarrativeArtifactVariantForgeryRejected(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "variant.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "variant-forgery", Text: "原来这句话。"})
	if err != nil {
		t.Fatal(err)
	}
	pov := "first_person"
	variant, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, StyleOverride: &core.RPStylePatch{POV: &pov}})
	if err != nil {
		t.Fatal(err)
	}
	input := variant.View.Artifact.Input
	input.Facts[0].PlaceName = "伪造的另一个地方"
	plan, err := core.BuildDefaultRPComposition(input)
	if err != nil {
		t.Fatal(err)
	}
	forged, err := core.RenderRPComposition(ctx, input, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := encodeRPNarrativeArtifact(forged)
	if err != nil {
		t.Fatal(err)
	}
	lines, _ := json.Marshal(forged.Lines)
	groups, _ := json.Marshal(forged.FactGroups)
	// Simulate out-of-band damage to an otherwise immutable presentation row.
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER rp_narrative_renders_no_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_narrative_renders SET artifact_json=?,lines_json=?,fact_groups_json=? WHERE render_id=?`, artifact, string(lines), string(groups), variant.View.RenderID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, RenderID: variant.View.RenderID}); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("selection accepted variant forgery", err)
	}
	if _, err := s.ObserveRPSession(ctx, read); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("history accepted variant forgery", err)
	}
	canonical, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil || !reflect.DeepEqual(canonical.View.Lines, turn.NarrativeLines) {
		t.Fatal("damaged variant changed canonical read", err)
	}
}

func TestRPNarrativeArtifactConcurrentRefinementReturnsWinner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "concurrent-refinement.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	full := true
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "concurrent-refinement", Text: "同一句原话。", NarrativeStyle: &core.RPStylePatch{FullProse: &full}})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	type outcome struct {
		view   core.RPNarrativeView
		chunks []string
		err    error
	}
	results := make(chan outcome, 2)
	request := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}
	for _, form := range []string{"subject_first", "quote_first"} {
		form := form
		go func() {
			provider := rpFullProseArtifactFixture{narrativeProviderFixture{call: func(ctx context.Context, in core.RPNarrativeInput, emit func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
				entered <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return core.RPNarrativeView{}, ctx.Err()
				}
				plan, err := core.BuildDefaultRPComposition(in)
				if err != nil {
					return core.RPNarrativeView{}, err
				}
				plan.Paragraphs[0].Beats[0].Form = form
				return core.RenderRPComposition(ctx, in, plan, emit)
			}}}
			var chunks []string
			value, err := s.streamRPNarrativeWithProvider(ctx, request, func(chunk core.RPNarrativeChunk) error { chunks = append(chunks, chunk.Line); return nil }, provider)
			results <- outcome{value.View, chunks, err}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			close(release)
			t.Fatal("concurrent provider retained DB or did not start", ctx.Err())
		}
	}
	close(release)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || !reflect.DeepEqual(first.view.Lines, second.view.Lines) || !reflect.DeepEqual(first.chunks, first.view.Lines) || !reflect.DeepEqual(second.chunks, first.view.Lines) {
		t.Fatal("losing plan escaped committed winner", first, second)
	}
	cached, err := s.ReadRPNarrative(ctx, request)
	if err != nil || !reflect.DeepEqual(cached.View.Lines, first.view.Lines) {
		t.Fatal("concurrent response differs from saved winner", cached, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_narrative_renders WHERE turn_run_id=?`, []any{turn.TurnRunID}, 0)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, turn.SettledSequence)
}

func TestRPNarrativeArtifactHistoricalPlaceSurvivesRename(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "place-history.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { _ = s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "artifact-place-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	const world = "artifact-place-world"
	if _, err := s.SaveStudioWorld(ctx, prepareNarrativeWindowWorld(t, ctx, s, world)); err != nil {
		t.Fatal(err)
	}
	player, _ := core.StudioWorldObjectID(world, "entity", "lin")
	place, _ := core.StudioWorldObjectID(world, "place", "home")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: "artifact-place-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	initial, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "artifact-place-turn", Text: "这是旧地点的对白。"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameRPLocation(ctx, RPLocationRenameRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: world, BranchID: "br_main", ExpectedHead: turn.SettledSequence, IdempotencyKey: "artifact-place-rename"}, LocationID: place, NewName: "新名字"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil || !reflect.DeepEqual(before.View.Lines, after.View.Lines) || !reflect.DeepEqual(before.View.Artifact.Input, after.View.Artifact.Input) {
		t.Fatal("rename reinterpreted frozen output", after, err)
	}
}
