package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectEventProjectionRepairAndFailClosedExtras(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-rebuild.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "repair-object")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	assertDiff := func(want int) []ProjectionDifference {
		t.Helper()
		diffs, err := rpObjectProjectionDifferences(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, item.EventSequence)
		if err != nil || len(diffs) != want {
			t.Fatalf("unexpected object Event differences %+v %v (want %d)", diffs, err, want)
		}
		return diffs
	}
	assertDiff(0)
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_objects SET display_name='伪造的杯子' WHERE object_id=?`, item.ObjectID); err != nil {
		t.Fatal(err)
	}
	diffs := assertDiff(1)
	if diffs[0].Projection != "rp_object" {
		t.Fatalf("tampered object not flagged: %+v", diffs)
	}
	tampered := objectTestRequest(t, ctx, s, player, "take", "tampered-object")
	tampered.ObjectID = item.ObjectID
	if _, err := s.ObjectRP(ctx, tampered); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupt physical projection was consumed: %v", err)
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repairRPObjectProjections(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID, diffs); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertDiff(0)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rp_object_anchors(anchor_id,instance_id,branch_id,place_id,zone_key,anchor_code,display_name,definition_event_id) VALUES ('forged_object_anchor',?,?,?,?,?,?,?)`, M2DemoInstanceID, M2DemoBranchID, M2AgentCafeID, "main", "forged", "不存在的锚点", item.EventID); err != nil {
		t.Fatal(err)
	}
	diffs = assertDiff(1)
	if diffs[0].Projection != "rp_object_anchor_extra" {
		t.Fatalf("unsourced object anchor not flagged: %+v", diffs)
	}
	tx, err = beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repairRPObjectProjections(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID, diffs); !core.HasCode(err, core.CodeProjectionDiverged) {
		tx.Rollback(ctx)
		t.Fatalf("repair silently deleted unknown authored projection: %v", err)
	}
	tx.Rollback(ctx)
}

func TestRPObjectCandidatesNeverExposeOtherActorsInventory(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-candidates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	sourceID, _, _ := objectTestSource(t, ctx, s)
	list, err := s.ReadRPObjectCandidates(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range list {
		if candidate.Kind == "source" && candidate.ID == sourceID {
			found = true
		}
	}
	if !found {
		t.Fatal("player's finite source not shown")
	}
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	list, err = s.ReadRPObjectCandidates(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range list {
		if candidate.ID == sourceID || candidate.Kind == "source" || candidate.Kind == "object" || candidate.Kind == "offer" {
			t.Fatalf("another actor's stock or unoffered object leaked: %+v", candidate)
		}
	}
	if _, err := s.ReadRPObjectCandidates(ctx, core.RPSessionReadRequest{PrincipalID: "principal_creator", SessionID: player.SessionID}); !core.HasCode(err, core.CodeUnauthorized) && !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("third party read private item candidates: %v", err)
	}
}
