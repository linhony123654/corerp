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

func TestStudioHistoricalRulePackagesAuthorizationAndIsolation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "historical-rules.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "rule-create-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	create := studioCreateFixture("historical-rule-world")
	create.NarrativePackage.Content.NarrativeStyle.ProseInstructions = "creator-private-package-instructions"
	create.NarrativePackage.Manifest.ContentHash, _ = core.HashJSON(create.NarrativePackage.Content)
	ready, err := s.CreateStudioWorld(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	r := StudioEventRequest{PrincipalID: "principal_creator", InstanceID: create.InstanceID, BranchID: "br_main", EventID: ready.ReadyEventID}
	if _, err := s.ReadStudioEvent(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("creation authority became inspection authority", err)
	}
	head := ready.EventSequence
	for _, principal := range []string{"principal_creator", "principal_operator"} {
		grant, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: r.InstanceID, BranchID: r.BranchID, ExpectedHead: head, IdempotencyKey: "rule-inspection-" + principal}, TargetPrincipalID: principal, Status: "active"})
		if err != nil {
			t.Fatal(err)
		}
		head = grant.EventSequence
	}
	read := func() StudioEventEvidence {
		t.Helper()
		out, err := s.ReadStudioEvent(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := read()
	if out.Rule.PackageStatus != "verified" || out.Rule.Packages == nil || out.Rule.Packages.System.Content.SystemRules.NPCDailyActionBudget != create.SystemPackage.Content.SystemRules.NPCDailyActionBudget || out.Rule.Packages.Narrative.Content.NarrativeStyle.ProseInstructions != "creator-private-package-instructions" {
		t.Fatal("missing installed contents", out.Rule)
	}
	lock := out.Rule.Packages.Lock
	if lock.System.ContentHash != create.SystemPackage.Manifest.ContentHash || lock.Narrative.ContentHash != create.NarrativePackage.Manifest.ContentHash {
		t.Fatal("wrong pins", lock)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{r.InstanceID, r.BranchID}, head)
	// Activation itself was written under the preceding preparation epoch.
	r.EventID = lock.ActivationEventID
	old := read()
	if old.Rule.PackageStatus != "preparation" || old.Rule.Packages != nil || old.Rule.EpochID != "epoch_0" {
		t.Fatal("future rules retroactively assigned to activation", old.Rule)
	}
	r.EventID = ready.ReadyEventID
	for _, principal := range []string{M2RPPlayerPrincipal, "principal_buyer"} {
		r.PrincipalID = principal
		if _, err := s.ReadStudioEvent(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatal("non-creator saw rules", err)
		}
	}
	r.PrincipalID = "principal_operator"
	ops := read()
	raw, _ := json.Marshal(ops)
	if ops.Rule.Packages != nil || ops.Rule.PackageStatus != "redacted" || strings.Contains(string(raw), "creator-private") {
		t.Fatal("ops package leak", string(raw))
	}
	r.PrincipalID = "principal_creator"
	// A later, unavailable epoch must not change interpretation of this old Event.
	if _, err := s.db.ExecContext(ctx, `UPDATE rule_epochs SET end_sequence=20 WHERE instance_id=? AND epoch_id=?`, r.InstanceID, out.Rule.EpochID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rule_epochs(instance_id,branch_id,epoch_id,start_sequence,ruleset_hash,lock_document) VALUES (?,?,'future-test-epoch',20,?,'{}')`, r.InstanceID, r.BranchID, out.Rule.RulesetHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE branches SET head_sequence=20 WHERE instance_id=? AND branch_id=?`, r.InstanceID, r.BranchID); err != nil {
		t.Fatal(err)
	}
	if got := read(); !reflect.DeepEqual(got.Rule.Packages.Lock, lock) || got.Rule.EpochID != out.Rule.EpochID {
		t.Fatal("read substituted current epoch", got.Rule)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM rule_epochs WHERE instance_id=? AND epoch_id='future-test-epoch'`, r.InstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=?`, head, r.InstanceID, r.BranchID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rule_epochs SET end_sequence=NULL,lock_document='{}' WHERE instance_id=? AND epoch_id=?`, r.InstanceID, out.Rule.EpochID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadStudioEvent(ctx, r); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("corrupt historical lock accepted", err)
	}
	if err := s.RebuildProjections(ctx, r.InstanceID, r.BranchID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(); !reflect.DeepEqual(got.Rule.Packages.Lock, lock) {
		t.Fatal("rebuild/reopen lost historical provenance", got.Rule)
	}
}
