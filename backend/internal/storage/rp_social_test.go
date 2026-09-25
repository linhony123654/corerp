package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func socialRequest(t *testing.T, ctx context.Context, s *Store, read core.RPSessionReadRequest, target, action, key string) core.RPSocialRequest {
	t.Helper()
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.ReadRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	known, err := rpIdentityKnown(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, target)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !known {
		target, err = rpAnonymousEntityIDForTest(ctx, s, session.InstanceID, session.BranchID, session.ControlledEntityID, target)
		if err != nil {
			t.Fatal(err)
		}
	}
	return core.RPSocialRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetEntityID: target, Action: action, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key}
}
func allowFixtureControl(t *testing.T, ctx context.Context, s *Store, entity string) core.RPSessionReadRequest {
	t.Helper()
	// Test-only control grant. World state/money still changes solely through
	// the real command path, not direct balance or personality edits.
	_, err := s.db.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,'world.rp.control',?,?,?,'[]','active',?)`, "test_control_"+entity, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, entity, m2RPSetupEventID)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: entity, POV: "second_person", IdempotencyKey: "test_control_" + entity})
	if err != nil {
		t.Fatal(err)
	}
	return core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
}
func activeLife(t *testing.T, ctx context.Context, s *Store, read core.RPSessionReadRequest, key string) core.RPDecisionInput {
	t.Helper()
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestRPSocialGiftCreatesRealEconomicNeedGoalDecisionAndReplay(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "gift.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	before := activeLife(t, ctx, s, player, "before-gift")
	choice, _ := (core.DeterministicRPDecisionProvider{}).Propose(ctx, before)
	if choice.Action != "respond" {
		t.Fatal("initial economic state wrong")
	}
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	gift := socialRequest(t, ctx, s, cai, M2RPPlayerID, "gift", "gift")
	gift.AmountMinor = 350
	s.beforeCommit = func() error { return errors.New("injected gift precommit") }
	if _, err := s.SocialRP(ctx, gift); err == nil {
		t.Fatal("gift rollback not injected")
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=(SELECT asset_account_id FROM materialized_entities WHERE entity_id=?)`, []any{M2RPNPCID}, 400)
	result, err := s.SocialRP(ctx, gift)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.SocialRP(ctx, gift)
	if err != nil || !retry.Replayed || retry.EventID != result.EventID {
		t.Fatalf("gift duplicated %+v %v", retry, err)
	}
	after := activeLife(t, ctx, s, player, "after-gift")
	if after.OwnAssetMinor != 50 || after.Life.Goals[0].Code != "collect_money_owed" {
		t.Fatalf("economic chain missing %+v", after.Life)
	}
	found := false
	for _, id := range after.Life.Goals[0].SourceEventIDs {
		if id == result.EventID {
			found = true
		}
	}
	if !found {
		t.Fatal("goal not sourced to actual gift posting")
	}
	changed, _ := (core.DeterministicRPDecisionProvider{}).Propose(ctx, after)
	if changed.Action != "refuse" || !strings.Contains(changed.Text, "开销") {
		t.Fatalf("money did not drive decision %+v", changed)
	}
	assertM2Value(t, ctx, s, `SELECT SUM(balance_minor) FROM account_balances WHERE account_id IN (SELECT asset_account_id FROM materialized_entities WHERE entity_id IN (?,?))`, []any{M2RPNPCID, M2RPPlayerID}, 700)
	differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("gift replay %v %v", differences, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=(SELECT asset_account_id FROM materialized_entities WHERE entity_id=?)`, []any{M2RPNPCID}, 50)
}

func TestRPSocialExperiencedConflictChangesLaterActionAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "relationship.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, _ := newRPWaitTestSession(t, ctx, s)
	for i := 0; i < 3; i++ {
		r := socialRequest(t, ctx, s, read, M2RPNPCID, "insult", string(rune('a'+i)))
		if _, err := s.SocialRP(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: "after-conflict"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.NarrativeLines[1], "离开了") {
		t.Fatalf("experience did not change real action %+v", result)
	}
	actual, err := s.ObserveRPSession(ctx, read)
	if err != nil || len(actual.PresentEntities) != 0 {
		t.Fatalf("NPC did not actually leave %+v %v", actual, err)
	}
	differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("social replay %v %v", differences, err)
	}
}

func TestRPSocialMeetingObligationTrustAndMemoryRequireActualFulfillment(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "promise.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, initial := newRPWaitTestSession(t, ctx, s)
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	at, _ := time.Parse(time.RFC3339, initial.WorldTime)
	at = at.Add(20 * time.Minute)
	promise := socialRequest(t, ctx, s, cai, M2RPPlayerID, "promise_meeting", "promise")
	promise.MeetingPlaceID = initial.PlaceID
	promise.MeetingWorldTime = at.Format(time.RFC3339)
	promised, err := s.SocialRP(ctx, promise)
	if err != nil {
		t.Fatal(err)
	}
	input := activeLife(t, ctx, s, player, "during-promise")
	if len(input.Life.Commitments) != 1 || input.Life.Relationships[0].Obligation != 1 || input.Life.Relationships[0].Role != "meeting_partner" {
		t.Fatalf("missing obligation %+v", input.Life)
	}
	choice, _ := (core.DeterministicRPDecisionProvider{}).Propose(ctx, input)
	if choice.Action != "wait" {
		t.Fatalf("commitment did not affect choice %+v", choice)
	}
	early := socialRequest(t, ctx, s, cai, M2RPPlayerID, "keep_meeting", "early")
	early.PromiseEventID = promised.EventID
	if _, err := s.SocialRP(ctx, early); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("early meeting accepted %v", err)
	}
	view, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: promise.MeetingWorldTime, Budget: 100, IdempotencyKey: "meeting-time"})
	if err != nil {
		t.Fatal(err)
	}
	keep := socialRequest(t, ctx, s, cai, M2RPPlayerID, "keep_meeting", "keep")
	keep.PromiseEventID = promised.EventID
	kept, err := s.SocialRP(ctx, keep)
	if err != nil {
		t.Fatal(err)
	}
	again := socialRequest(t, ctx, s, cai, M2RPPlayerID, "keep_meeting", "second-keep")
	again.PromiseEventID = promised.EventID
	if _, err := s.SocialRP(ctx, again); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("promise fulfilled twice %v", err)
	}
	after := activeLife(t, ctx, s, player, "after-promise")
	if len(after.Life.Commitments) != 0 || after.Life.Relationships[0].Obligation != 0 || after.Life.Relationships[0].Trust != 1 || after.Life.SalientMemories[0].SourceEventID != kept.EventID {
		t.Fatalf("meeting outcome not grounded %+v", after.Life)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	replayed := activeLife(t, ctx, s, player, "after-rebuild")
	if replayed.Life.Relationships[0].Trust != 1 || replayed.Life.Relationships[0].Obligation != 0 {
		t.Fatalf("relationship rebuild drift %+v", replayed.Life.Relationships)
	}
}

func TestRPSocialRejectsOffsiteUnfundedStaleAndConflictingRequests(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "social-negative.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	offsite := socialRequest(t, ctx, s, read, M2AgentAdaID, "greet", "offsite")
	if _, err := s.SocialRP(ctx, offsite); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("offsite social act accepted %v", err)
	}
	unfunded := socialRequest(t, ctx, s, read, M2RPNPCID, "gift", "unfunded")
	unfunded.AmountMinor = 301
	if _, err := s.SocialRP(ctx, unfunded); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("unfunded gift accepted %v", err)
	}
	stale := socialRequest(t, ctx, s, read, M2RPNPCID, "greet", "stale")
	stale.ExpectedCursor--
	if _, err := s.SocialRP(ctx, stale); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale gesture accepted %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, initial.ObservationCursor)
	valid := socialRequest(t, ctx, s, read, M2RPNPCID, "greet", "valid")
	if _, err := s.SocialRP(ctx, valid); err != nil {
		t.Fatal(err)
	}
	mismatch := valid
	mismatch.Action = "insult"
	if _, err := s.SocialRP(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("key mismatch accepted %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInterpersonalAction'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE json_extract(claim_payload,'$.claim_type')='interpersonal_action'`, nil, 2)
}
