package storage

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

const rpLifeNoraID = "entity_rp_life_nora"

// The representative world is built only through conserved materialization,
// evidence-backed background and existing economy/scheduler commands.
func prepareRPLifeLongWorld(t *testing.T, ctx context.Context, s *Store) {
	t.Helper()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	materialized, err := s.MaterializeCohort(ctx, m2AgentMaterialization("life_nora", rpLifeNoraID, "Nora", 1, 90, 1, 0, 30, setup.Routine.EventSequence, rpLifeSetupTime))
	if err != nil {
		t.Fatal(err)
	}
	routine := []core.RPBackgroundSchedule{}
	for day := 23; day <= 27; day++ {
		routine = append(routine,
			core.RPBackgroundSchedule{WorldTime: fmt.Sprintf("2026-09-%02dT08:00:00Z", day), PlaceID: "place_m2_work_ada", ActivityCode: "work", EmploymentContractID: m2EconomyContractID},
			core.RPBackgroundSchedule{WorldTime: fmt.Sprintf("2026-09-%02dT12:00:00Z", day), PlaceID: M2AgentCafeID, ActivityCode: "lunch"})
	}
	_, err = s.MaterializeRPBackground(ctx, core.RPBackgroundRequest{
		PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		EntityID: rpLifeNoraID, ExpectedHead: materialized.LastSequence, IdempotencyKey: "life-nora-background",
		AgeMin: 25, AgeMax: 34, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: M2AgentCafeID, Schedule: routine,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRPLifeSixtyTurnsAcrossFiveDaysWithEconomyMemoryAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "life-sixty.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	prepareRPLifeLongWorld(t, ctx, s)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "life-long-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	observe := func() RPObservation {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		return view
	}
	pressure, relieved, work, conflict, background := false, false, false, false, false
	calls := 0
	provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		proposal, err := (core.DeterministicRPDecisionProvider{}).Propose(ctx, input)
		if input.Life == nil {
			t.Fatal("missing permitted life context")
		}
		if input.NPCEntityID == rpLifeNoraID {
			background = background || input.Life.Background != nil && input.Life.Background.EntityID == rpLifeNoraID
			cashNeed := false
			for _, need := range input.Life.Needs {
				if need.Code == "cash_security" {
					cashNeed = true
					if len(need.SourceEventIDs) == 0 {
						t.Fatal("unsourced pressure")
					}
				}
			}
			pressure = pressure || cashNeed && proposal.Action == "refuse"
			relieved = relieved || !cashNeed && input.OwnAssetMinor >= 290
			work = work || !cashNeed && len(input.Life.Employment) > 0 && input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" && proposal.Action == "refuse"
		}
		if input.NPCEntityID == M2RPNPCID && proposal.Action == "leave" {
			for _, g := range input.Life.Goals {
				if g.Code == "avoid_conflict" && len(g.SourceEventIDs) > 0 {
					conflict = true
				}
			}
		}
		return proposal, err
	})
	// Time alone can motivate a sourced NPC choice; no empty/fabricated player
	// speech is used to open a turn. The subsequent60 spoken turns are unchanged.
	initial := observe()
	quietWait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, TargetWorldTime: "2026-09-22T07:10:00Z", Budget: 100, IdempotencyKey: "before-first-speech"})
	if err != nil {
		t.Fatal(err)
	}
	initiative, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: rpLifeNoraID, TriggerEventID: quietWait.EventID}, provider)
	if err != nil || initiative.Action != "respond" || initiative.EventID == "" {
		t.Fatalf("no autonomous life action: %+v %v", initiative, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	for index := 0; index < 60; index++ {
		if index > 0 && index%10 == 0 {
			view := observe()
			wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: fmt.Sprintf("2026-09-%02dT12:00:00Z", 22+index/10), Budget: 1000, IdempotencyKey: fmt.Sprintf("day-%d", index/10)})
			if err != nil || wait.Status != "completed" {
				t.Fatalf("day transition %d: %+v %v", index, wait, err)
			}
		}
		if index == 5 {
			gift := socialRequest(t, ctx, s, read, rpLifeNoraID, "gift", "help-nora")
			gift.AmountMinor = 200
			if _, err := s.SocialRP(ctx, gift); err != nil {
				t.Fatal(err)
			}
		}
		if index == 35 {
			for insult := 0; insult < 6; insult++ {
				if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2RPNPCID, "insult", fmt.Sprintf("conflict-%d", insult))); err != nil {
					t.Fatal(err)
				}
			}
		}
		if index == 40 || index == 50 {
			view := observe()
			to := "place_m2_home_bo"
			if index == 50 {
				to = M2AgentCafeID
			}
			if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: view.PlaceID, ToPlaceID: to, IdempotencyKey: fmt.Sprintf("quiet-move-%d", index)}); err != nil {
				t.Fatal(err)
			}
		}
		view := observe()
		request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: fmt.Sprintf("第%d次日常问候。", index+1), IdempotencyKey: fmt.Sprintf("life-turn-%02d", index)}
		if index == 29 {
			s.afterRPTurnStage = func(stage string) error {
				if stage == "npc_effects_committed" {
					return core.NewError(core.CodeInjectedFailure, "long-run restart")
				}
				return nil
			}
		}
		result, err := s.RunRPTurn(ctx, request, provider)
		if index == 29 {
			if !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("restart was not exercised: %v", err)
			}
			priorCalls := calls
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			result, err = s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey}, provider)
			if calls != priorCalls {
				t.Fatal("recovery repeated committed provider calls")
			}
		}
		if err != nil || result.Status != "settled" {
			t.Fatalf("turn %d: %+v %v", index, result, err)
		}
		if index >= 40 && index < 50 && len(result.NPCEventIDs) != 0 {
			t.Fatal("quiet home interval manufactured NPC activity")
		}
	}
	if !pressure || !relieved || !work || !conflict || !background {
		t.Fatalf("missing causal coverage: pressure=%v relief=%v work=%v conflict=%v background=%v", pressure, relieved, work, conflict, background)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'`, nil, 60)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND movement_kind='scheduled' AND activity_code='work'`, []any{rpLifeNoraID}, 5)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE status='active'`, nil, 5)
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 15)
	assertM2Value(t, ctx, s, `SELECT b.balance_minor FROM account_balances b JOIN materialized_entities n ON n.asset_account_id=b.account_id WHERE n.entity_id=?`, []any{rpLifeNoraID}, 340)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) > 0 {
		t.Fatalf("long-run replay: %+v %v", diff, err)
	}
	t.Logf("60 settled turns; four NPCs/five places; five world days; actual work/wages, pressure relief, conflict departure, quiet interval, interrupted-turn reopen and replay; provider calls=%d", calls)
}

