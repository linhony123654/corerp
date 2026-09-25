package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPPerceptionDoorWallDistanceAndSpeechChannelAreIndependent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "perception.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	if len(view.PresentEntities) != 1 || view.PresentEntities[0].EntityID != M2RPNPCID {
		t.Fatal("fixture has no co-located NPC", view.PresentEntities)
	}
	define := func(key, zone, kind, state string, distance, visual, audio int) {
		t.Helper()
		a, b := "main", zone
		if a > b {
			a, b = b, a
		}
		if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", key), PlaceID: M2AgentCafeID, ZoneA: a, ZoneB: b, BarrierKind: kind, BarrierState: state, DistanceM: distance, VisualRangeM: visual, AudioRangeM: audio}); err != nil {
			t.Fatal(err)
		}
	}
	place := func(key, zone string) {
		t.Helper()
		if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", key), AgentID: M2RPNPCID, PlaceID: M2AgentCafeID, ZoneKey: zone}); err != nil {
			t.Fatal(err)
		}
	}
	speak := func(key, channel string, expected int) {
		t.Helper()
		current, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		result, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: current.ObservationCursor, IdempotencyKey: key, Text: "这句话是否听见？", DeliveryChannel: channel})
		if err != nil || len(result.ListenerIDs) != expected {
			t.Fatal("wrong actual hearing", result, err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{result.EventID, M2RPNPCID}, int64(expected))
	}
	define("closed-door", "backroom", "door", "closed", 1, 30, 30)
	place("behind-door", "backroom")
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.PresentEntities) != 0 {
		t.Fatal("closed door leaked visual presence", view.PresentEntities, err)
	}
	if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2RPNPCID, "greet", "door-gesture")); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("closed door allowed a targeted visual action", err)
	}
	speak("door-voice", "voice", 0)
	define("solid-wall", "wallside", "wall", "closed", 1, 100, 100)
	place("behind-wall", "wallside")
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.PresentEntities) != 0 {
		t.Fatal("wall leaked visual presence", view.PresentEntities, err)
	}
	if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2RPNPCID, "greet", "wall-gesture")); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("wall allowed a targeted visual action", err)
	}
	speak("wall-shout", "shout", 0)
	define("far-open-passage", "porch", "open", "open", 8, 5, 30)
	place("far-away", "porch")
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.PresentEntities) != 0 {
		t.Fatal("out-of-range sight leaked", view.PresentEntities, err)
	}
	encounter, err := s.ResolveEncounter(ctx, core.EncounterRead{PrincipalID: "principal_creator", CapabilityID: "world.encounter.read", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ObserverAgentID: M2RPPlayerID, Fields: []string{"place_id", "participants", "evidence"}})
	if err != nil || len(encounter.Participants) != 0 {
		t.Fatal("encounter query leaked invisible participant", encounter, err)
	}
	speak("far-whisper", "whisper", 0)
	speak("far-voice", "voice", 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE delivery_channel='whisper'`, nil, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("perception changed replay projections", diffs, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_perception_links SET audio_range_m=1 WHERE place_id=? AND zone_a='main' AND zone_b='porch'`, M2AgentCafeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_actor_zones SET zone_key='main' WHERE agent_id=?`, M2RPNPCID); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 2 {
		t.Fatal("perception corruption missed", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("perception projection repair", err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("perception repair not stable", diffs, err)
	}
}
