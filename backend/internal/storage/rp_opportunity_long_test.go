package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

// Same actual world and player policy; production opportunity streams alone
// differ. The provider is the real deterministic implementation, not a fixture
// returning preselected destinations or generated random actions.
func TestRPOpportunitySameWorldFortnightDivergenceAndReplay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	baseline, err := Open(ctx, filepath.Join(dir, "baseline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer baseline.Close()
	setup, err := baseline.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const nora = "entity_emergent_nora"
	materialized, err := baseline.MaterializeCohort(ctx, m2AgentMaterialization("emergent_nora", nora, "Nora", 1, 200, 1, 0, 0, setup.Routine.EventSequence, rpLifeSetupTime))
	if err != nil {
		t.Fatal(err)
	}
	var schedule []core.RPBackgroundSchedule
	// Background accepts a bounded first week, with actual route transitions.
	// Do not fabricate a second residence or silently expand that contract.
	for day := 1; day <= 7; day++ {
		schedule = append(schedule, core.RPBackgroundSchedule{WorldTime: careerTime(day, 6, 0), PlaceID: "place_m2_home_bo", ActivityCode: "home"})
		if day < 7 {
			schedule = append(schedule, core.RPBackgroundSchedule{WorldTime: careerTime(day, 21, 0), PlaceID: M2AgentCafeID, ActivityCode: "present"})
		}
	}
	if _, err := baseline.MaterializeRPBackground(ctx, core.RPBackgroundRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: nora, ExpectedHead: materialized.LastSequence, IdempotencyKey: "stream-nora", AgeMin: 25, AgeMax: 34, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: M2AgentCafeID, Schedule: schedule}); err != nil {
		t.Fatal(err)
	}
	session, err := baseline.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "stream-friend"})
	if err != nil {
		t.Fatal(err)
	}
	lin := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	for _, key := range []string{"old-friend-one", "old-friend-two"} {
		gift := socialRequest(t, ctx, baseline, lin, nora, "gift", key)
		gift.AmountMinor = 1
		if _, err := baseline.SocialRP(ctx, gift); err != nil {
			t.Fatal(err)
		}
	}
	view, err := baseline.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := baseline.MoveRP(ctx, core.RPMoveRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_work_ada", IdempotencyKey: "lin-away"}); err != nil {
		t.Fatal(err)
	}
	bo := allowFixtureControl(t, ctx, baseline, M2AgentBoID)
	noraAlias, err := rpAnonymousEntityIDForTest(ctx, baseline, M2DemoInstanceID, M2DemoBranchID, M2AgentBoID, nora)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		InitialHash                                     string
		Visits                                          []string
		Gifts, Waits, Quiet, Calls, RareDraws, RareHits int
		NoraCash, Trust                                 int64
		FinalHash                                       string
	}
	results := map[string]outcome{}
	for _, run := range []struct{ name, stream string }{{"a", "life-stream-a"}, {"b", "life-stream-b"}, {"c", "life-stream-c"}, {"a-repeated", "life-stream-a"}} {
		t.Run(run.name, func(t *testing.T) {
			path := filepath.Join(dir, run.name+".db")
			if _, err := baseline.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
				t.Fatal(err)
			}
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			out := outcome{}
			replayHash := func() string {
				t.Helper()
				var head int64
				if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
					t.Fatal(err)
				}
				state, err := s.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, head)
				if err != nil {
					t.Fatal(err)
				}
				return state.StateHash
			}
			out.InitialHash = replayHash()
			if _, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "life-stream-policy"), Policy: RPOpportunityPolicy{StreamSeed: run.stream, VisitBasisPoints: 300, RareVisitBasisPoints: 100, ContactBasisPoints: 200, WarmEnabled: true, CooldownHours: 6, HistoryHours: 240}}); err != nil {
				t.Fatal(err)
			}
			provider := rpDecisionProviderFunc(func(ctx context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
				out.Calls++
				if in.Life == nil {
					t.Fatal("missing own life context")
				}
				return (core.DeterministicRPDecisionProvider{}).Propose(ctx, in)
			})
			service, err := NewRPService(s, provider, "deterministic")
			if err != nil {
				t.Fatal(err)
			}
			move := func(to, key string) {
				t.Helper()
				for step := 0; step < 2; step++ {
					view, err := s.ObserveRPSession(ctx, bo)
					if err != nil {
						t.Fatal(err)
					}
					if view.PlaceID == to {
						return
					}
					destination := to
					if view.PlaceID != M2AgentCafeID && to != M2AgentCafeID {
						destination = M2AgentCafeID
					}
					if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: bo.PrincipalID, SessionID: bo.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: view.PlaceID, ToPlaceID: destination, IdempotencyKey: fmt.Sprintf("%s-%d", key, step)}); err != nil {
						t.Fatal(err)
					}
				}
			}
			for day := 1; day <= 14; day++ {
				for _, hour := range []int{6, 7, 9, 10, 12, 18, 19, 20} {
					at := careerTime(day, hour, 15)
					destination := "place_m2_home_bo"
					if hour == 12 {
						destination = M2AgentCafeID
					}
					move(destination, "daily-"+at)
					view, err := s.ObserveRPSession(ctx, bo)
					if err != nil {
						t.Fatal(err)
					}
					request := core.RPWaitRequest{PrincipalID: bo.PrincipalID, SessionID: bo.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: "wait-" + at}
					before := out.Calls
					wait, err := service.WaitRP(ctx, request)
					if err != nil || wait.Status != "completed" || wait.CurrentWorldTime != at {
						t.Fatalf("day%d hour%d wait: %+v %v", day, hour, wait, err)
					}
					if out.Calls-before > core.RPHotInitiativeLimit || len(wait.WarmNPCIDs) > core.RPWarmDecisionLimit {
						t.Fatal("unbounded model/decision work")
					}
					out.Waits++
					quiet := true
					for _, effect := range wait.Initiatives {
						quiet = quiet && (effect.Action == "silence" || effect.Action == "wait")
						if effect.NPCEntityID == nora && effect.Action == "leave" {
							out.Visits = append(out.Visits, at)
						}
					}
					if quiet {
						out.Quiet++
					}
					var raw string
					if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, wait.EventID).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					var fact rpWaitEvent
					if err := json.Unmarshal([]byte(raw), &fact); err != nil {
						t.Fatal(err)
					}
					for _, visit := range fact.VisitOpportunities {
						if !visit.Rare {
							continue
						}
						out.RareDraws++
						if visit.Draw.ChanceBasisPoints > 100 || visit.Quiet {
							t.Fatal("rare guarantee/bonus introduced")
						}
						if visit.Draw.Selected {
							out.RareHits++
						}
					}
					for _, effect := range wait.Initiatives {
						if effect.NPCEntityID != nora || effect.Action != "leave" {
							continue
						}
						sourced := false
						for _, visit := range fact.VisitOpportunities {
							sourced = sourced || visit.ActorID == nora && visit.Draw.Selected
						}
						if !sourced {
							t.Fatal("counted an unsourced departure as stream-driven visit")
						}
					}
					calls := out.Calls
					if retry, err := service.WaitRP(ctx, request); err != nil || !retry.Replayed || calls != out.Calls {
						t.Fatalf("long-run retry: %+v %v", retry, err)
					}
					if hour == 12 {
						view, err := s.ObserveRPSession(ctx, bo)
						if err != nil {
							t.Fatal(err)
						}
						for _, person := range view.PresentEntities {
							if person.EntityID != noraAlias {
								continue
							}
							gift := socialRequest(t, ctx, s, bo, nora, "gift", "lunch-gift-"+at)
							gift.AmountMinor = 1
							if _, err := s.SocialRP(ctx, gift); err != nil {
								t.Fatal(err)
							}
							out.Gifts++
						}
					}
				}
				if day == 7 {
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
					service, err = NewRPService(s, provider, "deterministic")
					if err != nil {
						t.Fatal(err)
					}
				}
				if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
					t.Fatalf("day%d projections: %+v %v", day, diff, err)
				}
			}
			if out.Waits != 112 || out.Quiet*2 < out.Waits {
				t.Fatalf("ordinary life crowded out: %+v", out)
			}
			assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 15)
			assertM2Value(t, ctx, s, `SELECT SUM(population_count) FROM materialized_entities WHERE status='active'`, nil, 5)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_economic_obligations WHERE kind='wage'`, nil, 14)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_consumption_outcomes WHERE status='consumed'`, nil, 14)
			// None of the declared sources authorizes a new Career transition.
			// Optional outings must not manufacture major life events to fill time.
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded'`, nil, 0)
			tx, err := beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			input, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: nora, InterlocutorEntityID: M2AgentBoID})
			tx.Rollback(ctx)
			if err != nil {
				t.Fatal(err)
			}
			out.NoraCash = input.OwnAssetMinor
			for _, relationship := range input.Life.Relationships {
				if relationship.SubjectEntityID == M2AgentBoID {
					out.Trust = int64(relationship.Trust)
				}
			}
			if out.Trust != int64(out.Gifts) {
				t.Fatalf("actual encounters/gifts not reflected in relationship: %+v", out)
			}
			out.FinalHash = replayHash()
			results[run.name] = out
			t.Logf("stream=%s outcomes=%+v", run.stream, out)
		})
	}
	if t.Failed() {
		return
	}
	if !reflect.DeepEqual(results["a"], results["a-repeated"]) {
		t.Fatalf("same recorded stream did not replay: %+v / %+v", results["a"], results["a-repeated"])
	}
	baselineHash := results["a"].InitialHash
	paths, financial := map[string]bool{}, map[int64]bool{}
	for _, name := range []string{"a", "b", "c"} {
		out := results[name]
		if out.InitialHash != baselineHash {
			t.Fatal("runs do not share actual initial world")
		}
		path, _ := core.HashJSON(out.Visits)
		paths[path] = true
		financial[out.NoraCash] = true
	}
	if len(paths) < 2 || len(financial) < 2 {
		t.Fatalf("streams lack substantive path/economic divergence: %+v", results)
	}
}
