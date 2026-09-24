package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

// This is a real twenty-person economy, not a thousand-agent benchmark.
func TestRPLODTiersAcrossWeekAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lod-week.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "lod-policy"), Policy: RPOpportunityPolicy{StreamSeed: "lod-week", WarmEnabled: true, CooldownHours: 1, HistoryHours: 24}}); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "lod-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	calls := 0
	provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if in.Life == nil || in.NPCEntityID == M2DemoCohortID || in.NPCEntityID == M2RPPlayerID {
			t.Fatalf("invalid HOT context: %+v", in)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions n JOIN agent_positions p ON p.place_id=n.place_id WHERE n.agent_id=? AND p.agent_id=?`, []any{in.NPCEntityID, M2RPPlayerID}, 1)
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	service, err := NewRPService(s, provider, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
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
	wait := func(at string) RPWaitResult {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		r := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: at}
		before := calls
		out, err := service.WaitRP(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		if calls-before > len(out.InitiativeNPCIDs) || calls-before > core.RPHotInitiativeLimit || len(out.WarmNPCIDs) > core.RPWarmDecisionLimit {
			t.Fatalf("unbounded tier processing: calls=%d result=%+v", calls-before, out)
		}
		acceptedCalls := calls
		var events int64
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if replay, err := service.WaitRP(ctx, r); err != nil || !replay.Replayed || calls != acceptedCalls {
			t.Fatalf("retry repeated model work: %+v %v", replay, err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events`, nil, events)
		return out
	}
	for day := 1; day <= 7; day++ {
		move("place_m2_work_ada", fmt.Sprintf("work-%d", day))
		before := calls
		warm := wait(careerTime(day, 7, 15))
		if len(warm.WarmNPCIDs) != 1 || warm.WarmNPCIDs[0] != M2AgentAdaID || calls != before {
			t.Fatalf("day %d WARM should prepare without model: %+v", day, warm)
		}
		wait(careerTime(day, 8, 0))
		if calls != before+1 {
			t.Fatalf("day %d actual HOT arrival missing: calls=%d before=%d", day, calls, before)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_work_bo' AND activity_code='work'`, []any{M2AgentBoID}, 1)
		move(M2AgentCafeID, fmt.Sprintf("lunch-%d", day))
		wait(careerTime(day, 12, 15))
		assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 16)
		assertM2Value(t, ctx, s, `SELECT SUM(population_count) FROM materialized_entities WHERE status='active'`, nil, 4)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWarmDecisionRecorded'`, nil, int64(day))
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_economic_obligations WHERE kind='wage'`, nil, int64(day))
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_consumption_outcomes WHERE status='consumed'`, nil, int64(day))
		if day == 4 {
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
		if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
			t.Fatalf("day %d tier projections: %+v %v", day, differences, err)
		}
	}
	t.Logf("seven-day actual tier integration: %d HOT provider calls, 7 WARM decisions, 16 aggregate + 4 named people", calls)
}
