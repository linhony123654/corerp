package core

import (
	"strings"
	"time"
)

const IssueCurrencyCommandType = "IssueCurrencyCommand"

type IssueCurrencyCommand struct {
	CommandID       string `json:"command_id"`
	InstanceID      string `json:"instance_id"`
	BranchID        string `json:"branch_id"`
	PrincipalID     string `json:"principal_id"`
	CapabilityID    string `json:"capability_id"`
	PolicyID        string `json:"policy_id"`
	IdempotencyKey  string `json:"idempotency_key"`
	ExpectedHead    int64  `json:"expected_head"`
	WorldTime       string `json:"world_time"`
	TargetAccountID string `json:"target_account_id"`
	CurrencyID      string `json:"currency_id"`
	AmountMinor     int64  `json:"amount_minor"`
	ReasonCode      string `json:"reason_code"`
}

type IssueCurrencyResult struct {
	CommandID     string `json:"command_id"`
	BatchID       string `json:"batch_id"`
	EventID       string `json:"event_id"`
	FirstSequence int64  `json:"first_sequence"`
	LastSequence  int64  `json:"last_sequence"`
	EventCount    int64  `json:"event_count"`
	RequestHash   string `json:"request_hash"`
	BatchHash     string `json:"batch_hash"`
	Replayed      bool   `json:"replayed"`
}

func (c IssueCurrencyCommand) Validate() error {
	required := map[string]string{
		"command_id": c.CommandID, "instance_id": c.InstanceID, "branch_id": c.BranchID,
		"principal_id": c.PrincipalID, "capability_id": c.CapabilityID, "policy_id": c.PolicyID,
		"idempotency_key": c.IdempotencyKey, "world_time": c.WorldTime,
		"target_account_id": c.TargetAccountID, "currency_id": c.CurrencyID, "reason_code": c.ReasonCode,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	if c.ExpectedHead < 0 || c.ExpectedHead >= MaxJSONSafeInteger {
		return NewError(CodeInvalidArgument, "expected_head is outside the supported range")
	}
	if c.AmountMinor <= 0 {
		return NewError(CodeInvalidArgument, "amount_minor must be positive")
	}
	if c.AmountMinor > MaxJSONSafeInteger {
		return NewError(CodeIntegerOverflow, "amount_minor exceeds the interoperable JSON integer range")
	}
	if _, err := time.Parse(time.RFC3339, c.WorldTime); err != nil {
		return WrapError(CodeInvalidArgument, "world_time must be RFC 3339", err)
	}
	return nil
}

func IssueCurrencyRequestHash(c IssueCurrencyCommand) (string, error) {
	return HashJSON(struct {
		CommandType     string `json:"command_type"`
		InstanceID      string `json:"instance_id"`
		BranchID        string `json:"branch_id"`
		PrincipalID     string `json:"principal_id"`
		CapabilityID    string `json:"capability_id"`
		PolicyID        string `json:"policy_id"`
		ExpectedHead    int64  `json:"expected_head"`
		WorldTime       string `json:"world_time"`
		TargetAccountID string `json:"target_account_id"`
		CurrencyID      string `json:"currency_id"`
		AmountMinor     int64  `json:"amount_minor"`
		ReasonCode      string `json:"reason_code"`
	}{
		IssueCurrencyCommandType, c.InstanceID, c.BranchID, c.PrincipalID, c.CapabilityID,
		c.PolicyID, c.ExpectedHead, c.WorldTime, c.TargetAccountID, c.CurrencyID, c.AmountMinor, c.ReasonCode,
	})
}
