package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func nonverbalReadEvent(t *testing.T, events RPClientEvents, sequence int64) RPClientEvent {
	t.Helper()
	for _, event := range events.Events {
		if event.Sequence == sequence {
			return event
		}
	}
	t.Fatalf("missing visible nonverbal event at sequence %d: %+v", sequence, events.Events)
	return RPClientEvent{}
}

func nonverbalReadFact(t *testing.T, facts []RPContextFact, action string) RPContextFact {
	t.Helper()
	for _, fact := range facts {
		if fact.Kind == "nonverbal_action" && fact.Action == action {
			return fact
		}
	}
	t.Fatalf("nonverbal %s absent from observer evidence: %+v", action, facts)
	return RPContextFact{}
}

func nonverbalHistory(t *testing.T, turns []RPHistoryTurn, eventID string) string {
	t.Helper()
	for _, turn := range turns {
		if turn.TurnRunID == eventID {
			if len(turn.NarrativeLines) != 1 {
				t.Fatalf("nonverbal history must contain one sourced line: %+v", turn)
			}
			return turn.NarrativeLines[0]
		}
	}
	t.Fatalf("nonverbal history lacks event %s", eventID)
	return ""
}

func TestRPNonverbalScopedReadsFreezeUnseenTargetAndAnonymousWitness(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nonverbal-read.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, actorRead, _ := newRPWaitTestSession(t, ctx, s)
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
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID, FromPlaceID: adaView.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "read-ada-joins-cafe"}); err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"north", "south"} {
		a, b := "main", zone
		if a > b {
			a, b = b, a
		}
		if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "read-link-"+zone), PlaceID: M2AgentCafeID, ZoneA: a, ZoneB: b, BarrierKind: "open", BarrierState: "open", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
			t.Fatal(err)
		}
	}
	for _, placement := range []struct{ key, agent, zone string }{{"read-npc-north", M2RPNPCID, "north"}, {"read-ada-south", M2AgentAdaID, "south"}} {
		if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", placement.key), AgentID: placement.agent, PlaceID: M2AgentCafeID, ZoneKey: placement.zone}); err != nil {
			t.Fatal(err)
		}
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, M2RPPlayerID)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, actorRead)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.NonverbalRP(ctx, core.RPNonverbalRequest{PrincipalID: actorRead.PrincipalID, SessionID: actorRead.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "read-look-npc", Action: "look_at", TargetEntityID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	// Granting the NPC control is a disposable fixture setup, not a simulated
	// NPC action; its own session observes precisely the same committed Event.
	npcRead := allowFixtureControl(t, ctx, s, M2RPNPCID)
	check := func() {
		t.Helper()
		actorEvents, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: actorRead.PrincipalID, SessionID: actorRead.SessionID, After: result.EventSequence - 1})
		if err != nil {
			t.Fatal(err)
		}
		own := nonverbalReadEvent(t, actorEvents, result.EventSequence)
		if own.EventID != result.EventID || own.OwnAction == nil || own.OwnAction.Kind != "nonverbal" || own.OwnAction.Action != "look_at" || own.OwnAction.TargetEntityID != M2RPNPCID || len(own.Facts) != 0 {
			t.Fatalf("actor history invented self observation or lost own event: %+v", own)
		}
		actorView, err := s.ObserveRPSession(ctx, actorRead)
		if err != nil || nonverbalHistory(t, actorView.RecentTurns, result.EventID) != "有人看向另一人。" {
			t.Fatalf("actor historical action is missing: %v", err)
		}
		for _, response := range []any{actorEvents, actorView} {
			encoded, err := json.Marshal(response)
			if err != nil || strings.Contains(string(encoded), M2AgentAdaID) || strings.Contains(string(encoded), `"witnesses"`) {
				t.Fatalf("actor read leaked privileged Event witness list: %s %v", encoded, err)
			}
		}

		adaEvents, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID, After: result.EventSequence - 1})
		if err != nil {
			t.Fatal(err)
		}
		seen := nonverbalReadEvent(t, adaEvents, result.EventSequence)
		if seen.EventID == result.EventID || seen.OwnAction != nil || len(seen.Facts) != 1 {
			t.Fatalf("anonymous witness saw privileged Event or acted: %+v", seen)
		}
		fact := nonverbalReadFact(t, seen.Facts, "look_at")
		if fact.SubjectEntityID != alias || fact.TargetEntityID != "" || fact.SourceEventID != seen.EventID || fact.Text != "有人看向某处。" {
			t.Fatalf("witness saw unseen target or unmasked actor: %+v", fact)
		}
		adaContext, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID, SubjectEntityID: alias})
		if err != nil {
			t.Fatal(err)
		}
		known := nonverbalReadFact(t, adaContext.Facts, "look_at")
		if known.SubjectEntityID != alias || known.TargetEntityID != "" || known.SourceEventID != seen.EventID || known.Text != fact.Text {
			t.Fatalf("context leaked unseen target or disagreed with Event: %+v", known)
		}
		adaView, err := s.ObserveRPSession(ctx, adaRead)
		if err != nil || nonverbalHistory(t, adaView.RecentTurns, seen.EventID) != "有人看向某处。" {
			t.Fatalf("witness recent history leaked target: %v", err)
		}
		for _, response := range []any{adaEvents, adaContext, adaView} {
			encoded, err := json.Marshal(response)
			if err != nil || strings.Contains(string(encoded), M2RPPlayerID) || strings.Contains(string(encoded), M2RPNPCID) || strings.Contains(string(encoded), `"witnesses"`) {
				t.Fatalf("anonymous witness response leaked raw identities or Event witness list: %s %v", encoded, err)
			}
		}

		npcEvents, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: npcRead.PrincipalID, SessionID: npcRead.SessionID, After: result.EventSequence - 1})
		if err != nil {
			t.Fatal(err)
		}
		npcSeen := nonverbalReadEvent(t, npcEvents, result.EventSequence)
		if npcSeen.OwnAction != nil || len(npcSeen.Facts) != 1 || npcSeen.Facts[0].TargetEntityID != M2RPNPCID || npcSeen.Facts[0].Text != "有人看向另一人。" {
			t.Fatalf("visible target did not witness target-specific action: %+v", npcSeen)
		}
		npcContext, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: npcRead.PrincipalID, SessionID: npcRead.SessionID})
		if err != nil || nonverbalReadFact(t, npcContext.Facts, "look_at").TargetEntityID != M2RPNPCID {
			t.Fatalf("visible target's context lost the target: %+v %v", npcContext, err)
		}
		npcView, err := s.ObserveRPSession(ctx, npcRead)
		if err != nil || nonverbalHistory(t, npcView.RecentTurns, npcSeen.EventID) != "有人看向另一人。" {
			t.Fatalf("visible target's recent history is incomplete: %v", err)
		}
		for _, response := range []any{npcEvents, npcContext, npcView} {
			encoded, err := json.Marshal(response)
			if err != nil || strings.Contains(string(encoded), M2AgentAdaID) || strings.Contains(string(encoded), `"witnesses"`) {
				t.Fatalf("target read leaked unrelated witness identity: %s %v", encoded, err)
			}
		}
	}
	check()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	check()
}

