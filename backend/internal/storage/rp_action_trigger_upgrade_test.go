package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

var rp080Children = []string{"rp_turn_styles", "rp_turn_listener_skips", "rp_turn_listener_activations", "rp_provider_calls", "rp_turn_explicit_rebases", "rp_narrative_renders", "rp_narrative_selections"}

// Create a real 079 schema rather than dropping a column from the new schema.
func rp079UpgradeFixture(t *testing.T, ctx context.Context, path string) (*Store, core.RPSessionReadRequest, RPSpeechResult) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.applyMigration(ctx, "001_init.sql"); err != nil {
		t.Fatal(err)
	}
	for _, migration := range schemaMigrations {
		if migration.filename == "080_rp_action_triggers.sql" {
			break
		}
		if err := s.applyMigration(ctx, migration.filename); err != nil {
			t.Fatal(err)
		}
	}
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "upgrade079-speech", Text: "这是升级前接受的原话。"})
	if err != nil {
		t.Fatal(err)
	}
	observed, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.NonverbalRP(ctx, core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "nod", TargetEntityID: M2RPNPCID, ExpectedCursor: observed.ObservationCursor, IdempotencyKey: "upgrade079-real-nod"}); err != nil {
		t.Fatal(err)
	}
	now := "2026-10-01T00:00:00Z"
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO rp_turn_runs(turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,status,player_turn_id,player_event_id,narrative_json,settled_sequence,created_at_utc,updated_at_utc,settled_at_utc) VALUES ('upgrade-settled',?,'upgrade-settled','upgrade079-speech','hash','{}','settled',?,?,?, ?,?,?,?)`, []any{read.SessionID, speech.TurnID, speech.EventID, `["升级前的展示。"]`, speech.EventSequence, now, now, now}},
		{`INSERT INTO rp_turn_runs(turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,status,created_at_utc,updated_at_utc) VALUES ('upgrade-open',?,'upgrade-open','upgrade-open','open-hash','{}','open',?,?)`, []any{read.SessionID, now, now}},
		{`INSERT INTO rp_turn_styles VALUES ('upgrade-settled',?, '[]')`, []any{mustRP080JSON(t, core.DefaultRPStyle())}},
		{`INSERT INTO rp_turn_listener_skips VALUES ('upgrade-settled',?,?)`, []any{M2RPNPCID, speech.EventID}},
		{`INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,activation_rank,conversation_source_event_id) VALUES ('upgrade-settled',?,'activated','conversation_continuation',0,?)`, []any{M2RPNPCID, speech.EventID}},
		{`INSERT INTO rp_provider_calls(call_id,session_id,turn_run_id,subject_id,npc_entity_id,phase,provider_kind,attempted,attempt_count,result,fallback_kind,started_at_utc,finished_at_utc) VALUES ('upgrade-call',?,'upgrade-settled',?,?,'decision','fixture',1,1,'failed','silence',?,?)`, []any{read.SessionID, M2RPNPCID, M2RPNPCID, now, now}},
		{`INSERT INTO rp_turn_explicit_rebases VALUES ('upgrade-settled',1,'old-hash','new-hash',1,2,'explicit_current_observation',?)`, []any{now}},
		{`INSERT INTO rp_narrative_renders(render_id,turn_run_id,style_json,source_event_ids_json,lines_json,provider_kind,created_at_utc) VALUES ('upgrade-render','upgrade-settled',?, ?, '["升级前选中的原文。"]','fixture',?)`, []any{mustRP080JSON(t, core.DefaultRPStyle()), mustRP080JSON(t, []string{speech.EventID}), now}},
		{`INSERT INTO rp_narrative_selections VALUES ('upgrade-settled','upgrade-render',?)`, []any{now}},
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET narrative_fallback='fixture-fallback',execution_mode='orchestrated',responder_limit=1,narrative_presentation_mode='custom',narrative_presented_at_utc=?,narrative_composition_version='corerp.fact-composition.v1',narrative_fact_groups_json=?,narrative_fact_event_ids_json=? WHERE turn_run_id='upgrade-settled'`, now, mustRP080JSON(t, [][]string{{speech.EventID}}), mustRP080JSON(t, []string{speech.EventID})); err != nil {
		t.Fatal(err)
	}
	return s, read, speech
}
func mustRP080JSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func rp080Snapshot(t *testing.T, ctx context.Context, s *Store, table, columns string) string {
	t.Helper()
	rows, err := s.db.QueryContext(ctx, "SELECT "+columns+" FROM "+table+" ORDER BY 1,2")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var all [][]any
	for rows.Next() {
		values := make([]any, len(names))
		pointers := make([]any, len(names))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		all = append(all, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return mustRP080JSON(t, all)
}
func rp080ParentColumns(t *testing.T, ctx context.Context, s *Store) string {
	t.Helper()
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM pragma_table_info('rp_turn_runs') ORDER BY cid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, `"`+name+`"`)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(columns, ",")
}
func rp080AssertFK(t *testing.T, ctx context.Context, s *Store) {
	t.Helper()
	assertM2Value(t, ctx, s, `PRAGMA foreign_keys`, nil, 1)
	rows, err := s.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration left broken foreign keys")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
func TestRPActionTriggerUpgradePreservesPopulated079AndRollsBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("rollback_%v", fail), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "populated079.db")
			s, _, _ := rp079UpgradeFixture(t, ctx, path)
			columns := rp080ParentColumns(t, ctx, s)
			parentBefore := rp080Snapshot(t, ctx, s, "rp_turn_runs", columns)
			before := map[string]string{}
			for _, table := range rp080Children {
				before[table] = rp080Snapshot(t, ctx, s, table, "*")
				if before[table] == "null" {
					t.Fatalf("missing populated child %s", table)
				}
			}
			worldBefore := rp080Snapshot(t, ctx, s, "events", "*")
			rp080AssertFK(t, ctx, s)
			if fail {
				if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER fail_080_marker BEFORE INSERT ON schema_meta WHEN NEW.schema_version='corerp-rp-action-triggers-080-2026-10-01' BEGIN SELECT RAISE(ABORT,'injected upgrade failure'); END`); err != nil {
					t.Fatal(err)
				}
				if err := s.applyMigration(ctx, "080_rp_action_triggers.sql"); err == nil {
					t.Fatal("injected migration succeeded")
				}
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM pragma_table_info('rp_turn_runs') WHERE name='trigger_kind'`, nil, 0)
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPActionTriggerSchemaVersion}, 0)
				if got := rp080Snapshot(t, ctx, s, "rp_turn_runs", columns); got != parentBefore {
					t.Fatal("failed upgrade changed parent")
				}
				for _, table := range rp080Children {
					if got := rp080Snapshot(t, ctx, s, table, "*"); got != before[table] {
						t.Fatalf("failed upgrade changed %s", table)
					}
				}
				rp080AssertFK(t, ctx, s)
				if _, err := s.db.ExecContext(ctx, `DROP TRIGGER fail_080_marker`); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			upgraded, err := Open(ctx, path)
			if err != nil {
				t.Fatal("populated FK-ON upgrade failed", err)
			}
			defer upgraded.Close()
			if got := rp080Snapshot(t, ctx, upgraded, "rp_turn_runs", columns); got != parentBefore {
				t.Fatal("upgrade changed legacy parent columns")
			}
			for _, table := range rp080Children {
				if got := rp080Snapshot(t, ctx, upgraded, table, "*"); got != before[table] {
					t.Fatalf("upgrade changed %s", table)
				}
			}
			if got := rp080Snapshot(t, ctx, upgraded, "events", "*"); got != worldBefore {
				t.Fatal("migration changed world events")
			}
			rp080AssertFK(t, ctx, upgraded)
			assertM2Value(t, ctx, upgraded, `PRAGMA defer_foreign_keys`, nil, 0)
			assertM2Value(t, ctx, upgraded, `SELECT COUNT(*) FROM rp_turn_runs WHERE trigger_kind='speech'`, nil, 2)
			assertM2Value(t, ctx, upgraded, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPActionTriggerSchemaVersion}, 1)
			for _, query := range []string{
				`DELETE FROM rp_turn_styles WHERE turn_run_id='upgrade-settled'`,
				`DELETE FROM rp_provider_calls WHERE call_id='upgrade-call'`,
				`DELETE FROM rp_turn_explicit_rebases WHERE turn_run_id='upgrade-settled'`,
				`DELETE FROM rp_narrative_renders WHERE render_id='upgrade-render'`,
				`UPDATE rp_narrative_selections SET render_id='missing-render' WHERE turn_run_id='upgrade-settled'`,
				`INSERT INTO rp_turn_listener_skips VALUES ('missing-turn','x','missing-event')`,
			} {
				if _, err := upgraded.db.ExecContext(ctx, query); err == nil {
					t.Fatalf("upgrade lost authority/immutability check: %s", query)
				}
			}
			selectedBefore := rp080Snapshot(t, ctx, upgraded, "rp_narrative_selections", "*")
			if err := upgraded.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if !reflect.DeepEqual(rp080Snapshot(t, ctx, reopened, "rp_narrative_selections", "*"), selectedBefore) {
				t.Fatal("repeated Open changed selection")
			}
			rp080AssertFK(t, ctx, reopened)
		})
	}
}

