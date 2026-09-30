package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInteractionWaitOwnsOnlyItsAtomicDerivedActivityEvents(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wait-activity-interaction.db")
	s := openBootstrappedStore(t, ctx, path)
	defer s.Close()
	world := "wait-activity-interaction-world"
	read := openConcurrentActivitySession(t, ctx, s, world)
	startConcurrentRPActivities(t, ctx, s, read)
	before, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "等1小时，随后说「扫院子结束了。」", Mode: "SCENE", ExpectedCursor: before.ObservationCursor, IdempotencyKey: "wait-derived-then-speak"}
	result, err := service.RunRPInteraction(ctx, request)
	if err != nil || result.Status != "settled" || len(result.Outcomes) != 2 || result.Outcomes[0].Kind != "wait" || result.Outcomes[1].Kind != "speech" {
		t.Fatalf("owned activity completion blocked next step: %+v %v", result, err)
	}
	var first, last int64
	if err := s.db.QueryRowContext(ctx, `SELECT a.first_sequence,a.last_sequence FROM rp_wait_activity_settlements a JOIN rp_wait_intents i ON i.intent_id=a.intent_id WHERE i.session_id=? AND i.idempotency_key LIKE 'rpint_%' AND a.status='complete'`, read.SessionID).Scan(&first, &last); err != nil || last != first+1 || first <= result.Outcomes[0].EventSequence || last >= result.Outcomes[1].EventSequence {
		t.Fatalf("wait did not atomically bind both terminal events: %d..%d %v %+v", first, last, err, result.Outcomes)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type IN ('AgentActivityCompleted','AgentActivityCancelled')`, []any{world}, 2)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	service, err = NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.RunRPInteraction(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Status != "settled" || replayed.Outcomes[1].EventID != result.Outcomes[1].EventID {
		t.Fatalf("wait/speech replay duplicated effects: %+v %v", replayed, err)
	}
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("wait lineage projection drift: %v %v", diffs, err)
	}
}
