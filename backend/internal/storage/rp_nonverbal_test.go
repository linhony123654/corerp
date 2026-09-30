package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPNonverbalActionIsNeutralAtomicAndRecoverable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nonverbal.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	actions := []struct{ action, gesture, target string }{
		{"look_at", "", M2RPNPCID},
		{"smile", "", ""},
		{"nod", "", M2RPNPCID},
		{"shake_head", "", ""},
		{"gesture", "wave", ""},
		{"turn_away", "", M2RPNPCID},
	}
	var first core.RPNonverbalRequest
	var result RPNonverbalResult
	for i, choice := range actions {
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: choice.action, GestureCode: choice.gesture, TargetEntityID: choice.target, ExpectedCursor: view.ObservationCursor, IdempotencyKey: choice.action}
		if i == 0 {
			first = request
			s.beforeCommit = func() error { return errors.New("injected nonverbal precommit") }
			if _, err := s.NonverbalRP(ctx, request); err == nil {
				t.Fatal("precommit failure did not roll back nonverbal action")
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'`, nil, 0)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM commands WHERE command_type='RPNonverbalAction'`, nil, 0)
			assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, initial.ObservationCursor)
		}
		result, err = s.NonverbalRP(ctx, request)
		if err != nil || result.Replayed || result.EventSequence != view.ObservationCursor+1 || result.WorldTime != view.WorldTime {
			t.Fatalf("%s was not atomic at current world time: %+v %v", choice.action, result, err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=? AND subject_agent_id=?`, []any{result.EventID, M2RPNPCID, M2RPPlayerID}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=? AND observer_agent_id=?`, []any{result.EventID, M2RPNPCID}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=subject_agent_id`, []any{result.EventID}, 0)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE event_id=? AND agent_id=? AND action='nonverbal'`, []any{result.EventID, M2RPPlayerID}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=? AND topic='rp.nonverbal'`, []any{result.EventID}, 2)
		var raw string
		if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, result.EventID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var fact core.RPNonverbalFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Action != choice.action || fact.TargetEntityID != choice.target || len(fact.Witnesses) != 1 || fact.Witnesses[0].ObserverEntityID != M2RPNPCID || fact.Witnesses[0].TargetVisible != (choice.target != "") {
			t.Fatalf("%s lacked frozen visual evidence: %+v %v", choice.action, fact, err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInterpersonalAction'`, nil, 0)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE source_event_id IN (SELECT event_id FROM events WHERE event_type='RPNonverbalAction')`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM journal_entries WHERE event_id IN (SELECT event_id FROM events WHERE event_type='RPNonverbalAction')`, nil, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	prior, err := s.NonverbalRP(ctx, first)
	if err != nil || !prior.Replayed || prior.EventSequence != initial.ObservationCursor+1 || prior.WorldTime != initial.WorldTime {
		t.Fatalf("committed key did not recover after restart: %+v %v", prior, err)
	}
	changed := first
	changed.Action = "nod"
	if _, err := s.NonverbalRP(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("same key accepted different expression: %v", err)
	}
	out, err := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Operation: "nonverbal", IdempotencyKey: first.IdempotencyKey})
	if err != nil || out.Status != "completed" {
		t.Fatalf("retirement erased accepted action: %+v %v", out, err)
	}
	retired := first
	retired.IdempotencyKey = "retired-before-accept"
	if out, err := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Operation: "nonverbal", IdempotencyKey: retired.IdempotencyKey}); err != nil || out.Status != "retired" {
		t.Fatalf("fresh key retirement failed: %+v %v", out, err)
	}
	if _, err := s.NonverbalRP(ctx, retired); !core.HasCode(err, core.CodeRequestRetired) {
		t.Fatalf("retired key accepted: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'`, nil, int64(len(actions)))
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("nonverbal projection comparison failed: %v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id IN (SELECT event_id FROM events WHERE event_type='RPNonverbalAction')`, nil, int64(len(actions)))
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE action='nonverbal'`, nil, int64(len(actions)))
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("nonverbal rebuild was not stable: %v %v", diffs, err)
	}
}

func TestRPNonverbalWitnessVisualAndTargetPrivacyAreFrozen(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "witnesses.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, playerRead, _ := newRPWaitTestSession(t, ctx, s)
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
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID, FromPlaceID: adaView.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "witness-joins-cafe"}); err != nil {
		t.Fatal(err)
	}
	define := func(key, zone string) {
		t.Helper()
		a, b := "main", zone
		if a > b {
			a, b = b, a
		}
		if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", key), PlaceID: M2AgentCafeID, ZoneA: a, ZoneB: b, BarrierKind: "open", BarrierState: "open", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
			t.Fatal(err)
		}
	}
	define("actor-north", "north")
	define("actor-south", "south")
	for _, actor := range []struct{ key, agent, zone string }{{"npc-north", M2RPNPCID, "north"}, {"ada-south", M2AgentAdaID, "south"}} {
		if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", actor.key), AgentID: actor.agent, PlaceID: M2AgentCafeID, ZoneKey: actor.zone}); err != nil {
			t.Fatal(err)
		}
	}
	view, err := s.ObserveRPSession(ctx, playerRead)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "see-actor-not-target", Action: "look_at", TargetEntityID: M2RPNPCID}
	result, err := s.NonverbalRP(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, result.EventID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var fact core.RPNonverbalFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || len(fact.Witnesses) != 2 || fact.Witnesses[0].ObserverEntityID != M2AgentAdaID || fact.Witnesses[0].TargetVisible || fact.Witnesses[1].ObserverEntityID != M2RPNPCID || !fact.Witnesses[1].TargetVisible {
		t.Fatalf("wrong frozen visual relations: %+v %v", fact.Witnesses, err)
	}
	var adaClaim, adaOutbox string
	if err := s.db.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, result.EventID, M2AgentAdaID).Scan(&adaClaim); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE outbox_id=?`, "outbox_"+result.EventID+"_"+M2AgentAdaID).Scan(&adaOutbox); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(adaClaim, M2RPNPCID) || strings.Contains(adaOutbox, M2RPNPCID) || strings.Contains(adaOutbox, M2RPPlayerID) {
		t.Fatal("witness claim/outbox disclosed unseen target or unknown actor", adaClaim, adaOutbox)
	}
	var actorOutbox string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE outbox_id=?`, "outbox_"+result.EventID+"_"+M2RPPlayerID).Scan(&actorOutbox); err != nil || strings.Contains(actorOutbox, M2AgentAdaID) || strings.Contains(actorOutbox, "witnesses") {
		t.Fatal("actor outbox leaked a bystander they may not know", actorOutbox, err)
	}
	var npcClaim string
	if err := s.db.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, result.EventID, M2RPNPCID).Scan(&npcClaim); err != nil || !strings.Contains(npcClaim, M2RPNPCID) {
		t.Fatal("visible target absent from their witness claim", npcClaim, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{result.EventID, M2RPPlayerID}, 0)
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", "ada-sees-target-later"), AgentID: M2AgentAdaID, PlaceID: M2AgentCafeID, ZoneKey: "north"}); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("later witness movement rewrote historical evidence: %v %v", diffs, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE observation_records SET claim_payload=json_set(claim_payload,'$.target_entity_id',?) WHERE source_event_id=? AND observer_agent_id=?`, M2RPNPCID, result.EventID, M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) == 0 {
		t.Fatalf("tampered historical visibility passed projection comparison: %v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	var rebuilt string
	if err := s.db.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, result.EventID, M2AgentAdaID).Scan(&rebuilt); err != nil || rebuilt != adaClaim {
		t.Fatalf("rebuild recomputed historical sight: %s != %s (%v)", rebuilt, adaClaim, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM rp_own_actions WHERE agent_id=? AND event_id=?`, M2RPPlayerID, result.EventID); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) == 0 {
		t.Fatalf("missing autobiographical action passed projection comparison: %v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE agent_id=? AND event_id=?`, []any{M2RPPlayerID, result.EventID}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("nonverbal tamper repair not stable: %v %v", diffs, err)
	}
}

func TestRPNonverbalConcurrentSameKeyCommitsOneFact(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "nonverbal-concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "one-concurrent-nod", Action: "nod"}
	type answer struct {
		result RPNonverbalResult
		err    error
	}
	var results [6]answer
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := range results {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			results[i].result, results[i].err = s.NonverbalRP(ctx, request)
		}(i)
	}
	close(start)
	workers.Wait()
	first, newCount := results[0].result.EventID, 0
	for i, result := range results {
		if result.err != nil || result.result.EventID != first || result.result.EventSequence != view.ObservationCursor+1 {
			t.Fatalf("concurrent action %d disagreed: %+v", i, result)
		}
		if !result.result.Replayed {
			newCount++
		}
	}
	if newCount != 1 {
		t.Fatalf("concurrent key committed %d fresh receipts, want one", newCount)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'`, nil, 1)
}

func TestRPNonverbalAnonymousTargetUsesPublicHandleWithoutIdentification(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "nonverbal-anonymous.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "meet-unknown-for-expression"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "anonymous-look", Action: "look_at", TargetEntityID: M2AgentAdaID}
	if _, err := s.NonverbalRP(ctx, request); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("guessed authoritative identity could target a stranger", err)
	}
	request.TargetEntityID = alias
	result, err := s.NonverbalRP(ctx, request)
	if err != nil || strings.Contains(result.Description, M2AgentAdaID) {
		t.Fatalf("anonymous visible target could not be looked at: %+v %v", result, err)
	}
	var factRaw, ownOutbox string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, result.EventID).Scan(&factRaw); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE outbox_id=?`, "outbox_"+result.EventID+"_"+M2RPPlayerID).Scan(&ownOutbox); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(factRaw, M2AgentAdaID) || strings.Contains(ownOutbox, M2AgentAdaID) || !strings.Contains(ownOutbox, alias) {
		t.Fatalf("authoritative Event and public outbox lost identity boundary: %s / %s", factRaw, ownOutbox)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=?`, []any{M2RPPlayerID, M2AgentAdaID}, 0)
}

