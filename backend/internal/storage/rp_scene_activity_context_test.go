package storage

import (
	"context"
	"testing"

	"corerp.local/backend/internal/core"
)

// The actor starts a real typed activity while the observer is behind a closed
// door. Later visibility cannot make its unobserved completion known.
func TestRPSceneActivityContextDoesNotReadThroughClosedDoorOrRecoverHiddenEnd(t *testing.T) {
	f := newRPFocusFixtureWithSystem(t, "orchestrated", studioActivityTestPackage())
	ctx := context.Background()
	actor, observer := f.ids[len(f.ids)-1], f.ids[0]
	home, _ := core.StudioWorldObjectID(f.world, "place", "home")
	setDoor := func(state, key string) {
		t.Helper()
		_, err := f.s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: f.binding(t, key), PlaceID: home, ZoneA: "far", ZoneB: "main", BarrierKind: "door", BarrierState: state, DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5})
		if err != nil {
			t.Fatal(err)
		}
	}
	setZone := func(id, zone, key string) {
		t.Helper()
		if _, err := f.s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: f.binding(t, key), AgentID: id, PlaceID: home, ZoneKey: zone}); err != nil {
			t.Fatal(err)
		}
	}
	setDoor("closed", "scene-door-closed")
	setZone(f.player, "far", "scene-player-far")
	setZone(actor, "far", "scene-actor-far")
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: f.names[actor] + "，先整理账目吧。", IdempotencyKey: "hidden-activity-start"}, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		if in.NPCEntityID != actor {
			t.Fatal("wrong activity actor")
		}
		return core.RPDecisionProposal{Action: "act", ActivityCode: "tend_accounts"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	var start string
	if err := f.s.db.QueryRowContext(ctx, `SELECT start_event_id FROM rp_activities WHERE actor_id=?`, actor).Scan(&start); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{start, observer}, 0)
	setZone(f.player, "main", "scene-player-main")
	canonical, provider := captureRPSceneActivityContext(t, f, observer, "hidden-current")
	if len(canonical.SceneActivities) != 0 || len(provider.SceneActivities) != 0 {
		t.Fatal("unseen activity entered the canonical or provider scene packet")
	}
	// Entering the actor's zone legitimately reveals an ongoing activity.
	// That proves a current snapshot, not when it secretly began.
	view, err = f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T00:01:00Z", Budget: 100, IdempotencyKey: "scene-before-open"}); err != nil {
		t.Fatal(err)
	}
	setZone(observer, "far", "scene-observer-now-visible")
	setZone(f.player, "far", "scene-player-follows-observer")
	canonical, _ = captureRPSceneActivityContext(t, f, observer, "visible-current")
	if len(canonical.SceneActivities) != 1 || canonical.SceneActivities[0].ActorID != actor || canonical.SceneActivities[0].Status != "in_progress" {
		t.Fatal("currently visible activity was lost")
	}
	seen := canonical.SceneActivities[0]
	if seen.SourceEventID != start || seen.ObservationBasis != "current_visibility" || seen.WorldTime != canonical.WorldTime || !core.RPDecisionEvidenceEventIDs(canonical)[start] {
		t.Fatal("current scene lost provenance or disclosed an unobserved start time")
	}
	setZone(observer, "main", "scene-observer-no-longer-visible")
	setZone(f.player, "main", "scene-player-leaves-actor-zone")
	view, err = f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T00:35:00Z", Budget: 100, IdempotencyKey: "scene-hidden-end"}); err != nil {
		t.Fatal(err)
	}
	var end string
	if err := f.s.db.QueryRowContext(ctx, `SELECT end_event_id FROM rp_activities WHERE actor_id=? AND status='completed'`, actor).Scan(&end); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{end, observer}, 0)
	setZone(observer, "far", "scene-observer-after-hidden-end")
	setZone(f.player, "far", "scene-player-after-hidden-end")
	canonical, provider = captureRPSceneActivityContext(t, f, observer, "hidden-end-not-recovered")
	if len(canonical.SceneActivities) != 0 || len(provider.SceneActivities) != 0 {
		t.Fatal("later visibility revealed an unwitnessed terminal activity")
	}
	canonical, _ = captureRPSceneActivityContext(t, f, actor, "own-known-end")
	if len(canonical.SceneActivities) != 1 || canonical.SceneActivities[0].Status != "completed" || canonical.SceneActivities[0].SourceEventID != end || canonical.SceneActivities[0].ObservationBasis != "own_action" || !core.RPDecisionEvidenceEventIDs(canonical)[end] {
		t.Fatal("an actor lost its own committed outcome or its source")
	}
	if err := f.s.RebuildProjections(ctx, f.world, "br_main"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.s, err = Open(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	canonical, provider = captureRPSceneActivityContext(t, f, observer, "hidden-end-after-restart")
	if len(canonical.SceneActivities) != 0 || len(provider.SceneActivities) != 0 {
		t.Fatal("restart/rebuild recovered an unobserved terminal activity")
	}
	if differences, err := f.s.CompareProjections(ctx, f.world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatal(differences, err)
	}
}

func captureRPSceneActivityContext(t *testing.T, f *rpFocusFixture, observer, key string) (core.RPDecisionInput, core.RPDecisionInput) {
	t.Helper()
	ctx := context.Background()
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	var canonical, provider core.RPDecisionInput
	_, err = f.s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: f.names[observer] + "，眼下这里有什么动静？", IdempotencyKey: key}, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		if in.NPCEntityID != observer {
			t.Fatal("wrong scene observer")
		}
		provider = in
		var err error
		canonical, err = f.s.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, TurnID: in.TurnID, NPCEntityID: observer})
		if err != nil {
			t.Fatal(err)
		}
		return core.RPDecisionProposal{Action: "respond", Text: "我只说我看见的。"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	return canonical, provider
}

func TestRPSceneActivityContextRejectsUnsupportedSourceAndStaleVersion(t *testing.T) {
	f := newRPFocusFixtureWithSystem(t, "orchestrated", studioActivityTestPackage())
	ctx := context.Background()
	actor := f.ids[0]
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: f.names[actor] + "，请整理账目。", IdempotencyKey: "activity-source-start"}, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "act", ActivityCode: "tend_accounts"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	view, err = f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := core.StudioWorldObjectID(f.world, "place", "home")
	in := core.RPDecisionInput{InstanceID: f.world, BranchID: "br_main", NPCEntityID: actor, PlaceID: home, WorldTime: view.WorldTime, HeadSequence: view.ObservationCursor}
	conn, err := f.s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	activities, err := readRPSceneActivityContext(ctx, conn, in)
	if err != nil || len(activities) != 1 || activities[0].ObservationBasis != "own_action" || activities[0].SourceEventID == "" {
		t.Fatal("own activity source was lost", err)
	}
	stale := in
	stale.HeadSequence = 0
	if _, err := readRPSceneActivityContext(ctx, conn, stale); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("scene included a source after its pinned version", err)
	}
	// This is an isolated projection-corruption probe, not a canon change.
	if _, err := conn.ExecContext(ctx, `UPDATE rp_activities SET activity_code='sweep_court' WHERE actor_id=?`, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := readRPSceneActivityContext(ctx, conn, in); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("projection renamed an activity without a committed source", err)
	}
}
