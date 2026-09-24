package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPWalletOwnBalanceRecoveryAndAuthorization(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wallet.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	before, err := s.ReadRPWallet(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if before.ObservationCursor != initial.ObservationCursor || before.WorldTime != initial.WorldTime || before.CurrencyID == "" {
		t.Fatalf("wallet is not a current sourced snapshot: %+v", before)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=(SELECT asset_account_id FROM materialized_entities WHERE entity_id=?)`, []any{M2RPPlayerID}, before.BalanceMinor)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, initial.ObservationCursor)

	// A real economic command, not a balance fixture rewrite, must be reflected.
	gift := socialRequest(t, ctx, s, read, M2RPNPCID, "gift", "wallet-gift")
	gift.AmountMinor = 1
	if _, err := s.SocialRP(ctx, gift); err != nil {
		t.Fatal(err)
	}
	after, err := s.ReadRPWallet(ctx, read)
	if err != nil || after.BalanceMinor != before.BalanceMinor-1 || after.ObservationCursor <= before.ObservationCursor {
		t.Fatalf("wallet did not follow committed gift: %+v %v", after, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := s.ReadRPWallet(ctx, read)
	if err != nil || reopened != after {
		t.Fatalf("wallet changed across reopen: %+v %v", reopened, err)
	}
	foreign := read
	foreign.PrincipalID = "principal_creator"
	if _, err := s.ReadRPWallet(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("foreign principal accessed wallet: %v", err)
	}
	if _, err := s.ReadRPWallet(ctx, core.RPSessionReadRequest{}); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("empty binding accepted: %v", err)
	}
	// Revocation must take effect even for an already-open session.
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND capability_id='world.rp.control'`, read.PrincipalID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPWallet(ctx, read); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked controller accessed wallet: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, after.ObservationCursor)
}