func TestRPNonverbalRejectsInvisibleAndGuessedTargets(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "nonverbal-validation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "invalid", Action: "look_at"}
	if _, err := s.NonverbalRP(ctx, request); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("look_at did not require a target", err)
	}
	request.TargetEntityID = M2AgentAdaID
	if _, err := s.NonverbalRP(ctx, request); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("unknown authoritative target accepted", err)
	}
	request.TargetEntityID = M2RPNPCID
	request.Action, request.GestureCode = "gesture", "sit"
	if _, err := s.NonverbalRP(ctx, request); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("unsupported gesture inferred posture", err)
	}
	request.Action, request.GestureCode = "look_at", ""
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "closed-actor-door"), PlaceID: M2AgentCafeID, ZoneA: "backroom", ZoneB: "main", BarrierKind: "door", BarrierState: "closed", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", "npc-behind-closed-door"), AgentID: M2RPNPCID, PlaceID: M2AgentCafeID, ZoneKey: "backroom"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedCursor = view.ObservationCursor
	if _, err := s.NonverbalRP(ctx, request); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("invisible target accepted", err)
	}
	request.Action, request.TargetEntityID = "smile", ""
	result, err := s.NonverbalRP(ctx, request)
	if err != nil {
		t.Fatal("untargeted smile must not be blocked by unseen NPC", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=?`, []any{result.EventID}, 0)
}
