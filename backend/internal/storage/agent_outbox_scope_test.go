package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestAgentOutboxUsesActualSourceWorld(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "scoped-outbox.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	if err := s.BootstrapM2Demo(ctx); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []struct{ event, world string }{{"event_world_initialized", DemoInstanceID}, {"event_m2_cohort_initialized", M2DemoInstanceID}} {
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		err = insertAgentOutbox(ctx, tx.conn, "test_outbox_"+scope.world, scope.event, "scope.test", []byte(`{"test":true}`))
		if err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := insertAgentOutbox(ctx, tx.conn, "missing-outbox", "missing-event", "scope.test", []byte(`{}`)); err == nil {
		t.Fatal("unknown event accepted")
	}
	tx.Rollback(ctx)
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, world := range []string{DemoInstanceID, M2DemoInstanceID} {
		var raw, hash string
		if err := s.db.QueryRowContext(ctx, `SELECT audience_scope,audience_scope_hash FROM outbox WHERE outbox_id=?`, "test_outbox_"+world).Scan(&raw, &hash); err != nil {
			t.Fatal(err)
		}
		var audience struct {
			InstanceID string `json:"instance_id"`
			Kind       string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(raw), &audience); err != nil {
			t.Fatal(err)
		}
		wantHash, err := core.HashJSON(audience)
		if err != nil || audience.InstanceID != world || audience.Kind != "instance" || hash != wantHash {
			t.Fatal("wrong event audience", raw, hash, err)
		}
		assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{world, "br_main"}, 1)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE outbox_id='missing-outbox'`, nil, 0)
}