func TestRPActionTriggerUpgradeTypedParentConstraints(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "typedparent.db")
	s, _, speech := rp079UpgradeFixture(t, ctx, path)
	if err := s.applyMigration(ctx, "080_rp_action_triggers.sql"); err != nil {
		t.Fatal(err)
	}
	// Separate session avoids the existing interrupted-turn uniqueness fence.
	var session string
	if err := s.db.QueryRowContext(ctx, `SELECT session_id FROM rp_turn_runs WHERE turn_run_id='upgrade-settled'`).Scan(&session); err != nil {
		t.Fatal(err)
	}
	var actionEvent string
	if err := s.db.QueryRowContext(ctx, `SELECT event_id FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, M2RPPlayerID).Scan(&actionEvent); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, kind, status string
		turn, event        any
		valid              bool
	}{
		{"speech_missing_utterance", "speech", "player_committed", nil, speech.EventID, false},
		{"speech_missing_event", "speech", "player_committed", speech.TurnID, nil, false},
		{"speech_orphan_utterance", "speech", "player_committed", "missing-utterance", speech.EventID, false},
		{"nonverbal_with_utterance", "nonverbal", "settled", speech.TurnID, speech.EventID, false},
		{"nonverbal_without_event", "nonverbal", "settled", nil, nil, false},
		{"nonverbal_orphan_event", "nonverbal", "settled", nil, "missing-event", false},
		{"unknown_trigger", "invented", "settled", nil, speech.EventID, false},
		{"nonverbal_real_event", "nonverbal", "settled", nil, actionEvent, true},
	} {
		// Event kind/actor/session remain owner checks; FK/CHECK only bind shape.
		_, err := s.db.ExecContext(ctx, `INSERT INTO rp_turn_runs(turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,status,player_turn_id,player_event_id,created_at_utc,updated_at_utc,settled_at_utc,settled_sequence,trigger_kind) VALUES (?,?,?,?,'hash','{}',?,?,?,'now','now','now',1,?)`, tc.name, session, tc.name, tc.name, tc.status, tc.turn, tc.event, tc.kind)
		if (err == nil) != tc.valid {
			t.Fatalf("%s valid=%v err=%v", tc.name, tc.valid, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,activation_rank) VALUES ('nonverbal_real_event',?,'activated','direct_action',0)`, M2RPNPCID); err != nil {
		t.Fatal("direct_action rejected", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rp_turn_runs(turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,status,created_at_utc,updated_at_utc,trigger_kind) VALUES ('another-open',?,'another-open','another-open','hash','{}','open','now','now','nonverbal')`, session); err == nil {
		t.Fatal("lost one-unsettled-turn uniqueness fence")
	}
	rp080AssertFK(t, ctx, s)
}
