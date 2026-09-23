package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPServiceWaitDrainsOriginalRosterAcrossPartialRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "service-initiative.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	prepareRPLifeLongWorld(t, ctx, s)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "service"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	r := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T08:00:00Z", Budget: 100, IdempotencyKey: "service-wait"}
	wait, err := s.WaitRP(ctx, r)
	if err != nil || len(wait.InitiativeNPCIDs) != 2 {
		t.Fatalf("roster %+v %v", wait, err)
	}
	calls := map[string]int{}
	provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls[input.NPCEntityID]++
		return (core.DeterministicRPDecisionProvider{}).Propose(ctx, input)
	})
	service, err := NewRPService(s, provider, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	commits := 0
	s.beforeCommit = func() error {
		commits++
		if commits == 2 {
			return core.NewError(core.CodeInjectedFailure, "second initiative interrupted")
		}
		return nil
	}
	if _, err := service.WaitRP(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("partial interruption not exercised %v", err)
	}
	first, second := wait.InitiativeNPCIDs[0], wait.InitiativeNPCIDs[1]
	if calls[first] != 1 || calls[second] != 1 {
		t.Fatalf("unexpected calls %v", calls)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	service, err = NewRPService(s, provider, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := service.WaitRP(ctx, r)
	if err != nil || len(resumed.Initiatives) != 2 || !resumed.Initiatives[0].Replayed || resumed.Initiatives[1].Replayed {
		t.Fatalf("partial recovery %+v %v", resumed, err)
	}
	if calls[first] != 1 || calls[second] != 2 {
		t.Fatalf("committed actor called again %v", calls)
	}
	if _, err := service.WaitRP(ctx, r); err != nil {
		t.Fatal(err)
	}
	if calls[first] != 1 || calls[second] != 2 {
		t.Fatal("full retry called provider")
	}
	view, err = service.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	visible := false
	for _, turn := range view.RecentTurns {
		for _, line := range turn.NarrativeLines {
			if strings.Contains(line, "Nora说") && strings.Contains(line, "手头的开销") {
				visible = true
			}
			if strings.Contains(line, "input_hash") || strings.Contains(line, "silence") {
				t.Fatal("private/quiet diagnostic in scene history")
			}
		}
	}
	if !visible {
		t.Fatalf("initiative missing in Play history %+v", view.RecentTurns)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM commands WHERE command_type='RPNPCInitiative'`, nil, 2)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) > 0 {
		t.Fatalf("replay %v %v", diff, err)
	}
}
