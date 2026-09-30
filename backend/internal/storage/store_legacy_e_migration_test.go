package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestLegacyEStageMigrationAliasesUpgradeWithoutReapplyingSchemas(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-e.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{
		PrincipalID: read.PrincipalID, SessionID: session.SessionID,
		ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "legacy-e-speech", Text: "记住这句话。",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE event_id=?`, []any{speech.EventID}, 1)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM rp_own_actions WHERE event_id=?`, speech.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version IN (?,?,?)`, StudioLifeSeedingSchemaVersion, RPActivityContinuitySchemaVersion, RPNarrativeFallbackSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_meta(schema_version,applied_at_utc) VALUES (?,?),(?,?),(?,?)`,
		legacyStudioLifeSeedingSchemaVersion, "2026-09-26T00:00:00Z",
		legacyRPActivityContinuitySchemaVersion, "2026-09-26T00:00:00Z",
		legacyRPNarrativeFallbackSchemaVersion, "2026-09-26T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade legacy E migration aliases: %v", err)
	}
	defer s.Close()
	for _, version := range []string{StudioLifeSeedingSchemaVersion, RPActivityContinuitySchemaVersion, RPNarrativeFallbackSchemaVersion, SchemaVersion} {
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{version}, 1)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE event_id=?`, []any{speech.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM pragma_table_info('agent_profiles') WHERE name='persona_text'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM pragma_table_info('rp_turn_runs') WHERE name='narrative_fallback'`, nil, 1)
}
