package core

import "testing"

func TestMaterializeCohortValidationAndCanonicalHash(t *testing.T) {
	command := MaterializeCohortCommand{
		CommandID: "cmd_materialize", MaterializationID: "mat_1", InstanceID: "inst",
		BranchID: "branch", PrincipalID: "creator", CapabilityID: "world.cohort.materialize",
		IdempotencyKey: "idem", ExpectedHead: 1, WorldTime: "2026-09-22T01:00:00Z",
		SourceCohortID: "cohort", EntityID: "entity", DisplayName: "Named Person",
		PopulationCount: 1, AssetMinor: 100, InventoryMinor: 10, ReceivableMinor: 20,
		LiabilityMinor: 15, AllocationAlgorithmVersion: "equal-share-v1",
	}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	first, err := MaterializeCohortRequestHash(command)
	if err != nil {
		t.Fatal(err)
	}
	retry := command
	retry.CommandID = "cmd_retry"
	retry.IdempotencyKey = "idem_retry"
	second, err := MaterializeCohortRequestHash(retry)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("transport retry identity changed canonical request hash: %s != %s", first, second)
	}
	invalid := command
	invalid.PopulationCount = 0
	if err := invalid.Validate(); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("zero population should fail validation, got %v", err)
	}
	invalid = command
	invalid.LiabilityMinor = -1
	if err := invalid.Validate(); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("negative allocation should fail validation, got %v", err)
	}
}

func TestDematerializeCohortValidationAndCanonicalHash(t *testing.T) {
	command := DematerializeCohortCommand{
		CommandID: "cmd_dematerialize", MaterializationID: "mat_1", InstanceID: "inst",
		BranchID: "branch", PrincipalID: "creator", CapabilityID: "world.cohort.dematerialize",
		IdempotencyKey: "idem", ExpectedHead: 2, WorldTime: "2026-09-22T02:00:00Z",
		ReasonCode: "lod_return",
	}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	first, err := DematerializeCohortRequestHash(command)
	if err != nil {
		t.Fatal(err)
	}
	retry := command
	retry.CommandID = "cmd_retry"
	retry.IdempotencyKey = "idem_retry"
	second, err := DematerializeCohortRequestHash(retry)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("dematerialization retry identity changed canonical request hash: %s != %s", first, second)
	}
	command.ReasonCode = ""
	if err := command.Validate(); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("missing reason should fail validation, got %v", err)
	}
}
