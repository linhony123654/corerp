package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRPAccountEconomicSourcesUpgradePostAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "economic-sources.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	var before int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_account_economic_sources`).Scan(&before); err != nil || before == 0 {
		t.Fatalf("fixture needs posted economic sources: count=%d err=%v", before, err)
	}
	// Exercise the upgrade of a populated 054 database, not just fresh setup.
	for _, statement := range []string{
		`DROP TRIGGER rp_account_economic_sources_on_post`,
		`DROP TABLE rp_account_economic_sources`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-account-economic-sources-075-2026-09-28'`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal("upgrade populated 054", err)
	}
	if err := s.Ready(ctx); err != nil {
		t.Fatal("upgraded store is not ready", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_account_economic_sources`, nil, before)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("backfilled index differs from ledger: %+v %v", differences, err)
	}

	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	gift := socialRequest(t, ctx, s, cai, M2RPPlayerID, "gift", "source-post-upgrade")
	gift.AmountMinor = 1
	posted, err := s.SocialRP(ctx, gift)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_account_economic_sources WHERE event_id=?`, []any{posted.EventID}, 2)
	read := activeLife(t, ctx, s, player, "source-after-post")
	if !containsString(read.Life.EconomicSourceEventIDs, posted.EventID) {
		t.Fatalf("NPC life omitted posted gift source: %+v", read.Life.EconomicSourceEventIDs)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("new posting not indexed: %+v %v", differences, err)
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM rp_account_economic_sources WHERE event_id=? AND account_id=(SELECT asset_account_id FROM materialized_entities WHERE entity_id=?)`, posted.EventID, M2RPNPCID); err != nil {
		t.Fatal(err)
	}
	differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || !hasProjectionDifference(differences, "rp_account_economic_sources") {
		t.Fatalf("missing derived source was not detected: %+v %v", differences, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("rebuild did not restore source index: %+v %v", differences, err)
	}
	read = activeLife(t, ctx, s, player, "source-after-rebuild")
	if !containsString(read.Life.EconomicSourceEventIDs, posted.EventID) {
		t.Fatal("rebuilt NPC life lost posted gift source")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("reopen changed economic sources: %+v %v", differences, err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasProjectionDifference(differences []ProjectionDifference, name string) bool {
	for _, difference := range differences {
		if difference.Projection == name {
			return true
		}
	}
	return false
}
