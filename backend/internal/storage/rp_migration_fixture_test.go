package storage

import (
	"context"
	"testing"
)

// Simulate a genuinely empty pre-050 test database before replaying old
// migrations. Fixtures with committed object or nonverbal facts must never call
// this; populated-object upgrade coverage is separate.
func stripEmptyRPTypedMigrationsForTest(t *testing.T, ctx context.Context, s *Store) {
	t.Helper()
	for _, table := range []string{
		"rp_interaction_pending_actions", "rp_typed_action_retirements",
		"rp_object_offers", "rp_object_escrows", "rp_objects", "rp_object_sources", "rp_object_anchors",
	} {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("cannot downgrade populated %s fixture (%d rows): %v", table, count, err)
		}
		if _, err := s.db.ExecContext(ctx, "DROP TABLE "+table); err != nil {
			t.Fatalf("strip empty %s fixture: %v", table, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version IN (?,?,?)`, RPObjectSchemaVersion, RPTypedActionChildSchemaVersion, RPObjectStowSchemaVersion); err != nil {
		t.Fatal(err)
	}
}
