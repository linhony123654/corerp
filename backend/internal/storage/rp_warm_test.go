package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPWarmImportantRelationshipRespectsClosureAndSameWaitHOT(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprint(closed), func(t *testing.T) {
			ctx := context.Background()
			s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "warm-relation.db"))
			defer s.Close()
			if _, err := s.RunAgentLife(ctx, careerTime(1, 7, 0), 1000); err != nil {
				t.Fatal(err)
			}
			if closed {
				if _, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "closure"), FromPlaceID: "place_m2_home_ada", ToPlaceID: M2AgentCafeID, StartsAt: careerTime(1, 7, 5), EndsAt: careerTime(1, 8, 30)}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "warm-policy"), Policy: RPOpportunityPolicy{StreamSeed: "relations", WarmEnabled: true, CooldownHours: 1, HistoryHours: 24}}); err != nil {
				t.Fatal(err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "warm-rel-session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			move := func(to, key string) {
				t.Helper()
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: to, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key}); err != nil {
					t.Fatal(err)
				}
			}
			move("place_m2_home_ada", "meet-ada")
			if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2AgentAdaID, "insult", "actual-conflict")); err != nil {
				t.Fatal(err)
			}
			move(M2AgentCafeID, "return-cafe")
			calls := 0
			provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
				calls++
				if in.NPCEntityID == M2AgentAdaID {
					t.Fatal("new WARM arrival inserted into pinned HOT roster")
				}
				return core.RPDecisionProposal{Action: "silence"}, nil
			})
			service, err := NewRPService(s, provider, "deterministic")
			if err != nil {
				t.Fatal(err)
			}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			r := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 7, 15), Budget: 1000, IdempotencyKey: "relation-wait"}
			out, err := service.WaitRP(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.WarmNPCIDs) != 1 || out.WarmNPCIDs[0] != M2AgentAdaID || calls == 0 {
				t.Fatalf("missing WARM/HOT execution: %+v calls=%d", out, calls)
			}
			var raw string
			if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_type='RPWarmDecisionRecorded'`).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var fact rpWarmFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				t.Fatal(err)
			}
			if fact.Selection.Reason != "important_relationship" {
				t.Fatalf("unsourced importance: %+v", fact.Selection)
			}
			wantAction, wantPlace := "leave", M2AgentCafeID
			if closed {
				wantAction, wantPlace = "wait", "place_m2_home_ada"
			}
			if fact.Decision.Action != wantAction {
				t.Fatalf("route closure ignored: %+v", fact)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2AgentAdaID, wantPlace}, 1)
			before := calls
			if _, err := service.WaitRP(ctx, r); err != nil || calls != before {
				t.Fatalf("same wait repeated effects: %v calls=%d", err, calls)
			}
			other, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "another-session"})
			if err != nil {
				t.Fatal(err)
			}
			view, err = s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: other.SessionID})
			if err != nil {
				t.Fatal(err)
			}
			next, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: other.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 7, 30), Budget: 1000, IdempotencyKey: "other-wait"})
			if err != nil || len(next.WarmNPCIDs) != 0 {
				t.Fatalf("session bypassed WARM cadence: %+v %v", next, err)
			}
			if closed {
				// Keep the actually important actor off-scene at the boundary.
				move("place_m2_work_bo", "observe-elsewhere")
				for _, at := range []string{careerTime(1, 13, 14)[:17] + "59Z", careerTime(1, 13, 15)} {
					view, err = s.ObserveRPSession(ctx, read)
					if err != nil {
						t.Fatal(err)
					}
					boundary, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: at})
					if err != nil {
						t.Fatal(err)
					}
					want := 0
					if at == careerTime(1, 13, 15) {
						want = 1
					}
					if len(boundary.WarmNPCIDs) != want {
						t.Fatalf("six-hour boundary %s: %+v", at, boundary)
					}
					if want == 1 {
						decision, err := s.runRPWarmDecision(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2AgentAdaID, TriggerEventID: boundary.EventID})
						if err != nil || decision.Fact.Decision.Action != "wait" {
							t.Fatalf("released quiet decision: %+v %v", decision, err)
						}
					}
				}
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWarmDecisionRecorded'`, nil, 2)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("relation warm recovery: %+v %v", differences, err)
			}
		})
	}
}

func TestRPWarmActualPreparationCadencePrivacyAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "warm.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "warm-policy"), Policy: RPOpportunityPolicy{StreamSeed: "warm-world", WarmEnabled: true, CooldownHours: 1, HistoryHours: 24}}); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "warm-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_work_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "visit-workplace"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	r := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 7, 15), Budget: 1000, IdempotencyKey: "before-shift"}
	wait, err := s.WaitRP(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(wait.WarmNPCIDs) != 1 || wait.WarmNPCIDs[0] != M2AgentAdaID {
		t.Fatalf("actual imminent appointment roster: %+v", wait)
	}
	response, _ := json.Marshal(wait)
	if strings.Contains(string(response), "warm") || strings.Contains(string(response), M2AgentAdaID) {
		t.Fatalf("private warm roster exposed: %s", response)
	}
	var broadcast string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE event_id=?`, wait.EventID).Scan(&broadcast); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(broadcast, "warm_candidates") || strings.Contains(broadcast, M2AgentAdaID) {
		t.Fatal("off-scene selection broadcast")
	}
	request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2AgentAdaID, TriggerEventID: wait.EventID}
	wrong := request
	wrong.NPCEntityID = M2AgentBoID
	if _, err := s.runRPWarmDecision(ctx, wrong); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("unselected actor: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "warm rollback") }
	if _, err := s.runRPWarmDecision(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWarmDecisionRecorded'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_home_ada'`, []any{M2AgentAdaID}, 1)
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
	fact, err := s.runRPWarmDecision(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if fact.Fact.Decision.Action != "leave" || fact.Fact.Decision.ToPlaceID != M2AgentCafeID || len(fact.Fact.WorkPath) != 3 || len(fact.Fact.RouteSourceEventIDs) != 2 || fact.Fact.Decision.ScheduleSourceEventID == "" {
		t.Fatalf("not actual own multi-hop preparation: %+v", fact)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=? AND activity_code='rp_npc_leave'`, []any{M2AgentAdaID, M2AgentCafeID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM journal_entries WHERE event_id=?`, []any{fact.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{fact.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, fact.EventID}, 0)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	service, err := NewRPService(s, provider, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := service.WaitRP(ctx, r); err != nil || !retry.Replayed || calls != 0 {
		t.Fatalf("warm retry/model: %+v %v calls=%d", retry, err, calls)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE event_id=?`, []any{fact.EventID}, 1)
	for _, at := range []string{careerTime(1, 7, 45), careerTime(1, 8, 0)} {
		view, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		out, err := service.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: at})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.WarmNPCIDs) != 0 {
			t.Fatalf("warm cooldown/hot promotion failed: %+v", out)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWarmDecisionRecorded'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_work_ada' AND activity_code='work'`, []any{M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_work_bo' AND activity_code='work'`, []any{M2AgentBoID}, 1)
	if calls == 0 {
		t.Fatal("actual scene arrival did not promote HOT processing")
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("warm movement recovery: %+v %v", differences, err)
	}
}
