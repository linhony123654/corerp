package storage

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPInformationMigrationPreservesPopulatedObservationsAndKnowledge(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "f6-to-f7.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if filepath.Base(file) >= "047_" {
			continue
		}
		if err := s.applyMigration(ctx, filepath.Base(file)); err != nil {
			t.Fatal("create pre-F7 schema", file, err)
		}
	}
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID,
		Text: "旧版本里真实听到的话。", SpeechAct: "statement", ExpectedCursor: initial.ObservationCursor,
		IdempotencyKey: "pre-f7-hearing"})
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND channel='co_location'`, []any{speech.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=?`, []any{speech.EventID}, 1)
	if err := s.applyMigration(ctx, "047_information_channels.sql"); err != nil {
		t.Fatal("upgrade populated F6 world", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND channel='co_location'`, []any{speech.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=?`, []any{speech.EventID}, 1)
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		rows.Close()
		t.Fatal("047 upgrade left a foreign-key violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal("reopen upgraded F6 world", err)
	}
	defer s.Close()
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPInformationSchemaVersion}, 1)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("047 upgrade changed sourced knowledge", diff, err)
	}
}
