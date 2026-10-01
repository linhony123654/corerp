package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPSpeechToneOwnerAtomicHearingContextAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "delivery.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "delivery-player", Text: "我来了。"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	decision, err := s.DecideRP(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我听着。", SpeechTone: "gentle", Private: &core.RPDecisionPrivate{Intent: "DELIVERY_PRIVATE_SECRET", Emotion: "担忧", BasisEventIDs: []string{in.SpeechEventID}}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "delivery atomicity") }
	if _, err := s.CommitRPDecision(ctx, request, decision); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("rollback missing", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE json_extract(payload,'$.speech_tone') IS NOT NULL`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE json_extract(claim_payload,'$.speech_tone') IS NOT NULL`, nil, 0)
	committed, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	var payload, hearing string
	if err := s.db.QueryRowContext(ctx, `SELECT e.payload,o.claim_payload FROM events e JOIN observation_records o ON o.source_event_id=e.event_id AND o.observer_agent_id=? AND o.claim_key='speech:'||e.event_id WHERE e.event_id=?`, M2RPPlayerID, committed.EventID).Scan(&payload, &hearing); err != nil {
		t.Fatal(err)
	}
	tone, err := recordedRPSpeechTone(payload, hearing, M2RPNPCID, M2RPPlayerID)
	if err != nil || tone != "gentle" || strings.Contains(payload+hearing, "DELIVERY_PRIVATE_SECRET") || strings.Contains(payload+hearing, "担忧") {
		t.Fatal("delivery/private boundary", tone, err)
	}
	input, err := s.readRPNarrativeInput(ctx, read.SessionID, speech.TurnID, speech.EventID)
	if err != nil {
		t.Fatal(err)
	}
	input.Style = core.DefaultRPStyle()
	view, err := (core.DeterministicRPNarrativeProvider{}).Render(ctx, input)
	if err != nil || view.CompositionVersion != core.RPFactCompositionVersionV3 || !strings.Contains(strings.Join(view.Lines, ""), "温和地") {
		t.Fatal("delivery not rendered", view, err)
	}
	public, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, fact := range public.Facts {
		if fact.SourceEventID == committed.EventID {
			seen = fact.SpeechTone == "gentle"
		}
	}
	if !seen {
		t.Fatal("public hearing lost delivery", public.Facts)
	}
	publicJSON, _ := json.Marshal(public)
	if strings.Contains(string(publicJSON), "DELIVERY_PRIVATE_SECRET") {
		t.Fatal("private intent leaked")
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatal("projection differs", differences, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil || !replay.Replayed || replay.EventID != committed.EventID {
		t.Fatal("restart duplicated delivery", replay, err)
	}
	inputAfter, err := s.readRPNarrativeInput(ctx, read.SessionID, speech.TurnID, speech.EventID)
	inputAfter.Style = core.DefaultRPStyle()
	if err != nil || !reflect.DeepEqual(input, inputAfter) {
		t.Fatal("restart changed public delivery", err)
	}
	// The speaker's own recorded delivery is also retained by the one builder.
	observed, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: observed.ObservationCursor, IdempotencyKey: "delivery-followup", Text: "刚才的话听到了。"})
	if err != nil {
		t.Fatal(err)
	}
	npcInput, err := s.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: next.TurnID, NPCEntityID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	seen = false
	for _, dialogue := range npcInput.RecentDialogue {
		if dialogue.EventID == committed.EventID {
			seen = dialogue.SpeechTone == "gentle"
		}
	}
	for _, exchange := range npcInput.RelevantDialogue {
		for _, dialogue := range exchange.Dialogue {
			if dialogue.EventID == committed.EventID {
				seen = seen || dialogue.SpeechTone == "gentle"
			}
		}
	}
	for _, action := range npcInput.OwnActions {
		if action.EventID == committed.EventID {
			seen = seen || action.SpeechTone == "gentle"
		}
	}
	if !seen {
		t.Fatal("own accepted delivery lost from context")
	}
}

