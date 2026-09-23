package core

import (
	"strings"
	"time"
)

const (
	MaterializeCohortCommandType   = "MaterializeCohortCommand"
	DematerializeCohortCommandType = "DematerializeCohortCommand"
)

type MaterializeCohortCommand struct {
	CommandID                  string `json:"command_id"`
	MaterializationID          string `json:"materialization_id"`
	InstanceID                 string `json:"instance_id"`
	BranchID                   string `json:"branch_id"`
	PrincipalID                string `json:"principal_id"`
	CapabilityID               string `json:"capability_id"`
	IdempotencyKey             string `json:"idempotency_key"`
	ExpectedHead               int64  `json:"expected_head"`
	WorldTime                  string `json:"world_time"`
	SourceCohortID             string `json:"source_cohort_id"`
	EntityID                   string `json:"entity_id"`
	DisplayName                string `json:"display_name"`
	PopulationCount            int64  `json:"population_count"`
	AssetMinor                 int64  `json:"asset_minor"`
	InventoryMinor             int64  `json:"inventory_minor"`
	ReceivableMinor            int64  `json:"receivable_minor"`
	LiabilityMinor             int64  `json:"liability_minor"`
	AllocationAlgorithmVersion string `json:"allocation_algorithm_version"`
}

type DematerializeCohortCommand struct {
	CommandID         string `json:"command_id"`
	MaterializationID string `json:"materialization_id"`
	InstanceID        string `json:"instance_id"`
	BranchID          string `json:"branch_id"`
	PrincipalID       string `json:"principal_id"`
	CapabilityID      string `json:"capability_id"`
	IdempotencyKey    string `json:"idempotency_key"`
	ExpectedHead      int64  `json:"expected_head"`
	WorldTime         string `json:"world_time"`
	ReasonCode        string `json:"reason_code"`
}

type CohortTransitionResult struct {
	CommandID         string `json:"command_id"`
	MaterializationID string `json:"materialization_id"`
	EntityID          string `json:"entity_id"`
	BatchID           string `json:"batch_id"`
	EventID           string `json:"event_id"`
	FirstSequence     int64  `json:"first_sequence"`
	LastSequence      int64  `json:"last_sequence"`
	EventCount        int64  `json:"event_count"`
	RequestHash       string `json:"request_hash"`
	BatchHash         string `json:"batch_hash"`
	Replayed          bool   `json:"replayed"`
}

func (c MaterializeCohortCommand) Validate() error {
	required := map[string]string{
		"command_id": c.CommandID, "materialization_id": c.MaterializationID,
		"instance_id": c.InstanceID, "branch_id": c.BranchID, "principal_id": c.PrincipalID,
		"capability_id": c.CapabilityID, "idempotency_key": c.IdempotencyKey,
		"world_time": c.WorldTime, "source_cohort_id": c.SourceCohortID,
		"entity_id": c.EntityID, "display_name": c.DisplayName,
		"allocation_algorithm_version": c.AllocationAlgorithmVersion,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	if c.ExpectedHead < 0 || c.ExpectedHead >= MaxJSONSafeInteger {
		return NewError(CodeInvalidArgument, "expected_head is outside the supported range")
	}
	if c.PopulationCount <= 0 {
		return NewError(CodeInvalidArgument, "population_count must be positive")
	}
	for name, value := range map[string]int64{
		"population_count": c.PopulationCount, "asset_minor": c.AssetMinor,
		"inventory_minor": c.InventoryMinor, "receivable_minor": c.ReceivableMinor,
		"liability_minor": c.LiabilityMinor,
	} {
		if value < 0 {
			return NewError(CodeInvalidArgument, name+" must be nonnegative")
		}
		if value > MaxJSONSafeInteger {
			return NewError(CodeIntegerOverflow, name+" exceeds the interoperable JSON integer range")
		}
	}
	if _, err := time.Parse(time.RFC3339, c.WorldTime); err != nil {
		return WrapError(CodeInvalidArgument, "world_time must be RFC 3339", err)
	}
	return nil
}

func MaterializeCohortRequestHash(c MaterializeCohortCommand) (string, error) {
	return HashJSON(struct {
		CommandType                string `json:"command_type"`
		MaterializationID          string `json:"materialization_id"`
		InstanceID                 string `json:"instance_id"`
		BranchID                   string `json:"branch_id"`
		PrincipalID                string `json:"principal_id"`
		CapabilityID               string `json:"capability_id"`
		ExpectedHead               int64  `json:"expected_head"`
		WorldTime                  string `json:"world_time"`
		SourceCohortID             string `json:"source_cohort_id"`
		EntityID                   string `json:"entity_id"`
		DisplayName                string `json:"display_name"`
		PopulationCount            int64  `json:"population_count"`
		AssetMinor                 int64  `json:"asset_minor"`
		InventoryMinor             int64  `json:"inventory_minor"`
		ReceivableMinor            int64  `json:"receivable_minor"`
		LiabilityMinor             int64  `json:"liability_minor"`
		AllocationAlgorithmVersion string `json:"allocation_algorithm_version"`
	}{
		MaterializeCohortCommandType, c.MaterializationID, c.InstanceID, c.BranchID,
		c.PrincipalID, c.CapabilityID, c.ExpectedHead, c.WorldTime, c.SourceCohortID,
		c.EntityID, c.DisplayName, c.PopulationCount, c.AssetMinor, c.InventoryMinor,
		c.ReceivableMinor, c.LiabilityMinor, c.AllocationAlgorithmVersion,
	})
}

func (c DematerializeCohortCommand) Validate() error {
	required := map[string]string{
		"command_id": c.CommandID, "materialization_id": c.MaterializationID,
		"instance_id": c.InstanceID, "branch_id": c.BranchID, "principal_id": c.PrincipalID,
		"capability_id": c.CapabilityID, "idempotency_key": c.IdempotencyKey,
		"world_time": c.WorldTime, "reason_code": c.ReasonCode,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	if c.ExpectedHead < 0 || c.ExpectedHead >= MaxJSONSafeInteger {
		return NewError(CodeInvalidArgument, "expected_head is outside the supported range")
	}
	if _, err := time.Parse(time.RFC3339, c.WorldTime); err != nil {
		return WrapError(CodeInvalidArgument, "world_time must be RFC 3339", err)
	}
	return nil
}

func DematerializeCohortRequestHash(c DematerializeCohortCommand) (string, error) {
	return HashJSON(struct {
		CommandType       string `json:"command_type"`
		MaterializationID string `json:"materialization_id"`
		InstanceID        string `json:"instance_id"`
		BranchID          string `json:"branch_id"`
		PrincipalID       string `json:"principal_id"`
		CapabilityID      string `json:"capability_id"`
		ExpectedHead      int64  `json:"expected_head"`
		WorldTime         string `json:"world_time"`
		ReasonCode        string `json:"reason_code"`
	}{
		DematerializeCohortCommandType, c.MaterializationID, c.InstanceID, c.BranchID,
		c.PrincipalID, c.CapabilityID, c.ExpectedHead, c.WorldTime, c.ReasonCode,
	})
}
