package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPVisitSourcesActualMemoryNotFriendsCurrentPosition(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "visit-memory.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	r := newBackgroundCandidate(t, ctx, s)
	r.Schedule = r.Schedule[:1] // Nora goes home at03:00 and has no invented later trip.
	if _, err := s.MaterializeRPBackground(ctx, r); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "visit-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	for _, key := range []string{"gift-one", "gift-two"} {
		gift := socialRequest(t, ctx, s, read, r.EntityID, "gift", key)
		gift.AmountMinor = 1
		if _, err := s.SocialRP(ctx, gift); err != nil {
			t.Fatal(err)
		}
	}
	var lastSeen string
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(observed_world_time) FROM observation_records WHERE observer_agent_id=? AND subject_agent_id=?`, r.EntityID, M2RPPlayerID).Scan(&lastSeen); err != nil {
		t.Fatal(err)
	}
	seenAt, err := time.Parse(time.RFC3339, lastSeen)
	if err != nil {
		t.Fatal(err)
	}
	move := func(place, key string) {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: place, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key}); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(at time.Time, key string) {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at.UTC().Format(time.RFC3339), Budget: 1000, IdempotencyKey: key})
		if err != nil || out.CurrentWorldTime != at.UTC().Format(time.RFC3339) {
			t.Fatalf("actual wait: %+v %v", out, err)
		}
	}
	sources := func(start string) []core.RPVisitSource {
		t.Helper()
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		input, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: r.EntityID})
		if err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		got, err := readRPVisitSources(ctx, tx.conn, input, start)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	move("place_m2_home_ada", "player-away")
	boundary := seenAt.Add(core.RPOldFriendVisitMinimumGap)
	wait(boundary.Add(-time.Second), "before-week")
	before := sources(lastSeen)
	if len(before) != 1 || before[0].Kind != "familiar_public_place" || before[0].PlaceID != M2AgentCafeID {
		t.Fatalf("early old-friend/own visit source: %+v", before)
	}
	wait(boundary, "full-week")
	full := sources(lastSeen)
	if len(full) != 2 || full[0].Kind != "old_friend_place" || full[0].FriendID != M2RPPlayerID || full[0].PlaceID != M2AgentCafeID || full[0].RememberedWorldTime != lastSeen || full[0].ObservationID == "" {
		t.Fatalf("sourced rare visit: %+v", full)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_home_ada'`, []any{M2RPPlayerID}, 1)
	// Moving the friend remotely must not change Nora's remembered destination
	// or last-seen time, even when the friend happens to revisit that place.
	move(M2AgentCafeID, "friend-cafe")
	if !reflect.DeepEqual(full, sources(lastSeen)) {
		t.Fatal("remote position changed own visit memory")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(full, sources(lastSeen)) {
		t.Fatal("restart changed visit sources")
	}
	move("place_m2_home_bo", "actual-reencounter")
	after := sources(lastSeen)
	if len(after) != 1 || after[0].Kind != "familiar_public_place" {
		t.Fatalf("actual recent encounter ignored: %+v", after)
	}
	move(M2AgentCafeID, "friend-departs")
	if len(sources(lastSeen)) != 1 {
		t.Fatal("old public encounter resurrected after private meeting")
	}
	// The source history itself expires, independently of any future draw.
	wait(boundary.Add(2*time.Second), "expiry-clock")
	if got := sources(boundary.Add(time.Second).Format(time.RFC3339)); len(got) != 0 {
		t.Fatalf("expired own visits recycled: %+v", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("visit source projections: %+v %v", diff, err)
	}
}