func TestRPSpeechToneRequiresFrozenHearingWithoutRetrofilling(t *testing.T) {
	speech := rpSpeechEvent{SpeakerEntityID: "npc", UtteranceID: "utterance", Text: "我听着。", SpeechAct: "statement", SpeechTone: "gentle", ListenerIDs: []string{"player"}}
	hearing := rpSpeechClaim{ClaimType: "speaker_said", SpeakerEntityID: "npc", UtteranceID: "utterance", Text: speech.Text, SpeechAct: speech.SpeechAct, SpeechTone: speech.SpeechTone}
	source, _ := json.Marshal(speech)
	for _, tc := range []struct {
		name, observer string
		mutate         func(*rpSpeechClaim)
	}{
		{"missing_delivery", "player", func(c *rpSpeechClaim) { c.SpeechTone = "" }},
		{"changed_delivery", "player", func(c *rpSpeechClaim) { c.SpeechTone = "firm" }},
		{"wrong_words", "player", func(c *rpSpeechClaim) { c.Text = "没说过" }},
		{"unheard", "absent", func(*rpSpeechClaim) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claim := hearing
			tc.mutate(&claim)
			raw, _ := json.Marshal(claim)
			if _, err := recordedRPSpeechTone(string(source), string(raw), "npc", tc.observer); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatal("accepted unsupported delivery", err)
			}
		})
	}
}

func TestRPSpeechToneNonSpeechOwnersKeepAttributedKnowledge(t *testing.T) {
	for _, owner := range []string{"RPInstitutionFactRecorded", "RPCareerFactRecorded", "RPCultureFactRecorded"} {
		// Such owners retain their own payload contracts; empty delivery is
		// not a license to reconstruct an RP speech or a voice from them.
		if tone, err := rpKnowledgeSpeechTone(owner, `{"owner_data":"unchanged"}`, `{"claim_type":"speaker_said","text":"原有公告。"}`, "speaker", "listener"); err != nil || tone != "" {
			t.Fatal("legacy attributed knowledge was reinterpreted", owner, tone, err)
		}
		if _, err := rpKnowledgeSpeechTone(owner, `{"speech_tone":"gentle"}`, `{"claim_type":"speaker_said","speech_tone":"gentle"}`, "speaker", "listener"); !core.HasCode(err, core.CodeProjectionDiverged) {
			t.Fatal("owner without delivery authority acquired a tone", owner, err)
		}
	}
}

func TestRPSpeechToneSavedV3RestartAndRecomputedForgery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "delivery-artifact.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	calls := 0
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return core.RPDecisionProposal{Action: "respond", Text: "我听着。", SpeechTone: "gentle"}, nil
	})
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "delivery-saved", Text: "你好。"}
	turn, err := s.RunRPTurn(ctx, request, provider)
	if err != nil || calls != 1 || turn.CompositionVersion != core.RPFactCompositionVersionV3 {
		t.Fatal("v3 primary missing", turn, calls, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.RunRPTurn(ctx, request, provider)
	if err != nil || calls != 1 || !replay.Replayed || !reflect.DeepEqual(replay.NarrativeLines, turn.NarrativeLines) {
		t.Fatal("v3 cache did not survive", calls, err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var artifact core.RPNarrativeArtifact
	if err := json.Unmarshal([]byte(raw), &artifact); err != nil {
		t.Fatal(err)
	}
	for i := range artifact.Input.Facts {
		if artifact.Input.Facts[i].SpeechTone != "" {
			artifact.Input.Facts[i].SpeechTone = "firm"
		}
	}
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
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET narrative_artifact_json=?,narrative_json=?,narrative_fact_groups_json=?,narrative_fact_event_ids_json=? WHERE turn_run_id=?`, encoded, string(lines), string(groups), string(ids), turn.TurnRunID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("self-hashed tone forgery accepted", err)
	}
	if _, err := s.ObserveRPSession(ctx, read); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("history accepted tone forgery", err)
	}
}

func TestRPSpeechTonePrivateEmotionDoesNotPublishDelivery(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "delivery-private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "private-only", Text: "你好。"}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我听着。", Private: &core.RPDecisionPrivate{Intent: "PRIVATE_DELIVERY_INTENT", Emotion: "语气温和"}}, nil
	}))
	if err != nil || turn.CompositionVersion != core.RPFactCompositionVersionV2 || strings.Contains(strings.Join(turn.NarrativeLines, ""), "温和") {
		t.Fatal("private emotion became observable delivery", turn, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE payload LIKE '%PRIVATE_DELIVERY_INTENT%'`, nil, 0)
}
