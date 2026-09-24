package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

// Select fixture streams before installation to exercise both outcomes. Neither
// personality nor accepted weather is edited to force the resulting action.
func TestRPEnvironmentRainChangesCommittedMovementAndSurvivesRestart(t *testing.T) {
	for _, rain := range []bool{false, true} {
		t.Run(fmt.Sprintf("rain-%t", rain), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "weather-action.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			prepareRPLifeLongWorld(t, ctx, s)
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "weather-session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			gift := socialRequest(t, ctx, s, read, rpLifeNoraID, "gift", "weather-reserve")
			gift.AmountMinor = 200
			if _, err := s.SocialRP(ctx, gift); err != nil {
				t.Fatal(err)
			}
			const at = "2026-09-22T07:15:00Z"
			// Private command IDs depend on identity/key, not the seed or head.
			prospectiveID := func(namespace, command, key string) string {
				keyHash, err := core.HashJSON([]string{"principal_creator", key})
				if err != nil {
					t.Fatal(err)
				}
				hash, err := core.HashJSON([]string{M2DemoInstanceID, M2DemoBranchID, command, keyHash})
				if err != nil {
					t.Fatal(err)
				}
				return "event_" + namespace + "_" + hash[7:]
			}
			policyID := prospectiveID("opportunity", "DefineRPOpportunityPolicy", "weather-policy")
			sourceID := prospectiveID("environment", "DefineRPEnvironmentSource", "weather-source")
			placeIdentity, err := core.HashJSON([]string{"place", M2AgentCafeID})
			if err != nil {
				t.Fatal(err)
			}
			seed := ""
			for i := 0; i < 1000; i++ {
				candidate := fmt.Sprintf("fixture-weather-action-%d", i)
				draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policyID, StreamSeed: candidate, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: placeIdentity, Kind: "local_environment", SourceEventID: sourceID, WorldTime: at}, 5000)
				if err != nil {
					t.Fatal(err)
				}
				if draw.Selected == rain {
					seed = candidate
					break
				}
			}
			if seed == "" {
				t.Fatal("no fixture stream")
			}
			policy, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "weather-policy"), Policy: RPOpportunityPolicy{StreamSeed: seed, ContactBasisPoints: 0, CooldownHours: 1, HistoryHours: 24}})
			if err != nil || policy.EventID != policyID {
				t.Fatalf("policy: %+v %v", policy, err)
			}
			source, err := s.DefineRPEnvironmentSource(ctx, EnvironmentSourceRequest{Binding: careerTestBinding(t, s, "principal_creator", "weather-source"), Source: RPEnvironmentSource{PlaceID: M2AgentCafeID, RainBasisPoints: 5000, CooldownHours: 6}})
			if err != nil || source.EventID != sourceID {
				t.Fatalf("source: %+v %v", source, err)
			}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 100, IdempotencyKey: "weather-wait"})
			if err != nil {
				t.Fatal(err)
			}
			request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: rpLifeNoraID, TriggerEventID: wait.EventID}
			input, err := s.BuildRPInitiativeInput(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			condition, action, place := "clear", "silence", M2AgentCafeID
			if rain {
				condition, action, place = "rain", "leave", "place_m2_home_bo"
			}
			if input.Environment == nil || input.Environment.Condition != condition || input.Environment.SourceEventID != wait.EventID || input.Life == nil || input.Life.Disposition.Caution <= 0 {
				t.Fatalf("actual weather/personality prerequisites: environment=%+v life=%+v", input.Environment, input.Life)
			}
			out, err := s.RunRPInitiative(ctx, request, core.DeterministicRPDecisionProvider{})
			if err != nil || out.Action != action {
				t.Fatalf("weather action: %+v %v; life=%+v", out, err, input.Life)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{rpLifeNoraID, place}, 1)
			moveCount := int64(0)
			if rain {
				moveCount = 1
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND event_type='RPNPCMoved'`, []any{out.EventID}, moveCount)
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
			retry, err := s.RunRPInitiative(ctx, request, core.DeterministicRPDecisionProvider{})
			if err != nil || !retry.Replayed || retry.EventID != out.EventID || retry.Action != action {
				t.Fatalf("weather retry: %+v %v", retry, err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{rpLifeNoraID, place}, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND event_type='RPNPCMoved'`, []any{out.EventID}, moveCount)
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("weather rebuild: %+v %v", differences, err)
			}
			if rain {
				// Repeated observation retains the original occurrence. Six hours
				// starts at07:15, not07:00 and not the later07:45 observation.
				for _, target := range []string{"2026-09-22T07:45:00Z", "2026-09-22T13:00:00Z", "2026-09-22T14:00:00Z"} {
					view, err := s.ObserveRPSession(ctx, read)
					if err != nil {
						t.Fatal(err)
					}
					later, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: target, Budget: 100, IdempotencyKey: target})
					if err != nil {
						t.Fatal(err)
					}
					var raw string
					if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, later.EventID).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					var payload rpWaitEvent
					if err := json.Unmarshal([]byte(raw), &payload); err != nil {
						t.Fatal(err)
					}
					e := payload.Environment
					if e == nil {
						t.Fatal("missing weather receipt")
					}
					switch target {
					case "2026-09-22T07:45:00Z":
						if e.OriginEventID != wait.EventID || e.WorldTime != at || e.Condition != "rain" {
							t.Fatalf("same-hour reroll: %+v", e)
						}
					case "2026-09-22T13:00:00Z":
						if !e.CoolingDown || e.Draw.ChanceBasisPoints != 0 || e.Condition != "clear" {
							t.Fatalf("premature cooldown expiry: %+v", e)
						}
					case "2026-09-22T14:00:00Z":
						if e.CoolingDown || e.Draw.ChanceBasisPoints != 5000 {
							t.Fatalf("observation prolonged cooldown: %+v", e)
						}
					}
				}
			}
		})
	}
}
