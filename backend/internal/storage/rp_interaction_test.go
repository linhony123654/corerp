package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInteractionMoveThenSpeechRecoversOriginalPlan(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "interaction.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	destination := ""
	for _, place := range initial.ReachablePlaces {
		if place.PlaceID == "place_m2_home_ada" && place.CanMoveNow {
			destination = place.DisplayName
		}
	}
	if destination == "" {
		t.Fatal("fixture has no currently reachable Ada home")
	}
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "去" + destination + "，随后说「你好，Ada。」", Mode: "SCENE", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "mixed-original"}
	store.afterRPInteractionStep = func(index int) error {
		if index == 0 {
			return core.NewError(core.CodeInjectedFailure, "reply lost after committed move")
		}
		return nil
	}
	if _, err := service.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected post-move failure, got %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_interactions WHERE status='open' AND next_step=0 AND pending_kind='move'`, nil, 1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err = NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || result.Status != "settled" || result.PlanKind != "MIXED" || len(result.Outcomes) != 2 || result.Outcomes[0].Kind != "move" || result.Outcomes[1].Kind != "speech" || result.Outcomes[0].EventID == "" || result.Outcomes[1].EventID == "" || result.Outcomes[0].EventID == result.Outcomes[1].EventID || result.Outcomes[1].TurnRunID == "" {
		t.Fatalf("original ordered interaction did not recover: %+v %v", result, err)
	}
	after, err := store.ObserveRPSession(ctx, read)
	if err != nil || after.PlaceID != "place_m2_home_ada" {
		t.Fatalf("mixed interaction did not change real position: %+v %v", after, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='你好，Ada。'`, []any{M2RPPlayerID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'`, nil, 1)
	var sequenceBefore int64
	if err := store.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&sequenceBefore); err != nil {
		t.Fatal(err)
	}
	replayed, err := service.RunRPInteraction(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Outcomes[0].EventID != result.Outcomes[0].EventID || replayed.Outcomes[1].EventID != result.Outcomes[1].EventID {
		t.Fatalf("exact retry changed result: %+v %v", replayed, err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, sequenceBefore)
	mismatch := request
	mismatch.Text = "去" + destination + "，随后说「另一句话」"
	if _, err := service.RunRPInteraction(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed original input accepted: %v", err)
	}
	if _, err := service.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: M2AgentAdaPrincipal, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey}); err == nil {
		t.Fatal("foreign principal resumed interaction")
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("mixed world projections differ: %v %v", differences, err)
	}
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("mixed world did not rebuild: %v %v", differences, err)
	}
}

