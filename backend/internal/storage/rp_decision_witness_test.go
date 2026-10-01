package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPDecisionNonverbalWitnessProjectionFreezesTargetAndSourceTime(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "decision-witness.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	ada, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	adaRead := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID}
	view, err := s.ObserveRPSession(ctx, adaRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "witness-ada-arrival"}); err != nil {
		t.Fatal(err)
	}
	act := func(key, action, target, gesture string) RPNonverbalResult {
		t.Helper()
		v, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		if target == M2AgentAdaID {
			target, err = rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, target)
			if err != nil {
				t.Fatal(err)
			}
		}
		r, err := s.NonverbalRP(ctx, core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: v.ObservationCursor, IdempotencyKey: key, Action: action, TargetEntityID: target, GestureCode: gesture})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	directed := act("toward-npc", "nod", M2RPNPCID, "")
	other := act("toward-other", "gesture", M2AgentAdaID, "wave")
	for _, zone := range []string{"north", "south"} {
		if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "witness-link-"+zone), PlaceID: M2AgentCafeID, ZoneA: "main", ZoneB: zone, BarrierKind: "open", BarrierState: "open", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []struct{ actor, zone string }{{M2RPNPCID, "north"}, {M2AgentAdaID, "south"}} {
		if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", "witness-zone-"+p.zone), AgentID: p.actor, PlaceID: M2AgentCafeID, ZoneKey: p.zone}); err != nil {
			t.Fatal(err)
		}
	}
	hidden := act("hidden-target", "look_at", M2AgentAdaID, "")
	var historicalClaim string
	if err := s.db.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, hidden.EventID, M2RPNPCID).Scan(&historicalClaim); err != nil {
		t.Fatal(err)
	}
	var frozen core.RPNonverbalClaim
	if err := json.Unmarshal([]byte(historicalClaim), &frozen); err != nil || frozen.TargetEntityID != "" {
		t.Fatalf("fixture did not hide target: %+v %v", frozen, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "witness-later-time", TargetWorldTime: at.Add(time.Minute).UTC().Format(time.RFC3339), Budget: 32}); err != nil {
		t.Fatal(err)
	}
	build := func(key string) core.RPDecisionInput {
		t.Helper()
		v, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: v.ObservationCursor, IdempotencyKey: key, Text: "刚才的动作，你看见了吗？"})
		if err != nil {
			t.Fatal(err)
		}
		input, err := s.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: r.TurnID, NPCEntityID: M2RPNPCID})
		if err != nil {
			t.Fatal(err)
		}
		return input
	}
	find := func(input core.RPDecisionInput, event string) core.RPDecisionKnowledge {
		t.Helper()
		for _, k := range input.Knowledge {
			if k.SourceEventID == event {
				return k
			}
		}
		t.Fatalf("missing sourced witness %s", event)
		return core.RPDecisionKnowledge{}
	}
	input := build("witness-question")
	if k := find(input, directed.EventID); k.Action != "nod" || k.TargetEntityID != M2RPNPCID || k.SubjectEntityID != M2RPPlayerID || k.WorldTime != directed.WorldTime {
		t.Fatalf("directed witness: %+v", k)
	}
	if k := find(input, other.EventID); k.Action != "gesture" || k.GestureCode != "wave" || k.TargetEntityID != M2AgentAdaID || k.WorldTime != other.WorldTime {
		t.Fatalf("other-target witness: %+v", k)
	}
	if k := find(input, hidden.EventID); k.Action != "look_at" || k.TargetEntityID != "" || k.WorldTime != hidden.WorldTime || k.WorldTime == input.WorldTime {
		t.Fatalf("hidden historical witness: %+v", k)
	}
	projected, err := s.rpDecisionProviderView(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, s, input.InstanceID, input.BranchID, input.NPCEntityID, M2AgentAdaID)
	if err != nil {
		t.Fatal(err)
	}
	if k := find(projected, other.EventID); k.TargetEntityID != alias || k.SourceEventID != other.EventID || k.WorldTime != other.WorldTime {
		t.Fatalf("stranger target was unmasked or provenance changed: %+v", k)
	}
	if k := find(projected, hidden.EventID); k.TargetEntityID != "" {
		t.Fatalf("provider restored hidden target: %+v", k)
	}
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "witness-later-visibility"), PlaceID: M2AgentCafeID, ZoneA: "north", ZoneB: "south", BarrierKind: "open", BarrierState: "open", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
		t.Fatal(err)
	}
	input = build("witness-later-question")
	if k := find(input, hidden.EventID); k.TargetEntityID != "" {
		t.Fatalf("later sight granted old target: %+v", k)
	}
	for _, damage := range []struct{ name, sql string }{
		{"knowledge target", `UPDATE agent_knowledge SET claim_payload=json_set(claim_payload,'$.target_entity_id','forged') WHERE source_event_id=? AND observer_agent_id=?`},
		{"observer scope", `UPDATE observation_records SET observer_agent_id='entity_m2_agent_bo' WHERE source_event_id=? AND observer_agent_id=?`},
		{"actor attribution", `UPDATE observation_records SET claim_payload=json_set(claim_payload,'$.actor_entity_id','forged') WHERE source_event_id=? AND observer_agent_id=?`},
	} {
		t.Run(damage.name, func(t *testing.T) {
			tx, err := beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.conn.ExecContext(ctx, damage.sql, directed.EventID, M2RPNPCID); err != nil {
				t.Fatal(err)
			}
			fresh := input
			fresh.Knowledge = nil
			if _, err := readRPOwnDecisionContext(ctx, tx.conn, fresh); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatalf("damaged witness accepted: %v", err)
			}
		})
	}
	for _, forged := range []struct{ name, event, expression string }{
		{"both forged action", directed.EventID, `json_set(claim_payload,'$.action','shake_head','$.description','有人摇了摇头。')`},
		{"both forged target", directed.EventID, `json_set(claim_payload,'$.target_entity_id','entity_m2_agent_ada')`},
		{"both restore historically unseen target", hidden.EventID, `json_set(claim_payload,'$.target_entity_id','entity_m2_agent_ada','$.description','有人看向另一人。')`},
	} {
		t.Run(forged.name, func(t *testing.T) {
			tx, err := beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			for _, table := range []string{"agent_knowledge", "observation_records"} {
				if _, err := tx.conn.ExecContext(ctx, `UPDATE `+table+` SET claim_payload=`+forged.expression+` WHERE source_event_id=? AND observer_agent_id=?`, forged.event, M2RPNPCID); err != nil {
					t.Fatal(err)
				}
			}
			fresh := input
			fresh.Knowledge = nil
			if _, err := readRPOwnDecisionContext(ctx, tx.conn, fresh); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatalf("matching forged projections accepted: %v", err)
			}
		})
	}
	t.Run("legacy omitted typed fields", func(t *testing.T) {
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		for _, table := range []string{"agent_knowledge", "observation_records"} {
			if _, err := tx.conn.ExecContext(ctx, `UPDATE `+table+` SET claim_payload=json_remove(claim_payload,'$.action','$.gesture_code','$.target_entity_id','$.actor_entity_id') WHERE source_event_id=? AND observer_agent_id=?`, directed.EventID, M2RPNPCID); err != nil {
				t.Fatal(err)
			}
		}
		fresh := input
		fresh.Knowledge = nil
		legacy, err := readRPOwnDecisionContext(ctx, tx.conn, fresh)
		if err != nil {
			t.Fatal(err)
		}
		if k := find(legacy, directed.EventID); k.Action != "" || k.GestureCode != "" || k.TargetEntityID != "" || k.WorldTime != directed.WorldTime {
			t.Fatalf("legacy invented fields: %+v", k)
		}
	})
}
