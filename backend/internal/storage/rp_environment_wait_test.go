package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPEnvironmentSharedAcrossActorsAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-weather.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, firstActor, _ := newRPWaitTestSession(t, ctx, s)
	secondActor := allowFixtureControl(t, ctx, s, M2RPNPCID)
	_, err = s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "stream"), Policy: RPOpportunityPolicy{StreamSeed: "shared-environment", ContactBasisPoints: 0, CooldownHours: 1, HistoryHours: 24}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.DefineRPEnvironmentSource(ctx, EnvironmentSourceRequest{Binding: careerTestBinding(t, s, "principal_creator", "environment"), Source: RPEnvironmentSource{PlaceID: M2AgentCafeID, RainBasisPoints: 5000, CooldownHours: 6}})
	if err != nil {
		t.Fatal(err)
	}
	readCondition := func(event string) rpEnvironmentCondition {
		t.Helper()
		var raw string
		if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, event).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var wait rpWaitEvent
		if err := json.Unmarshal([]byte(raw), &wait); err != nil {
			t.Fatal(err)
		}
		if wait.Environment == nil {
			t.Fatal("missing shared condition")
		}
		return *wait.Environment
	}
	var first rpEnvironmentCondition
	for i, actor := range []core.RPSessionReadRequest{firstActor, secondActor} {
		view, err := s.ObserveRPSession(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		at := "2026-09-22T03:15:00Z"
		if i == 1 {
			at = "2026-09-22T03:45:00Z"
		}
		r := core.RPWaitRequest{PrincipalID: actor.PrincipalID, SessionID: actor.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 100, IdempotencyKey: "weather"}
		if i == 0 {
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "weather rollback") }
			if _, err := s.WaitRP(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("rollback: %v", err)
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted'`, nil, 0)
		}
		out, err := s.WaitRP(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		condition := readCondition(out.EventID)
		if condition.SourceEventID != source.EventID || condition.PlaceID != M2AgentCafeID || condition.UntilWorldTime != "2026-09-22T04:00:00Z" {
			t.Fatalf("invalid shared condition: %+v", condition)
		}
		if i == 0 {
			first = condition
		} else if condition != first {
			t.Fatalf("second actor rerolled: %+v vs %+v", condition, first)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=? AND json_type(payload,'$.environment') IS NOT NULL`, []any{out.EventID}, 0)
		view, err = s.ObserveRPSession(ctx, actor)
		if err != nil || view.Environment == nil || view.Environment.SourceEventID != first.OriginEventID || view.Environment.Condition != first.Condition {
			t.Fatalf("local observation %+v %v", view.Environment, err)
		}
		if i == 0 {
			input, err := s.BuildRPInitiativeInput(ctx, core.RPInitiativeRequest{PrincipalID: actor.PrincipalID, SessionID: actor.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: out.EventID})
			if err != nil || input.Environment == nil || *input.Environment != *view.Environment {
				t.Fatalf("NPC/player disagree on current local environment: %+v %v", input.Environment, err)
			}
			encoded, _ := json.Marshal(input.Environment)
			if strings.Contains(string(encoded), "roll_basis_points") || strings.Contains(string(encoded), "shared-environment") || strings.Contains(string(encoded), source.EventID) {
				t.Fatal("environment context leaked source configuration")
			}
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
		retry, err := s.WaitRP(ctx, r)
		if err != nil || !retry.Replayed || readCondition(retry.EventID) != first {
			t.Fatalf("weather recovery: %+v %v", retry, err)
		}
	}
	view, err := s.ObserveRPSession(ctx, firstActor)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: firstActor.PrincipalID, SessionID: firstActor.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "away-from-weather"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, firstActor)
	if err != nil || view.Environment != nil {
		t.Fatalf("distant weather exposed: %+v %v", view.Environment, err)
	}
	_, err = s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: firstActor.PrincipalID, SessionID: firstActor.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T04:00:00Z", Budget: 100, IdempotencyKey: "environment-expiry"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, secondActor)
	if err != nil || view.Environment != nil {
		t.Fatalf("expired condition remained known-current: %+v %v", view.Environment, err)
	}
}
