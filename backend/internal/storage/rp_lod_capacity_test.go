package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

// Exercise the actual storage/service cap, without inventing population in
// projections. Pure 1000-candidate tests cover selection, not world throughput.
func TestRPHotActualOverCapacityFairnessAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hot-capacity.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		name := fmt.Sprintf("capacity_%02d", i)
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		command := m2AgentMaterialization(name, "entity_"+name, name, 1, 10, 0, 0, 0, head, m2RPSetupTime)
		if _, err := s.MaterializeCohort(ctx, command); err != nil {
			t.Fatal(err)
		}
		background := core.RPBackgroundRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: command.EntityID, ExpectedHead: head + 1, IdempotencyKey: name, AgeMin: 25, AgeMax: 34, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: M2AgentCafeID, Schedule: []core.RPBackgroundSchedule{{WorldTime: careerTime(2, 3, 0), PlaceID: "place_m2_home_bo", ActivityCode: "home"}}}
		if _, err := s.MaterializeRPBackground(ctx, background); err != nil {
			t.Fatal(err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 1)
	assertM2Value(t, ctx, s, `SELECT SUM(population_count) FROM materialized_entities WHERE status='active'`, nil, 19)
	if _, err := s.RunAgentLife(ctx, careerTime(1, 12, 0), 1000); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "capacity-player"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	calls := 0
	served := map[string]int{}
	provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		served[in.NPCEntityID]++
		if in.Life == nil || in.PlaceID != M2AgentCafeID || in.NPCEntityID == M2RPPlayerID || in.NPCEntityID == M2DemoCohortID {
			t.Fatalf("invalid HOT context: %+v", in)
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	service, err := NewRPService(s, provider, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	var firstRequest core.RPWaitRequest
	var first RPWaitResult
	for round := 0; round < 2; round++ {
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		if len(view.PresentEntities) != 18 {
			t.Fatalf("not an over-capacity scene: %d", len(view.PresentEntities))
		}
		request := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 12+round, 15), Budget: 1000, IdempotencyKey: fmt.Sprintf("capacity-%d", round)}
		before := calls
		out, err := service.WaitRP(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.InitiativeNPCIDs) != core.RPHotInitiativeLimit || calls-before != core.RPHotInitiativeLimit || len(out.Initiatives) != core.RPHotInitiativeLimit {
			t.Fatalf("cap: calls=%d result=%+v", calls-before, out)
		}
		acceptedCalls := calls
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		if retry, err := service.WaitRP(ctx, request); err != nil || !retry.Replayed || calls != acceptedCalls || !reflect.DeepEqual(retry.InitiativeNPCIDs, out.InitiativeNPCIDs) {
			t.Fatalf("retry repeated/reselected HOT: %+v %v", retry, err)
		}
		assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, head)
		if round == 0 {
			firstRequest, first = request, out
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
	}
	if calls != 32 || len(served) != 18 {
		t.Fatalf("starved over-capacity actors: calls=%d distinct=%d", calls, len(served))
	}
	// A later round and restart must not rewrite the original pinned roster.
	if retry, err := service.WaitRP(ctx, firstRequest); err != nil || !retry.Replayed || calls != 32 || !reflect.DeepEqual(retry.InitiativeNPCIDs, first.InitiativeNPCIDs) {
		t.Fatalf("old wait changed: %+v %v", retry, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("capacity projections: %+v %v", diff, err)
	}
	t.Logf("actual conserved 20-person world: 18 scene NPCs, 16 calls/wait, all 18 served after 2 waits with restart; aggregate excluded")
}
