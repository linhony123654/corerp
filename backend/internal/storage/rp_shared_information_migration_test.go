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

func TestRPSharedInformationMigrationPreservesPopulatedRounds(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "f7-shared-upgrade.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	defer s.Close()
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if filepath.Base(file) >= "048_" {
			continue
		}
		if err := s.applyMigration(ctx, filepath.Base(file)); err != nil {
			t.Fatal("create pre-information-round schema", file, err)
		}
	}
	humanSession, human, _ := newRPWaitTestSession(t, ctx, s)
	for _, p := range []struct{ id, kind string }{{"principal_info_migrate_operator", "operator"}, {"principal_info_migrate_service", "service"}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, p.id, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_info_migrate_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("info-migrate-enroll"),
		EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_info_migrate_service", ControllerInstanceID: "info-migrate-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("info-migrate-assign"),
		EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	externalSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_info_migrate_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID,
		POV: "second_person", IdempotencyKey: "info-migrate-session"})
	if err != nil {
		t.Fatal(err)
	}
	external := core.RPSessionReadRequest{PrincipalID: "principal_info_migrate_service", SessionID: externalSession.SessionID}
	open := func(key string) RPSharedRound {
		for _, read := range []core.RPSessionReadRequest{human, external} {
			if _, err := s.ObserveRPSession(ctx, read); err != nil {
				t.Fatal(err)
			}
		}
		round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding(key),
			HumanSessionID: humanSession.SessionID, ExternalSessionIDs: []string{externalSession.SessionID}})
		if err != nil {
			t.Fatal(err)
		}
		return round
	}
	speech := open("info-migrate-speech-round")
	if _, err := s.SubmitRPSharedSpeech(ctx, RPSharedSpeechRequest{PrincipalID: external.PrincipalID,
		SessionID: external.SessionID, RoundID: speech.RoundID, Text: "升级前的共享发言。",
		IdempotencyKey: "info-migrate-speech"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: human.PrincipalID,
		SessionID: human.SessionID, RoundID: speech.RoundID, HorizonWorldTime: "2026-09-22T03:00:00Z",
		IdempotencyKey: "info-migrate-human-wait"}); err != nil {
		t.Fatal(err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: external.PrincipalID, SessionID: external.SessionID, RoundID: speech.RoundID}, Budget: 1000}
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" {
		t.Fatal("pre-048 settled speech", settled, err)
	}
	move := open("info-migrate-pending-move")
	if _, err := s.SubmitRPSharedMove(ctx, RPSharedMoveRequest{PrincipalID: external.PrincipalID,
		SessionID: external.SessionID, RoundID: move.RoundID, FromPlaceID: "place_m2_home_ada",
		ToPlaceID: M2AgentCafeID, IdempotencyKey: "info-migrate-move"}); err != nil {
		t.Fatal("pre-048 pending move", err)
	}
	if err := s.applyMigration(ctx, "048_shared_information_rounds.sql"); err != nil {
		t.Fatal("upgrade populated 047 world", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil ||
		receipt.EventSequence != settled.EventSequence || receipt.OwnDisposition != "action_accepted" {
		t.Fatal("settled speech changed across 048", receipt, err)
	}
	moveRead := RPSharedRoundReadRequest{PrincipalID: external.PrincipalID, SessionID: external.SessionID, RoundID: move.RoundID}
	if pending, err := s.ReadRPSharedRound(ctx, moveRead); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("pending move changed across 048", pending, err)
	}
	if err := s.applyMigration(ctx, "049_shared_information_stance.sql"); err != nil {
		t.Fatal("upgrade populated 048 world", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil ||
		receipt.EventSequence != settled.EventSequence || receipt.OwnDisposition != "action_accepted" {
		t.Fatal("settled speech changed across 049", receipt, err)
	}
	if pending, err := s.ReadRPSharedRound(ctx, moveRead); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("pending move changed across 049", pending, err)
	}
	if err := s.applyMigration(ctx, "050_shared_information_relay.sql"); err != nil {
		t.Fatal("upgrade populated 049 world", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil ||
		receipt.EventSequence != settled.EventSequence || receipt.OwnDisposition != "action_accepted" {
		t.Fatal("settled speech changed across 050", receipt, err)
	}
	if pending, err := s.ReadRPSharedRound(ctx, moveRead); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("pending move changed across 050", pending, err)
	}
	if err := s.applyMigration(ctx, "051_shared_public_notice_access.sql"); err != nil {
		t.Fatal("upgrade populated 050 world", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil ||
		receipt.EventSequence != settled.EventSequence || receipt.OwnDisposition != "action_accepted" {
		t.Fatal("settled speech changed across 051", receipt, err)
	}
	if pending, err := s.ReadRPSharedRound(ctx, moveRead); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("pending move changed across 051", pending, err)
	}
	if err := s.applyMigration(ctx, "052_shared_organization_notice_access.sql"); err != nil {
		t.Fatal("upgrade populated 051 world", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil ||
		receipt.EventSequence != settled.EventSequence || receipt.OwnDisposition != "action_accepted" {
		t.Fatal("settled speech changed across 052", receipt, err)
	}
	if pending, err := s.ReadRPSharedRound(ctx, moveRead); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("pending move changed across 052", pending, err)
	}
	if err := s.applyMigration(ctx, "053_shared_public_notice_publish.sql"); err != nil {
		t.Fatal("upgrade populated 052 world", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil ||
		receipt.EventSequence != settled.EventSequence || receipt.OwnDisposition != "action_accepted" {
		t.Fatal("settled speech changed across 053", receipt, err)
	}
	if pending, err := s.ReadRPSharedRound(ctx, moveRead); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("pending move changed across 053", pending, err)
	}
	if err := s.applyMigration(ctx, "054_shared_organization_notice_publish.sql"); err != nil {
		t.Fatal("upgrade populated 053 world", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil ||
		receipt.EventSequence != settled.EventSequence || receipt.OwnDisposition != "action_accepted" {
		t.Fatal("settled speech changed across 054", receipt, err)
	}
	if pending, err := s.ReadRPSharedRound(ctx, moveRead); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("pending move changed across 054", pending, err)
	}
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		rows.Close()
		t.Fatal("048–054 migrations left foreign-key violation")
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
		t.Fatal(err)
	}
	defer s.Close()
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPSharedInformationSchemaVersion}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPSharedStanceSchemaVersion}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPSharedRelaySchemaVersion}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPSharedPublicAccessSchemaVersion}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPSharedOrganizationAccessSchemaVersion}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPSharedPublicPublishSchemaVersion}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPSharedOrganizationPublishSchemaVersion}, 1)
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil || receipt.EventSequence != settled.EventSequence {
		t.Fatal("reopened speech receipt after 048", receipt, err)
	}
	if pending, err := s.ReadRPSharedRound(ctx, moveRead); err != nil || pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("reopened pending move after 048", pending, err)
	}
}
