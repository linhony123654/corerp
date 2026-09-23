package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

const rpTestPrincipal = "principal_rp_test_player"

func grantRPControlForTest(t *testing.T, ctx context.Context, store *Store, entityID string) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status) VALUES (?, 'player', 'RP Test Player', 'active')`, []any{rpTestPrincipal}},
		{`INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('world.rp.control', 'Control one existing world entity for RP', 'rp1-v1')`, nil},
		{`INSERT INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id)
		 VALUES (?, ?, 'world.rp.control', ?, ?, ?, '[]', 'active', ?)`, []any{"grant_rp_test_" + entityID, rpTestPrincipal, M2DemoInstanceID, M2DemoBranchID, entityID, m2AgentSetupEventID}},
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func rpTestOpenRequest() core.RPSessionOpenRequest {
	return core.RPSessionOpenRequest{
		PrincipalID: rpTestPrincipal, InstanceID: M2DemoInstanceID,
		BranchID: M2DemoBranchID, EntityID: M2AgentAdaID,
		POV: "second_person", IdempotencyKey: "rp-test-open-1",
	}
}

func TestRPSessionFirstSliceDerivesPresenceAndResumesAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-session.db")
	store := openM2AgentStore(t, ctx, path, false)
	grantRPControlForTest(t, ctx, store, M2AgentAdaID)
	request := rpTestOpenRequest()
	session, err := store.OpenRPSession(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if session.Replayed || session.SessionID == "" || session.ObservationCursor != 0 || session.TurnState != "idle" {
		t.Fatalf("unexpected initial session: %+v", session)
	}
	view, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if view.PlaceID != "place_m2_home_ada" || view.WorldTime != M2AgentSetupTime || len(view.PresentEntities) != 0 || view.ObservationCursor != 4 {
		t.Fatalf("initial observation leaked or omitted world state: %+v", view)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 4); err != nil {
		t.Fatal(err)
	}
	view, err = store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if view.PlaceID != M2AgentCafeID || view.WorldTime != M2AgentNoonTime || view.ObservationCursor != 8 || len(view.PresentEntities) != 1 || view.PresentEntities[0].EntityID != M2AgentBoID {
		t.Fatalf("session failed to derive new co-location from world state: %+v", view)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"asset_account", "goal_code", "knowledge", "memory", "liability", "principal_id"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("player observation leaks private/internal field %q: %s", forbidden, encoded)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	resumed, err := reopened.ResumeRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.SessionID != session.SessionID || resumed.ObservationCursor != 8 || resumed.ControlledEntityID != M2AgentAdaID {
		t.Fatalf("RP session did not persist independently of process: %+v", resumed)
	}
	view, err = reopened.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: session.SessionID})
	if err != nil || view.PlaceID != M2AgentCafeID || len(view.PresentEntities) != 1 {
		t.Fatalf("reopened observation lost world presence: %+v, %v", view, err)
	}
	replayed, err := reopened.OpenRPSession(ctx, request)
	if err != nil || !replayed.Replayed || replayed.SessionID != session.SessionID {
		t.Fatalf("open retry did not replay session: %+v, %v", replayed, err)
	}
	var count int
	if err := reopened.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_sessions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("open retry duplicated sessions: count=%d, err=%v", count, err)
	}
}

func TestRPSessionRejectsInvalidBindingAndIdempotencyMismatch(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "rp-reject.db"), false)
	defer store.Close()
	grantRPControlForTest(t, ctx, store, M2AgentAdaID)
	request := rpTestOpenRequest()
	for _, invalid := range []core.RPSessionOpenRequest{
		{PrincipalID: request.PrincipalID, InstanceID: "missing", BranchID: request.BranchID, EntityID: request.EntityID, POV: request.POV, IdempotencyKey: "missing-world"},
		{PrincipalID: request.PrincipalID, InstanceID: request.InstanceID, BranchID: "missing", EntityID: request.EntityID, POV: request.POV, IdempotencyKey: "missing-branch"},
		{PrincipalID: request.PrincipalID, InstanceID: request.InstanceID, BranchID: request.BranchID, EntityID: "missing", POV: request.POV, IdempotencyKey: "missing-entity"},
	} {
		if _, err := store.OpenRPSession(ctx, invalid); err == nil {
			t.Fatalf("invalid binding accepted: %+v", invalid)
		}
	}
	session, err := store.OpenRPSession(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := request
	mismatch.POV = "first_person"
	if _, err := store.OpenRPSession(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("same key/different payload accepted: %v", err)
	}
	mismatch.EntityID = M2AgentBoID
	if _, err := store.OpenRPSession(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("same key/different entity did not report idempotency conflict: %v", err)
	}
	if _, err := store.ReadRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "principal_creator", SessionID: session.SessionID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("other principal read session: %v", err)
	}
	if _, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "principal_creator", SessionID: session.SessionID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("other principal observed through session: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE capability_grants SET status = 'revoked' WHERE grant_id = ?`, "grant_rp_test_"+M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: session.SessionID}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked control grant still observed: %v", err)
	}
}

func TestRPSessionCloseAndPrecommitFailureLeaveNoWorldEffect(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "rp-close.db"), false)
	defer store.Close()
	grantRPControlForTest(t, ctx, store, M2AgentAdaID)
	request := rpTestOpenRequest()
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "precommit RP session failure") }
	if _, err := store.OpenRPSession(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected injected precommit failure, got %v", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_sessions`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4)
	session, err := store.OpenRPSession(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: session.SessionID}
	closed, err := store.CloseRPSession(ctx, read)
	if err != nil || closed.Status != "closed" {
		t.Fatalf("close failed: %+v, %v", closed, err)
	}
	closedAgain, err := store.CloseRPSession(ctx, read)
	if err != nil || closedAgain.Status != "closed" {
		t.Fatalf("close retry failed: %+v, %v", closedAgain, err)
	}
	if _, err := store.ResumeRPSession(ctx, read); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("closed session resumed: %v", err)
	}
	if _, err := store.ObserveRPSession(ctx, read); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("closed session observed: %v", err)
	}
	replayed, err := store.OpenRPSession(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Status != "closed" {
		t.Fatalf("closed session retry changed lifecycle: %+v, %v", replayed, err)
	}
	request.IdempotencyKey = "rp-test-open-new"
	newSession, err := store.OpenRPSession(ctx, request)
	if err != nil || newSession.SessionID == session.SessionID || newSession.Status != "active" {
		t.Fatalf("new key failed to create new session: %+v, %v", newSession, err)
	}
}

func TestRPSchemaUpgradeFrom019PreservesExistingM2World(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-upgrade.db")
	store := openM2AgentStore(t, ctx, path, false)
	removeRPStyleSchemaForUpgradeTest(t, ctx, store)
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_turn_runs`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_npc_decisions`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_utterances`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_wait_intents`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_place_links`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_sessions`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version IN (?, ?, ?, ?, ?, ?)`, RPTurnSchemaVersion, RPNPCDecisionSchemaVersion, RPSpeechSchemaVersion, RPWaitSchemaVersion, RPRouteSchemaVersion, RPSessionSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	assertM2Value(t, ctx, upgraded, `SELECT COUNT(*) FROM schema_meta WHERE schema_version = ?`, []any{SchemaVersion}, 1)
	assertM2Value(t, ctx, upgraded, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4)
	assertM2Value(t, ctx, upgraded, `SELECT COUNT(*) FROM materialized_entities WHERE status = 'active'`, nil, 2)
	if _, err := upgraded.BootstrapRPPlayDemo(ctx); err != nil {
		t.Fatalf("upgraded M2 world cannot enter RP: %v", err)
	}
}