func TestRPInteractionWaitThenSpeechRecoversWithoutRepeatingTime(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wait-then-speech.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "等1小时，随后说「现在可以聊聊了。」", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "wait-then-speech"}
	store.afterRPInteractionStep = func(index int) error {
		if index == 0 {
			return core.NewError(core.CodeInjectedFailure, "reply lost after committed wait")
		}
		return nil
	}
	if _, err := service.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected post-wait failure: %v", err)
	}
	var afterWaitClock string
	if err := store.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&afterWaitClock); err != nil || afterWaitClock <= initial.WorldTime {
		t.Fatalf("wait did not move shared clock: %q %v", afterWaitClock, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err = NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || result.Status != "settled" || result.PlanKind != "MIXED" || len(result.Outcomes) != 2 || result.Outcomes[0].Kind != "wait" || result.Outcomes[1].Kind != "speech" || result.Outcomes[0].SettledSequence < result.Outcomes[0].EventSequence || result.Outcomes[0].SettledSequence >= result.Outcomes[1].EventSequence {
		t.Fatalf("wait/speech recovery changed step order: %+v %v", result, err)
	}
	var afterSpeechClock string
	if err := store.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&afterSpeechClock); err != nil || afterSpeechClock != afterWaitClock {
		t.Fatalf("speech or retry advanced clock again: before=%q after=%q %v", afterWaitClock, afterSpeechClock, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='现在可以聊聊了。'`, []any{M2RPPlayerID}, 1)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("wait/speech diverged after replay: %v %v", differences, err)
	}
}

func TestRPInteractionSpeechChildResultLossRecoversNoDuplicate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "speech-child-loss.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "这句话只能提交一次。", Mode: "DIALOGUE", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "speech-child-result-lost"}
	store.afterRPInteractionStep = func(int) error { return core.NewError(core.CodeInjectedFailure, "reply lost after committed speech") }
	if _, err := service.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected post-speech failure: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text=?`, []any{M2RPPlayerID, request.Text}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_interactions WHERE status='open' AND next_step=0 AND pending_kind='speech'`, nil, 1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err = NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || result.Status != "settled" || len(result.Outcomes) != 1 || result.Outcomes[0].Kind != "speech" || result.Outcomes[0].TurnRunID == "" {
		t.Fatalf("speech child was not recovered: %+v %v", result, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text=?`, []any{M2RPPlayerID, request.Text}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'`, nil, 1)
}

func TestRPInteractionConcurrentExactCallsCommitOnePlan(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "concurrent-interaction.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	var destination string
	for _, place := range initial.ReachablePlaces {
		if place.PlaceID == "place_m2_home_ada" && place.CanMoveNow {
			destination = place.DisplayName
		}
	}
	if destination == "" {
		t.Fatal("fixture has no legal move")
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "去" + destination + "，随后说「并发只算一次。」", Mode: "SCENE", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "concurrent-exact"}
	var group sync.WaitGroup
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.RunRPInteraction(ctx, request)
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil && !core.HasCode(err, core.CodeCommandInProgress) {
			t.Fatalf("concurrent exact call failed outside retryable progress boundary: %v", err)
		}
	}
	result, err := service.RunRPInteraction(ctx, request)
	if err != nil || result.Status != "settled" || len(result.Outcomes) != 2 {
		t.Fatalf("concurrent exact calls did not converge: %+v %v", result, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_interactions WHERE session_id=? AND idempotency_key=?`, []any{session.SessionID, request.IdempotencyKey}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='并发只算一次。'`, []any{M2RPPlayerID}, 1)
}

func TestRPInteractionMigration030UpgradeKeepsExistingWorldAndTurn(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade-030.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, observation := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "升级前的话仍在。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "before-f1-upgrade"})
	if err != nil || turn.Status != "settled" {
		t.Fatalf("old turn: %+v %v", turn, err)
	}
	var beforeHead, beforeKnowledge int64
	if err := store.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&beforeHead); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_knowledge`).Scan(&beforeKnowledge); err != nil {
		t.Fatal(err)
	}
	// Recreate schema030 in this disposable DB by removing only the empty F1
	// application tables; preserve every world Event and existing RP receipt.
	for _, statement := range []string{`DROP TABLE rp_interactions`, `DROP TABLE rp_interaction_mode_revisions`, `DROP TABLE rp_interaction_retirements`, `DELETE FROM schema_meta WHERE schema_version='corerp-f1-interactions-031-2026-09-25'`} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		store, err = Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPInteractionSchemaVersion}, 1)
		assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, beforeHead)
		assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge`, nil, beforeKnowledge)
		if original, err := store.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, TurnRunID: turn.TurnRunID}); err != nil || !strings.Contains(strings.Join(original.View.Lines, "\n"), "升级前的话仍在") {
			t.Fatalf("legacy turn lost during upgrade: %+v %v", original, err)
		}
		if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
			t.Fatalf("upgrade changed projections: %v %v", differences, err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRPInteractionLongNarrativeFixtureStreamRecoveryAndWorldInvariance(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "interaction-long.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, observation := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	speech := strings.Repeat("我把今天真实发生的事按顺序讲给你听，", 60)
	if len([]rune(speech)) < 1000 || len([]rune(speech)) > 2000 {
		t.Fatalf("fixture length outside supported speech contract: %d", len([]rune(speech)))
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: speech, Mode: "DIALOGUE", NarrativeDensity: "long", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "long-narrative-original"}
	result, err := service.RunRPInteraction(ctx, request)
	if err != nil || result.Status != "settled" || len(result.Outcomes) != 1 || result.Outcomes[0].TurnRunID == "" {
		t.Fatalf("long dialogue failed to settle: %+v %v", result, err)
	}
	readNarrative := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, TurnRunID: result.Outcomes[0].TurnRunID}
	view, err := store.ReadRPNarrative(ctx, readNarrative)
	if err != nil || view.Style.Profile.NarrativeDensity != "long" || len([]rune(strings.Join(view.View.Lines, "\n"))) < 1000 || len(view.View.EventIDs) == 0 || view.View.EventIDs[0] != result.Outcomes[0].EventID || !strings.Contains(strings.Join(view.View.Lines, "\n"), "「"+speech+"」") {
		t.Fatalf("long accepted speech was truncated or detached from Event: %+v %v", view, err)
	}
	var head, knowledge int64
	const moneyAndClock = `SELECT c.current_world_time || '|' ||
 (SELECT json_group_array(json_object('account_id',account_id,'balance_minor',balance_minor))
  FROM (SELECT account_id,balance_minor FROM account_balances ORDER BY account_id))
 FROM world_clocks c WHERE c.instance_id=? AND c.branch_id=?`
	var beforeMoneyAndClock string
	if err := store.db.QueryRowContext(ctx, moneyAndClock, M2DemoInstanceID, M2DemoBranchID).Scan(&beforeMoneyAndClock); err != nil {
		t.Fatal(err)
	}
	checkMoneyAndClock := func(s *Store) {
		t.Helper()
		var actual string
		if err := s.db.QueryRowContext(ctx, moneyAndClock, M2DemoInstanceID, M2DemoBranchID).Scan(&actual); err != nil || actual != beforeMoneyAndClock {
			t.Fatalf("narrative presentation changed money or world time: before=%q after=%q err=%v", beforeMoneyAndClock, actual, err)
		}
	}
	if err := store.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_knowledge`).Scan(&knowledge); err != nil {
		t.Fatal(err)
	}
	concise := "concise"
	variant := readNarrative
	variant.StyleOverride = &core.RPStylePatch{NarrativeDensity: &concise}
	if _, err := store.ReadRPNarrative(ctx, variant); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, head)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge`, nil, knowledge)
	checkMoneyAndClock(store)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	streamed := []core.RPNarrativeChunk{}
	after, err := store.StreamRPNarrative(ctx, readNarrative, func(chunk core.RPNarrativeChunk) error {
		streamed = append(streamed, chunk)
		return nil
	})
	if err != nil || !reflect.DeepEqual(after.View.Lines, view.View.Lines) || !reflect.DeepEqual(after.View.EventIDs, view.View.EventIDs) || after.View.RenderID != "" || len(streamed) != len(view.View.Lines) || streamed[0].EventID != result.Outcomes[0].EventID || streamed[0].Line != view.View.Lines[0] {
		t.Fatalf("long stream changed after reopen: %+v chunks=%d %v", after, len(streamed), err)
	}
	service, err = NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.RunRPInteraction(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Outcomes[0].EventID != result.Outcomes[0].EventID {
		t.Fatalf("long presentation caused duplicate world action: %+v %v", replayed, err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, head)
	checkMoneyAndClock(store)
}

func TestRPInteractionActionDialogueWaitAndAmbiguity(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "interaction-modes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, observation := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	destination := ""
	for _, place := range observation.ReachablePlaces {
		if place.PlaceID == "place_m2_home_ada" && place.CanMoveNow {
			destination = place.DisplayName
		}
	}
	move := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "action-only", Text: "去" + destination}
	result, err := service.RunRPInteraction(ctx, move)
	if err != nil || result.Status != "settled" || result.PlanKind != "ACTION" || len(result.Outcomes) != 1 || result.Outcomes[0].Kind != "move" {
		t.Fatalf("action-only interaction: %+v %v", result, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	observation, err = store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	dialogue := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "explicit-dialogue", Mode: "DIALOGUE", Text: "去咖啡馆"}
	result, err = service.RunRPInteraction(ctx, dialogue)
	if err != nil || result.Status != "settled" || result.PlanKind != "DIALOGUE" || len(result.Outcomes) != 1 || result.Outcomes[0].Kind != "speech" {
		t.Fatalf("explicit dialogue became action: %+v %v", result, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='去咖啡馆'`, []any{M2RPPlayerID}, 1)
	observation, err = store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := store.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	ambiguous := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "ambiguous-continue", Text: "继续"}
	result, err = service.RunRPInteraction(ctx, ambiguous)
	if err != nil || result.Status != "clarification" || result.PlanKind != "CLARIFICATION" || result.Clarification == "" || len(result.Outcomes) != 0 {
		t.Fatalf("ambiguous continue caused action: %+v %v", result, err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, before)
	wait := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "wait-one-hour", Text: "等一小时"}
	result, err = service.RunRPInteraction(ctx, wait)
	if err != nil || result.Status != "settled" || result.PlanKind != "ACTION" || len(result.Outcomes) != 1 || result.Outcomes[0].Kind != "wait" || result.Outcomes[0].EventSequence <= before {
		t.Fatalf("wait did not advance shared world: %+v %v", result, err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("interaction modes diverged: %v %v", differences, err)
	}
}

func TestRPInteractionSessionDefaultExplicitOverrideAndPinnedRetry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "interaction-default.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, observation := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	destination := ""
	for _, place := range observation.ReachablePlaces {
		if place.PlaceID == "place_m2_home_ada" && place.CanMoveNow {
			destination = place.DisplayName
		}
	}
	mode, err := store.ReadRPInteractionMode(ctx, read)
	if err != nil || mode.Mode != "AUTO" || mode.Revision != 0 {
		t.Fatalf("wrong legacy session default: %+v %v", mode, err)
	}
	setting := RPInteractionModeSetRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Mode: "DIALOGUE", ExpectedRevision: 0, IdempotencyKey: "default-dialogue"}
	mode, err = store.SetRPInteractionMode(ctx, setting)
	if err != nil || mode.Mode != "DIALOGUE" || mode.Revision != 1 {
		t.Fatalf("cannot set dialogue default: %+v %v", mode, err)
	}
	retry, err := store.SetRPInteractionMode(ctx, setting)
	if err != nil || !retry.Replayed || retry.Revision != mode.Revision {
		t.Fatalf("mode retry not exact: %+v %v", retry, err)
	}
	bad := setting
	bad.Mode = "SCENE"
	if _, err := store.SetRPInteractionMode(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("mode key mismatch accepted: %v", err)
	}
	if _, err := store.SetRPInteractionMode(ctx, RPInteractionModeSetRequest{PrincipalID: M2AgentAdaPrincipal, SessionID: session.SessionID, Mode: "AUTO", IdempotencyKey: "foreign-mode"}); err == nil {
		t.Fatal("foreign principal changed interaction default")
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "inherited-dialogue", Text: "去" + destination}
	result, err := service.RunRPInteraction(ctx, request)
	if err != nil || result.Status != "settled" || result.PlanKind != "DIALOGUE" || len(result.Outcomes) != 1 || result.Outcomes[0].Kind != "speech" {
		t.Fatalf("session default did not override AUTO interpretation: %+v %v", result, err)
	}
	setting = RPInteractionModeSetRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Mode: "SCENE", ExpectedRevision: 1, IdempotencyKey: "default-scene"}
	mode, err = store.SetRPInteractionMode(ctx, setting)
	if err != nil || mode.Revision != 2 {
		t.Fatalf("cannot change default: %+v %v", mode, err)
	}
	replayed, err := service.RunRPInteraction(ctx, request)
	if err != nil || !replayed.Replayed || replayed.PlanKind != "DIALOGUE" || replayed.Outcomes[0].EventID != result.Outcomes[0].EventID {
		t.Fatalf("new default reinterpreted accepted input: %+v %v", replayed, err)
	}
	observation, err = store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	explicit := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "explicit-dialogue-again", Mode: "DIALOGUE", Text: "去" + destination}
	result, err = service.RunRPInteraction(ctx, explicit)
	if err != nil || result.PlanKind != "DIALOGUE" || result.Outcomes[0].Kind != "speech" {
		t.Fatalf("explicit mode lost to session default: %+v %v", result, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mode, err = store.ReadRPInteractionMode(ctx, read)
	if err != nil || mode.Mode != "SCENE" || mode.Revision != 2 {
		t.Fatalf("mode default did not survive reopen: %+v %v", mode, err)
	}
}

func TestRPInteractionPausesAfterInterveningWorldChangeAndStopsWithoutRollback(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "interaction-pause.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	destination := ""
	for _, place := range initial.ReachablePlaces {
		if place.PlaceID == "place_m2_home_ada" && place.CanMoveNow {
			destination = place.DisplayName
		}
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "pause-original", Mode: "SCENE", Text: "去" + destination + "，随后说「旧计划的话」"}
	store.afterRPInteractionStep = func(index int) error { return core.NewError(core.CodeInjectedFailure, "lost after child") }
	if _, err := service.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("did not stop at post-child failure: %v", err)
	}
	store.afterRPInteractionStep = nil
	afterMove, err := store.ObserveRPSession(ctx, read)
	if err != nil || afterMove.PlaceID != "place_m2_home_ada" {
		t.Fatalf("first child not committed: %+v %v", afterMove, err)
	}
	if _, err := store.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, FromPlaceID: afterMove.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: afterMove.ObservationCursor, IdempotencyKey: "independent-return"}); err != nil {
		t.Fatal(err)
	}
	paused, err := service.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || paused.Status != "paused" || paused.NextStep != 1 || len(paused.Outcomes) != 1 || paused.PauseReason == "" {
		t.Fatalf("changed scene did not pause original plan: %+v %v", paused, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='旧计划的话'`, []any{M2RPPlayerID}, 0)
	if _, err := service.RunRPInteraction(ctx, core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: afterMove.ObservationCursor, IdempotencyKey: "new-before-stop", Text: "先说新话"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatalf("paused plan allowed a second active interaction: %v", err)
	}
	if _, err := service.StopRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: M2AgentAdaPrincipal, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey}); err == nil {
		t.Fatal("foreign principal stopped partial plan")
	}
	stopped, err := service.StopRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || stopped.Status != "stopped" || stopped.NextStep != 1 || stopped.Outcomes[0].EventID != paused.Outcomes[0].EventID {
		t.Fatalf("stop erased or repeated committed movement: %+v %v", stopped, err)
	}
	retry, err := service.RunRPInteraction(ctx, request)
	if err != nil || retry.Status != "stopped" || !retry.Replayed || retry.NextStep != 1 {
		t.Fatalf("stopped key restarted old plan: %+v %v", retry, err)
	}
	current, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	newRequest := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: current.ObservationCursor, IdempotencyKey: "new-after-choice", Mode: "DIALOGUE", Text: "我重新选择说话"}
	result, err := service.RunRPInteraction(ctx, newRequest)
	if err != nil || result.Status != "settled" || result.PlanKind != "DIALOGUE" {
		t.Fatalf("new explicit choice remained blocked: %+v %v", result, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'`, nil, 2)
}

func TestRPInteractionRetirementAndUnacceptedPinnedChildStop(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "interaction-retire.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, observation := newRPWaitTestSession(t, ctx, store)
	service, err := NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	retired, err := store.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Operation: "interaction", IdempotencyKey: "never-accepted"})
	if err != nil || retired.Status != "retired" {
		t.Fatalf("could not fence unaccepted interaction: %+v %v", retired, err)
	}
	if _, err := service.RunRPInteraction(ctx, core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "never-accepted", Text: "你好"}); !core.HasCode(err, core.CodeRequestRetired) {
		t.Fatalf("retired interaction key acted: %v", err)
	}
	destination := ""
	for _, place := range observation.ReachablePlaces {
		if place.PlaceID == "place_m2_home_ada" && place.CanMoveNow {
			destination = place.DisplayName
		}
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "prepared-not-accepted", Text: "去" + destination}
	run, replayed, err := service.ensureRPInteraction(ctx, request)
	if err != nil || replayed || run.PendingKind != "" {
		t.Fatalf("could not pin interaction plan: %+v %v", run, err)
	}
	run, err = service.prepareRPInteractionStep(ctx, request, run)
	if err != nil || run.PendingKind != "move" {
		t.Fatalf("could not pin first child request: %+v %v", run, err)
	}
	var child core.RPMoveRequest
	if err := json.Unmarshal([]byte(run.PendingRequest), &child); err != nil {
		t.Fatal(err)
	}
	status, err := store.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Operation: "interaction", IdempotencyKey: request.IdempotencyKey})
	if err != nil || status.Status != "in_progress" {
		t.Fatalf("accepted plan was falsely retired: %+v %v", status, err)
	}
	stopped, err := service.StopRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || stopped.Status != "stopped" || stopped.NextStep != 0 {
		t.Fatalf("unaccepted child could not be safely fenced: %+v %v", stopped, err)
	}
	if _, err := store.MoveRP(ctx, child); !core.HasCode(err, core.CodeRequestRetired) {
		t.Fatalf("retired child executed after stop: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'`, nil, 0)
	status, err = store.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Operation: "interaction", IdempotencyKey: request.IdempotencyKey})
	if err != nil || status.Status != "completed" {
		t.Fatalf("stopped plan did not report final acceptance: %+v %v", status, err)
	}
}
