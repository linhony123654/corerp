package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func prepareRPStorefrontWorld(t *testing.T, ctx context.Context, s *Store) {
	t.Helper()
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	setup, err := s.PrepareM2EconomicDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Keep household liquidity in the cohort so this fixture reaches the finite
	// stock boundary instead of the separate insufficient-funds boundary. Actual
	// materialization commands conserve assets; no balances are overwritten.
	cai, err := s.MaterializeCohort(ctx, m2AgentMaterialization("rp_cai", M2RPNPCID, "Cai", 1, 0, 0, 0, 60, setup.EventSequence, "2026-09-22T07:01:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MaterializeCohort(ctx, m2AgentMaterialization("rp_lin", M2RPPlayerID, "Lin", 1, 0, 0, 0, 45, cai.LastSequence, "2026-09-22T07:02:00Z")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.setupRPParticipantsAt(ctx, rpLifeSetupTime); err != nil {
		t.Fatal(err)
	}
	if _, err := s.setupRPTravelAt(ctx, rpLifeSetupTime); err != nil {
		t.Fatal(err)
	}
}

func TestRPStorefrontFiniteShortageRestockAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "storefront.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	prepareRPStorefrontWorld(t, ctx, s)
	r := StorefrontSourceRequest{Binding: careerTestBinding(t, s, "principal_creator", "storefront"), Source: RPStorefrontSource{PlaceID: M2AgentCafeID, StoreActorID: m2StoreActorID, SKUID: M2DemoSKUID}}
	bad := r
	bad.Source.NoticeBasisPoints = 5001
	if _, err := s.DefineRPStorefrontSource(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("unbounded shortage notice: %v", err)
	}
	bad.Source.NoticeBasisPoints = 1500
	if _, err := s.DefineRPStorefrontSource(ctx, bad); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("notice without stream: %v", err)
	}
	bad = r
	bad.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.DefineRPStorefrontSource(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("noncreator: %v", err)
	}
	for _, field := range []string{"place", "store", "sku"} {
		bad = r
		switch field {
		case "place":
			bad.Source.PlaceID = "missing"
		case "store":
			bad.Source.StoreActorID = m2SupplierActorID
		case "sku":
			bad.Source.SKUID = "missing"
		}
		if _, err := s.DefineRPStorefrontSource(ctx, bad); !core.HasCode(err, core.CodeNotFound) {
			t.Fatalf("invalid %s: %v", field, err)
		}
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "storefront rollback") }
	if _, err := s.DefineRPStorefrontSource(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPStorefrontSourceDefined'`, nil, 0)
	created, err := s.DefineRPStorefrontSource(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if created.Fact.StoreSourceEventID != m2EconomyEventID || created.Fact.PlaceSourceEventID == "" || created.Fact.BuilderSourceEventID == "" {
		t.Fatal("missing storefront sources")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{created.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=?`, []any{created.EventID}, 0)
	bad = r
	bad.Source.PlaceID = "place_m2_home_bo"
	if _, err := s.DefineRPStorefrontSource(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed retry: %v", err)
	}
	bad.Binding = careerTestBinding(t, s, "principal_creator", "second-outlet")
	if _, err := s.DefineRPStorefrontSource(ctx, bad); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("duplicate shelf at another place: %v", err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "store-observer"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	observe := func(available bool) core.RPStoreAvailability {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil || len(view.Stores) != 1 {
			t.Fatalf("store observation: %+v %v", view.Stores, err)
		}
		store := view.Stores[0]
		if store.Available != available || store.StorefrontSourceEventID != created.EventID || store.StockSourceEventID == "" {
			t.Fatalf("availability: %+v", store)
		}
		raw, err := json.Marshal(store)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{"quantity", "balance", "stock_location", m2StoreLocation, m2StoreCash} {
			if strings.Contains(string(raw), private) {
				t.Fatalf("private store state exposed: %s", raw)
			}
		}
		return store
	}
	initial := observe(true)
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	input, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: M2RPNPCID, InterlocutorEntityID: M2RPPlayerID})
	tx.Rollback(ctx)
	if err != nil || len(input.Stores) != 1 || input.Stores[0] != initial {
		t.Fatalf("NPC shelf differs: %+v %v", input.Stores, err)
	}
	// Real conserved purchases exhaust opening stock. The actual day26 purchase
	// rejects without inventing goods, before the paid supplier restock at07:07.
	if run, err := s.RunAgentLife(ctx, m2WageTime(26, 7, 6), 2000); err != nil || run.PendingDue != 0 {
		t.Fatalf("actual stock depletion: %+v %v", run, err)
	}
	shortage := observe(false)
	if shortage.StockSourceEventID == initial.StockSourceEventID {
		t.Fatal("stock-out has no new causal stock source")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE day=26 AND reason_code='insufficient_stock'`, nil, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if restored := observe(false); restored != shortage {
		t.Fatalf("shortage recovery changed: %+v", restored)
	}
	retry, err := s.DefineRPStorefrontSource(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != created.EventID {
		t.Fatalf("source retry: %+v %v", retry, err)
	}
	// Corruption is not a source-backed shortage/recovery and must fail closed.
	if _, err := s.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor=1 WHERE location_id=? AND sku_id=?`, m2StoreLocation, M2DemoSKUID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ObserveRPSession(ctx, read); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("forged stock accepted: %v", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if run, err := s.RunAgentLife(ctx, m2WageTime(26, 7, 7), 100); err != nil || run.PendingDue != 0 {
		t.Fatalf("actual paid restock: %+v %v", run, err)
	}
	restocked := observe(true)
	if restocked.StockSourceEventID == shortage.StockSourceEventID {
		t.Fatal("restock source unchanged")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND event_type='M2StoreRestocked'`, []any{restocked.StockSourceEventID}, 1)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "leave-store"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.Stores) != 0 {
		t.Fatalf("distant shelves exposed: %+v %v", view.Stores, err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("store recovery: %+v %v", differences, err)
	}
}
