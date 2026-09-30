package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

// E-5: a store close/reopen on the same DB must lose nothing the world is
// made of. Committed events are the fact source; every projection used by the
// RP loop — activity lifecycle, identity familiarity, own-action channel,
// narrative fact window — is either persisted or derivable by replay, so the
// world resumes where it stopped, and RebuildProjections can restore damaged
// projection rows from the events alone.
func TestRPWorldSurvivesStoreRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-restart.db")
	s := openBootstrappedStore(t, ctx, path)
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "restart-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveStudioWorld(ctx, prepareNarrativeWindowWorld(t, ctx, s, "restart-world")); err != nil {
		t.Fatal(err)
	}
	entity, _ := core.StudioWorldObjectID("restart-world", "entity", "lin")
	npc, _ := core.StudioWorldObjectID("restart-world", "entity", "cai")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: "restart-world", BranchID: "br_main", EntityID: entity, POV: "second_person", IdempotencyKey: "restart-world"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}

	// Before the restart: an introduction, then a started activity the wait's
	// deterministic sweep completes. Both must survive the restart.
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	introducing := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我是 Cai。", IntroduceSelf: true}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你是哪位？", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "restart-intro-turn"}, introducing); err != nil {
		t.Fatal(err)
	}
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	acting := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "act", ActivityCode: "tend_accounts"}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你把账理一理。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "restart-act-turn"}, acting); err != nil {
		t.Fatal(err)
	}
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: "2026-09-22T00:35:00Z", Budget: 8, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "restart-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatal("restart sweep wait", wait, err)
	}
	s.Close()

	// The restart: reopen the same DB (no re-bootstrap), replay the activity
	// projection, and confirm the world state the player would see is intact.
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { reopened.Close() }()
	if err := reopened.RebuildProjections(ctx, "restart-world", "br_main"); err != nil {
		t.Fatal("rebuild after restart", err)
	}
	if diffs, err := reopened.CompareProjections(ctx, "restart-world", "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("projection drift after restart", diffs, err)
	}
	assertStudioString(t, ctx, reopened, `SELECT status FROM rp_activities WHERE actor_id=? AND activity_code='tend_accounts'`, []any{npc}, "completed")
	if _, err := reopened.db.ExecContext(ctx, `DELETE FROM rp_own_actions WHERE agent_id=?`, npc); err != nil {
		t.Fatal(err)
	}
	if diffs, err := reopened.CompareProjections(ctx, "restart-world", "br_main"); err != nil || len(diffs) == 0 {
		t.Fatal("deleted own actions were not detected", diffs, err)
	}
	if err := reopened.RebuildProjections(ctx, "restart-world", "br_main"); err != nil {
		t.Fatal("rebuild damaged own actions", err)
	}
	assertM2Value(t, ctx, reopened, `SELECT COUNT(*) FROM rp_own_actions WHERE agent_id=?`, []any{npc}, 3)
	assertM2Value(t, ctx, reopened, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=? AND origin_kind='introduction'`, []any{entity, npc}, 1)
	// introduction speech, the in-progress start, and the settled completion.
	assertM2Value(t, ctx, reopened, `SELECT COUNT(*) FROM rp_own_actions WHERE agent_id=?`, []any{npc}, 3)

	// Replay rebuild is not just persistence: damage the activity row and let
	// RebuildProjections restore the terminal state from the committed events.
	if _, err := reopened.db.ExecContext(ctx, `UPDATE rp_activities SET status='in_progress', end_event_id=NULL WHERE actor_id=?`, npc); err != nil {
		t.Fatal(err)
	}
	if err := reopened.RebuildProjections(ctx, "restart-world", "br_main"); err != nil {
		t.Fatal("rebuild damaged activity", err)
	}
	assertStudioString(t, ctx, reopened, `SELECT status FROM rp_activities WHERE actor_id=? AND activity_code='tend_accounts'`, []any{npc}, "completed")

	// Session resume is idempotent on the same key, and play continues: the
	// next turn's narrative window reaches back across the restart to the
	// settled activity, rendered with the declared prose label.
	resumed, err := reopened.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: "restart-world", BranchID: "br_main", EntityID: entity, POV: "second_person", IdempotencyKey: "restart-world"})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.SessionID != session.SessionID {
		t.Fatalf("session resume changed session id: %q != %q", resumed.SessionID, session.SessionID)
	}
	observation, err = reopened.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	responding := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "账已经理清了。"}, nil
	})
	turn, err := reopened.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "辛苦你了。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "restart-second-turn"}, responding)
	if err != nil {
		t.Fatal(err)
	}
	view, err := reopened.renderRPTurn(ctx, session.SessionID, turn.PlayerTurnID, turn.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(view, "\n")
	if !strings.Contains(joined, "做完了 理账") || !strings.Contains(joined, "账已经理清了。") {
		t.Fatalf("post-restart narrative window lost settled activity: %q", joined)
	}
	if diffs, err := reopened.CompareProjections(ctx, "restart-world", "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("post-restart projection replay", diffs, err)
	}
}