func TestRPNonverbalBlindCoLocatedActorHasNoReadReceipt(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "nonverbal-blind-read.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, actorRead, _ := newRPWaitTestSession(t, ctx, s)
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "blind-read-door"), PlaceID: M2AgentCafeID, ZoneA: "backroom", ZoneB: "main", BarrierKind: "door", BarrierState: "closed", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", "blind-read-npc-zone"), AgentID: M2RPNPCID, PlaceID: M2AgentCafeID, ZoneKey: "backroom"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, actorRead)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.NonverbalRP(ctx, core.RPNonverbalRequest{PrincipalID: actorRead.PrincipalID, SessionID: actorRead.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "unseen-smile", Action: "smile"})
	if err != nil {
		t.Fatal(err)
	}
	npcRead := allowFixtureControl(t, ctx, s, M2RPNPCID)
	events, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: npcRead.PrincipalID, SessionID: npcRead.SessionID, After: result.EventSequence - 1})
	if err != nil || len(events.Events) != 0 {
		t.Fatalf("wall-crossing co-location leaked action Event: %+v %v", events.Events, err)
	}
	contextView, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: npcRead.PrincipalID, SessionID: npcRead.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range contextView.Facts {
		if fact.Kind == "nonverbal_action" {
			t.Fatalf("blind observer acquired action knowledge: %+v", fact)
		}
	}
	observation, err := s.ObserveRPSession(ctx, npcRead)
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range observation.RecentTurns {
		if turn.TurnRunID == result.EventID {
			t.Fatalf("blind observer received action history: %+v", turn)
		}
	}
}

func TestRPNonverbalOwnAnonymousTargetReadsRemainAnonymousAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nonverbal-read-anonymous.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "read-meet-ada"}); err != nil {
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
	result, err := s.NonverbalRP(ctx, core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "read-anonymous-look", Action: "look_at", TargetEntityID: alias})
	if err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		events, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, After: result.EventSequence - 1})
		if err != nil {
			t.Fatal(err)
		}
		own := nonverbalReadEvent(t, events, result.EventSequence)
		if own.OwnAction == nil || own.OwnAction.TargetEntityID != alias || len(own.Facts) != 0 {
			t.Fatalf("own action leaked target identity or invented self observation: %+v", own)
		}
		contextView, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, SubjectEntityID: alias})
		if err != nil {
			t.Fatal(err)
		}
		for _, fact := range contextView.Facts {
			if fact.Kind == "nonverbal_action" && fact.SourceEventID == result.EventID {
				t.Fatalf("own act forged self-observation: %+v", fact)
			}
		}
		history, err := s.ObserveRPSession(ctx, read)
		if err != nil || nonverbalHistory(t, history.RecentTurns, result.EventID) != "有人看向另一人。" {
			t.Fatalf("own action disappeared from recent history: %v", err)
		}
		for _, response := range []any{events, contextView, history} {
			encoded, err := json.Marshal(response)
			if err != nil || strings.Contains(string(encoded), M2AgentAdaID) || strings.Contains(string(encoded), `"witnesses"`) {
				t.Fatalf("own anonymous-target response leaked internal identity or witness IDs: %s %v", encoded, err)
			}
		}
	}
	check()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	check()
}
