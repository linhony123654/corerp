package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestScopedAccountProjectionUsesRecordedWorld(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "account-scope.db"))
	defer s.Close()
	if err := s.BootstrapM2Demo(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, c := range []struct{ world, branch, account, currency, other string }{
		{DemoInstanceID, DemoBranchID, DemoBuyerAccountID, DemoCurrencyID, M2DemoInstanceID},
		{M2DemoInstanceID, M2DemoBranchID, M2DemoCohortAssetAccountID, M2DemoCurrencyID, DemoInstanceID},
	} {
		if err := verifyScopedAccountProjection(ctx, tx.conn, c.world, c.branch, c.account, c.currency); err != nil {
			t.Fatal("actual posted balance rejected", err)
		}
		if err := verifyScopedAccountProjection(ctx, tx.conn, c.other, c.branch, c.account, c.currency); !core.HasCode(err, core.CodeProjectionDiverged) {
			t.Fatal("other world supplied account authority", err)
		}
		if err := verifyScopedAccountProjection(ctx, tx.conn, c.world, c.branch, c.account, "wrong-currency"); !core.HasCode(err, core.CodeProjectionDiverged) {
			t.Fatal("currency mismatch accepted", err)
		}
	}
}
