package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPWaitSettle049Upgrades048DisposableSchemaWithoutInventingHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade048.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, view := newRPWaitTestSession(t, ctx, s)
	start, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: start.Add(15 * time.Minute).UTC().Format(time.RFC3339), Budget: 100, IdempotencyKey: "legacy-complete-marker"})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("historical marker fixture: %+v %v", wait, err)
	}
	var intentID, finished string
	if err := s.db.QueryRowContext(ctx, `SELECT a.intent_id,a.completed_at_utc FROM rp_wait_activity_settlements a JOIN rp_wait_intents i ON i.intent_id=a.intent_id WHERE i.session_id=? AND i.idempotency_key='legacy-complete-marker' AND a.status='complete'`, session.SessionID).Scan(&intentID, &finished); err != nil {
		t.Fatal(err)
	}
	// Only the disposable fixture is reconstructed as 048; old migrations and
	// any user database remain untouched. A pre-existing complete marker has no
	// event range and cannot be retrospectively attributed to its wait.
	stripEmptyRPTypedMigrationsForTest(t, ctx, s)
	for _, statement := range []string{
		`DROP TABLE rp_wait_activity_settlements`,
		`CREATE TABLE rp_wait_activity_settlements (intent_id TEXT PRIMARY KEY REFERENCES rp_wait_intents(intent_id),status TEXT NOT NULL CHECK (status IN ('pending','complete')),completed_at_utc TEXT,CHECK ((status='pending')=(completed_at_utc IS NULL))) STRICT`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-wait-settle-lineage-069-2026-09-27'`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rp_wait_activity_settlements(intent_id,status,completed_at_utc) VALUES (?,'complete',?)`, intentID, finished); err != nil {
		t.Fatal(err)
	}
	if err := s.Ready(ctx); err == nil {
		t.Fatal("048 fixture incorrectly reports 049 readiness")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	var columns int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('rp_wait_activity_settlements') WHERE name IN ('first_sequence','last_sequence')`).Scan(&columns); err != nil || columns != 2 {
		t.Fatalf("049 lineage columns missing: %d %v", columns, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_wait_activity_settlements WHERE intent_id=? AND status='complete' AND first_sequence IS NULL AND last_sequence IS NULL`, []any{intentID}, 1)
}
