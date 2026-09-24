package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestStudioEventEvidenceAuthorizationAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "studio.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	r := StudioEventRequest{PrincipalID: "principal_creator", InstanceID: DemoInstanceID, BranchID: DemoBranchID, EventID: "event_world_initialized"}
	if _, err := s.ReadStudioEvent(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("ordinary creator grant escalated: %v", err)
	}
	for _, role := range []string{"creator", "operator", "buyer"} {
		capability := "world.inspector.read"
		if role == "operator" {
			capability = "diagnostics.inspector.read"
		}
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO capability_definitions VALUES (?, 'Read event and rule evidence', 'rp8-v1')`, capability); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,?,?,?,?,'["event","rule"]','active','event_world_initialized')`, "inspect-"+role, "principal_"+role, capability, DemoInstanceID, DemoBranchID, DemoBranchID); err != nil {
			t.Fatal(err)
		}
	}
	command := demoPurchase("cmd-studio-purchase", "studio-purchase")
	state, err := s.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	command.ExpectedHead = state.HeadSequence
	purchase, err := s.Purchase(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	bought := r
	bought.EventID = purchase.EventID
	actual, err := s.ReadStudioEvent(ctx, bought)
	if err != nil || actual.CommandID != purchase.CommandID || actual.BatchID != purchase.BatchID || actual.BatchHash != purchase.BatchHash || actual.EventSequence != purchase.LastSequence {
		t.Fatalf("actual committed purchase evidence differs: %+v %v", actual, err)
	}
	var purchasePayload map[string]json.RawMessage
	if err := json.Unmarshal(actual.Payload, &purchasePayload); err != nil || len(purchasePayload) == 0 {
		t.Fatal("missing committed purchase content", err)
	}
	before, err := s.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ReadStudioEvent(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.EventID != r.EventID || out.EventSequence != 1 || out.EventType != "WorldInitialized" || out.AccessLevel != "creator" || out.Redacted || len(out.Payload) == 0 || out.BatchHash == "" || out.Rule.RulesetHash == "" || out.Rule.StartSequence != 1 || out.CausationEvidence != "not_recorded" {
		t.Fatalf("wrong evidence: %+v", out)
	}
	var hash string
	if err := s.db.QueryRowContext(ctx, `SELECT ruleset_hash FROM rule_epochs WHERE instance_id=? AND branch_id=? AND epoch_id=?`, r.InstanceID, r.BranchID, out.Rule.EpochID).Scan(&hash); err != nil || hash != out.Rule.RulesetHash {
		t.Fatal("rule evidence mismatch", err)
	}
	op := r
	op.PrincipalID = "principal_operator"
	redacted, err := s.ReadStudioEvent(ctx, op)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(redacted)
	expectedRule := out.Rule
	expectedRule.PackageStatus, expectedRule.Packages = "redacted", nil
	if out.Rule.PackageStatus != "not_recorded" || !redacted.Redacted || redacted.Rule != expectedRule || redacted.EventID != out.EventID || redacted.CausationEvidence != "redacted" {
		t.Fatal("diagnostic metadata incorrect")
	}
	for _, forbidden := range []string{`"payload"`, `"actor_id"`, `"command_id"`, `"causation_event_id"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("operator leaked %s", forbidden)
		}
	}
	for _, principal := range []string{"principal_buyer", "principal_intruder"} {
		bad := r
		bad.PrincipalID = principal
		if _, err := s.ReadStudioEvent(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("principal %s: %v", principal, err)
		}
	}
	bad := r
	bad.BranchID = "br_other"
	if _, err := s.ReadStudioEvent(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("cross branch: %v", err)
	}
	bad = r
	bad.InstanceID = "unknown"
	if _, err := s.ReadStudioEvent(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("cross instance: %v", err)
	}
	bad = r
	bad.EventID = "missing"
	if _, err := s.ReadStudioEvent(ctx, bad); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("missing event: %v", err)
	}
	bad = r
	bad.EventID = strings.Repeat("a", 257)
	if _, err := s.ReadStudioEvent(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("unbounded id: %v", err)
	}
	after, err := s.ReadDemoState(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("read mutated world", err)
	}
	// These deliberate authorization fixtures have no grant Events. Rebuild
	// must remove unsourced grants; sourced rebuild/revocation is tested in
	// TestStudioAccessLocalAtomicRetryRevokeAndRepair instead.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := s.ReadStudioEvent(ctx, r)
	if err != nil || !reflect.DeepEqual(out, recovered) {
		t.Fatal("reopen/rebuild changed evidence", err)
	}
	actualRecovered, err := s.ReadStudioEvent(ctx, bought)
	if err != nil || !reflect.DeepEqual(actual, actualRecovered) {
		t.Fatal("purchase evidence changed after recovery", err)
	}
	for _, change := range []string{
		`UPDATE capability_grants SET field_scope='["event"]' WHERE grant_id='inspect-creator'`,
		`UPDATE capability_grants SET field_scope='["event","rule"]',status='revoked' WHERE grant_id='inspect-creator'`,
		`UPDATE capability_grants SET status='active',subject_id='other' WHERE grant_id='inspect-creator'`,
		`UPDATE capability_grants SET subject_id='br_main' WHERE grant_id='inspect-creator'; UPDATE principals SET status='disabled' WHERE principal_id='principal_creator'`,
	} {
		change = strings.ReplaceAll(change, "grant_id='inspect-creator'", "principal_id='principal_creator' AND capability_id='world.inspector.read'")
		if _, err := s.db.ExecContext(ctx, change); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReadStudioEvent(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("failed to reauthorize %s: %v", change, err)
		}
	}
}
