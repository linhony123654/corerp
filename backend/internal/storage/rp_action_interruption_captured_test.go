package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The operator supplies a BACKUP COPY of the captured database. Never sample
// new decisions or use a provider while upgrading/re-reading accepted receipts.
func TestRPActionInterruptionCaptured080Conservation(t *testing.T) {
	path := os.Getenv("CORERP_INTERRUPTION_CAPTURE_COPY")
	if path == "" {
		t.Skip("explicit captured database copy not supplied")
	}
	abs, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(abs, "/tmp/corerp-r1-interruption-work-20261002/") {
		t.Fatal("reader requires isolated capture copy path")
	}
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	baseline := &Store{db: db}
	cols := rp080ParentColumns(t, ctx, baseline)
	tables := append([]string{"events", "event_batches", "commands", "command_attempts", "rp_utterances", "rp_npc_decisions", "rp_provider_calls", "agent_knowledge", "observation_records"}, rp080Children...)
	before := map[string]string{}
	for _, table := range tables {
		before[table] = rp080Snapshot(t, ctx, baseline, table, "*")
	}
	beforeTurns := rp080Snapshot(t, ctx, baseline, "rp_turn_runs", cols)
	db.Close()
	saved := map[string]RPTurnResult{}
	for pass := 0; pass < 2; pass++ {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		var reads []core.RPSessionReadRequest
		rows, err := s.db.QueryContext(ctx, `SELECT principal_id,session_id FROM rp_sessions ORDER BY session_id`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var read core.RPSessionReadRequest
			if err := rows.Scan(&read.PrincipalID, &read.SessionID); err != nil {
				t.Fatal(err)
			}
			reads = append(reads, read)
		}
		rows.Close()
		var ids []string
		rows, err = s.db.QueryContext(ctx, `SELECT turn_run_id FROM rp_turn_runs ORDER BY turn_run_id`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, read := range reads {
			if _, err := s.ObserveRPSession(ctx, read); err != nil {
				t.Fatal("captured observe", err)
			}
			if _, err := s.ReadRPObservatory(ctx, RPObservatoryRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID}); err != nil {
				t.Fatal("captured observatory", err)
			}
			if _, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID}); err != nil {
				t.Fatal("captured public history", err)
			}
		}
		for _, id := range ids {
			run, err := s.loadRPTurnRun(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			out, err := s.loadRPTurnResult(ctx, run, true)
			if err != nil || out.Interruption != nil {
				t.Fatal("captured canonical receipt", id, err)
			}
			if pass == 0 {
				saved[id] = out
			} else if !reflect.DeepEqual(saved[id], out) {
				t.Fatal("restart changed captured receipt", id)
			}
		}
		for _, table := range tables {
			if got := rp080Snapshot(t, ctx, s, table, "*"); got != before[table] {
				t.Fatal("captured authority/provider receipt changed", table)
			}
		}
		if got := rp080Snapshot(t, ctx, s, "rp_turn_runs", cols); got != beforeTurns {
			t.Fatal("migration/read changed captured turn bytes")
		}
		rp080AssertFK(t, ctx, s)
		t.Logf("pass=%d sessions=%d canonical_turns=%d zero provider invocations; authority/receipt snapshots conserved", pass+1, len(reads), len(ids))
		s.Close()
	}
}
