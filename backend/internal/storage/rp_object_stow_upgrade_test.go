package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectStowMigrationPreservesPopulatedOffersAndEscrows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "object-stow-upgrade.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// Build a populated 051 database using the original 050 table definition,
	// rather than only removing the 052 marker from a table already rebuilt by 052.
	original, err := migrationFiles.ReadFile("migrations/070_rp_object_interactions.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(original), "CREATE TABLE rp_objects (")
	end := strings.Index(string(original), "CREATE INDEX ix_rp_objects_local")
	if start < 0 || end <= start {
		t.Fatal("050 object table definition not found")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, statement := range []string{
		"DROP TABLE rp_objects",
		string(original[start:end]),
		"CREATE INDEX ix_rp_objects_local ON rp_objects(instance_id,branch_id,place_id,zone_key)",
		"CREATE INDEX ix_rp_objects_owner ON rp_objects(instance_id,branch_id,owner_actor_id)",
		"DELETE FROM schema_meta WHERE schema_version='" + RPObjectStowSchemaVersion + "'",
	} {
		if _, err := tx.conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("prepare original 051 schema: %v", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var tableSQL string
	if err := s.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name='rp_objects'`).Scan(&tableSQL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tableSQL, "physical_state IN ('held','placed')") || strings.Contains(tableSQL, "'stowed'") {
		t.Fatal("upgrade fixture did not restore the original pre-052 state constraint")
	}
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "upgrade-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "upgrade-offer")
	offer.ObjectID, offer.TargetEntityID = item.ObjectID, M2RPNPCID
	proposed, err := s.ObjectRP(ctx, offer)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Ready(ctx); err == nil {
		t.Fatal("pre-052 fixture incorrectly reported ready")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("populated object child tables were lost during migration: %v", err)
	}
	defer s.Close()
	if err := s.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND physical_state='held'`, []any{item.ObjectID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_escrows WHERE object_id=?`, []any{item.ObjectID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE offer_id=? AND status='offered'`, []any{proposed.OfferID}, 1)
	var violations int
	rows, err := s.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		violations++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if violations != 0 {
		t.Fatalf("migration broke %d foreign keys", violations)
	}
	stow := objectTestRequest(t, ctx, s, player, "stow", "upgrade-stow-locked")
	stow.ObjectID = item.ObjectID
	if _, err := s.ObjectRP(ctx, stow); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("migration forgot active offer lock: %v", err)
	}
	cancel := objectTestRequest(t, ctx, s, player, "cancel_offer", "upgrade-cancel")
	cancel.OfferID = proposed.OfferID
	if _, err := s.ObjectRP(ctx, cancel); err != nil {
		t.Fatal(err)
	}
	stow = objectTestRequest(t, ctx, s, player, "stow", "upgrade-stow")
	stow.ObjectID = item.ObjectID
	if _, err := s.ObjectRP(ctx, stow); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("migration and stow replay mismatch: %+v %v", differences, err)
	}
}