// Four copies of one SQLite snapshot isolate RNG, provider and accumulated
// interaction effects. Seeded providers here are controlled test fixtures, not
// a claim that RP-5's production opportunity/LOD machinery already exists.
func TestRPLifeSameSnapshotDivergentSixtyTurnRuns(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source, err := Open(ctx, filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	prepareRPLifeLongWorld(t, ctx, source)
	type scenario struct {
		name    string
		seeded  bool
		seed    int64
		insults bool
	}
	scenarios := []scenario{{"seed-one", true, 1, false}, {"seed-two", true, 2, false}, {"deterministic-quiet", false, 1, false}, {"deterministic-conflict", false, 1, true}}
	type outcome struct {
		place       string
		leaves      int64
		turns       int64
		initialHash string
	}
	results := map[string]outcome{}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(dir, scenario.name+".db")
			if _, err := source.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
				t.Fatal(err)
			}
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var head int64
			if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
				t.Fatal(err)
			}
			initial, err := s.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, head)
			if err != nil {
				t.Fatal(err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "same-initial-session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			rng := rand.New(rand.NewSource(scenario.seed))
			provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
				if scenario.seeded && input.NPCEntityID == M2RPNPCID && len(input.ReachablePlaceIDs) >= 2 {
					return core.RPDecisionProposal{Action: "leave", DestinationPlaceID: input.ReachablePlaceIDs[rng.Intn(2)]}, nil
				}
				return (core.DeterministicRPDecisionProvider{}).Propose(ctx, input)
			})
			for index := 0; index < 60; index++ {
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				if index > 0 && index%10 == 0 {
					wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: fmt.Sprintf("2026-09-%02dT12:00:00Z", 22+index/10), Budget: 1000, IdempotencyKey: fmt.Sprintf("day-%d", index/10)})
					if err != nil || wait.Status != "completed" {
						t.Fatalf("wait: %+v %v", wait, err)
					}
				}
				if scenario.insults && index == 5 {
					for n := 0; n < 6; n++ {
						if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2RPNPCID, "insult", fmt.Sprintf("conflict-%d", n))); err != nil {
							t.Fatal(err)
						}
					}
				}
				view, err = s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				result, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "今天过得怎么样？", IdempotencyKey: fmt.Sprintf("turn-%02d", index)}, provider)
				if err != nil || result.Status != "settled" {
					t.Fatalf("turn %d: %+v %v", index, result, err)
				}
			}
			out := outcome{initialHash: initial.StateHash}
			if err := s.db.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, M2RPNPCID).Scan(&out.place); err != nil {
				t.Fatal(err)
			}
			if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_npc_decisions WHERE npc_entity_id=? AND action='leave'`, M2RPNPCID).Scan(&out.leaves); err != nil {
				t.Fatal(err)
			}
			if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'`).Scan(&out.turns); err != nil {
				t.Fatal(err)
			}
			if out.turns != 60 {
				t.Fatalf("short run: %+v", out)
			}
			if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) > 0 {
				t.Fatalf("divergent run broke authority: %v %v", diff, err)
			}
			results[scenario.name] = out
			t.Logf("seed=%d seeded_provider=%v interaction_conflict=%v turns=%d Cai_place=%s departures=%d initial_state=%s", scenario.seed, scenario.seeded, scenario.insults, out.turns, out.place, out.leaves, out.initialHash)
		})
	}
	if t.Failed() {
		return
	}
	seed1, seed2, quiet, conflict := results["seed-one"], results["seed-two"], results["deterministic-quiet"], results["deterministic-conflict"]
	for _, out := range results {
		if out.initialHash != quiet.initialHash {
			t.Fatal("runs did not start from the identical world")
		}
	}
	if seed1.place == seed2.place || seed1.leaves != 1 || seed2.leaves != 1 {
		t.Fatal("RNG did not change an actual destination")
	}
	if quiet.place != M2AgentCafeID || quiet.leaves != 0 || seed1.place == quiet.place {
		t.Fatal("provider choice did not change committed facts")
	}
	if conflict.place == quiet.place || conflict.leaves != 1 {
		t.Fatal("accumulated relationship did not change later action")
	}
}
