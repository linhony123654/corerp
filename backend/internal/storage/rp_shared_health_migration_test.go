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

func TestRPSharedHealthMigrationPreservesF3RoundRowsAndForeignKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "f3-to-f5.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	defer func() { s.Close() }()
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if filepath.Base(file) >= "046_" {
			continue
		}
		if err := s.applyMigration(ctx, filepath.Base(file)); err != nil {
			t.Fatal("create pre-F5 schema", file, err)
		}
	}
	_, human, _ := newRPWaitTestSession(t, ctx, s)
	for _, principal := range []struct{ id, kind string }{{"principal_migration_operator", "operator"}, {"principal_migration_service", "service"}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, principal.id, principal.kind, principal.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_migration_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("migration-enroll"),
		EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_migration_service", ControllerInstanceID: "migration-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("migration-assign"),
		EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	externalSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_migration_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID,
		POV: "second_person", IdempotencyKey: "migration-session"})
	if err != nil {
		t.Fatal(err)
	}
	external := core.RPSessionReadRequest{PrincipalID: "principal_migration_service", SessionID: externalSession.SessionID}
	open := func(key string) RPSharedRound {
		for _, read := range []core.RPSessionReadRequest{human, external} {
			if _, err := s.ObserveRPSession(ctx, read); err != nil {
				t.Fatal(err)
			}
		}
		round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding(key), HumanSessionID: human.SessionID,
			ExternalSessionIDs: []string{external.SessionID}})
		if err != nil {
			t.Fatal(err)
		}
		return round
	}
	speech := open("migration-speech-round")
	if _, err := s.SubmitRPSharedSpeech(ctx, RPSharedSpeechRequest{PrincipalID: external.PrincipalID, SessionID: external.SessionID,
		RoundID: speech.RoundID, Text: "升级前的真实共享发言。", IdempotencyKey: "migration-speech"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID,
		RoundID: speech.RoundID, HorizonWorldTime: "2026-09-22T03:00:00Z", IdempotencyKey: "migration-human-wait"}); err != nil {
		t.Fatal(err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: external.PrincipalID,
		SessionID: external.SessionID, RoundID: speech.RoundID}, Budget: 1000}
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.OwnDisposition != "action_accepted" {
		t.Fatal("pre-F5 speech receipt", settled, err)
	}
	move := open("migration-move-round")
	if _, err := s.SubmitRPSharedMove(ctx, RPSharedMoveRequest{PrincipalID: external.PrincipalID, SessionID: external.SessionID,
		RoundID: move.RoundID, FromPlaceID: "place_m2_home_ada", ToPlaceID: M2AgentCafeID,
		IdempotencyKey: "migration-move"}); err != nil {
		t.Fatal("pre-F5 move proposal", err)
	}
	if err := s.applyMigration(ctx, "046_shared_health_rounds.sql"); err != nil {
		t.Fatal("upgrade populated F3 rounds", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil || receipt.EventSequence != settled.EventSequence || receipt.OwnDisposition != "action_accepted" {
		t.Fatal("speech receipt changed across migration", receipt, err)
	}
	if pending, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: external.PrincipalID,
		SessionID: external.SessionID, RoundID: move.RoundID}); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("pending move proposal changed across migration", pending, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_shared_round_actions WHERE round_id=? AND action_kind='move' AND submission_key='migration-move'`, []any{move.RoundID}, 1)
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		rows.Close()
		t.Fatal("migration left a foreign-key violation")
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
		t.Fatal("reopen upgraded F3 world", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil || receipt.EventSequence != settled.EventSequence {
		t.Fatal("reopened speech receipt", receipt, err)
	}
}
