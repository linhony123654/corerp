package storage

import (
	"context"
	"database/sql"
	"math"
	"strings"

	"corerp.local/backend/internal/core"
)

type M2WageClaimBalance struct {
	SlotIndex    int64  `json:"slot_index"`
	OriginKind   string `json:"origin_kind"`
	OriginID     string `json:"origin_id"`
	ClaimantKind string `json:"claimant_kind"`
	ClaimantID   string `json:"claimant_id"`
	Due          int64  `json:"due_minor"`
	Paid         int64  `json:"paid_minor"`
	Outstanding  int64  `json:"outstanding_minor"`
}

type M2WageClaimStatus struct {
	ObligationID string               `json:"obligation_id"`
	Status       string               `json:"status"`
	Due          int64                `json:"due_minor"`
	Paid         int64                `json:"paid_minor"`
	Outstanding  int64                `json:"outstanding_minor"`
	Current      []M2WageClaimBalance `json:"current_claimants"`
	Opening      []M2WageClaimBalance `json:"bankruptcy_opening_claimants,omitempty"`
}

// A creator-scoped read of fixture-local named wage debt. Opening claimants
// are a historical snapshot; current claimants can change only by T09 events.
func (s *Store) ReadM2WageClaimStatus(ctx context.Context, request core.StateReadRequest, obligationID string) (M2WageClaimStatus, error) {
	if err := request.Validate(); err != nil {
		return M2WageClaimStatus{}, err
	}
	if strings.TrimSpace(obligationID) == "" {
		return M2WageClaimStatus{}, core.NewError(core.CodeInvalidArgument, "obligation_id is required")
	}
	if request.CapabilityID != "world.cohort.materialize" {
		return M2WageClaimStatus{}, core.NewError(core.CodeUnauthorized, "wage claim read requires Cohort creator scope")
	}
	if err := s.authorizeExactScope(ctx, request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID, M2DemoCohortID); err != nil {
		return M2WageClaimStatus{}, err
	}
	if request.InstanceID != M2DemoInstanceID || request.BranchID != M2DemoBranchID {
		return M2WageClaimStatus{}, core.NewError(core.CodeNotFound, "M2 wage claim branch not found")
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return M2WageClaimStatus{}, core.WrapError(core.CodeStorageFailure, "open wage claim read", err)
	}
	defer conn.Close()
	var result M2WageClaimStatus
	result.ObligationID = obligationID
	if err := conn.QueryRowContext(ctx, `SELECT status, amount_due_minor, amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = ? AND kind = 'wage' AND contract_id = ?`, obligationID, m2EconomyContractID).Scan(&result.Status, &result.Due, &result.Paid); err != nil {
		return M2WageClaimStatus{}, classifyMissing(err, "M2 wage claim")
	}
	var splitCount int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id = ?`, obligationID).Scan(&splitCount); err != nil {
		return M2WageClaimStatus{}, core.WrapError(core.CodeStorageFailure, "check wage claim participation", err)
	}
	transferred, err := hasM2WageOwnerTransitions(ctx, conn, obligationID)
	if err != nil {
		return M2WageClaimStatus{}, err
	}
	if splitCount == 0 && !transferred {
		return M2WageClaimStatus{}, core.NewError(core.CodeNotFound, "named wage claim detail is not recorded for this obligation")
	}
	if err := verifyM2WageParticipationReturns(ctx, conn); err != nil {
		return M2WageClaimStatus{}, err
	}
	if err := verifyM2ObligationPaid(ctx, conn, obligationID, "wage", result.Paid); err != nil {
		return M2WageClaimStatus{}, err
	}
	slots, err := loadM2WageClaimSlots(ctx, conn, obligationID, math.MaxInt64)
	if err != nil {
		return M2WageClaimStatus{}, err
	}
	result.Current = make([]M2WageClaimBalance, 0, len(slots))
	var due, paid int64
	for _, slot := range slots {
		result.Current = append(result.Current, M2WageClaimBalance{slot.Index, slot.OriginKind, slot.OriginID, slot.OwnerKind, slot.OwnerID, slot.Due, slot.Paid, slot.Due - slot.Paid})
		due += slot.Due
		paid += slot.Paid
	}
	if due != result.Due || paid != result.Paid {
		return M2WageClaimStatus{}, core.NewError(core.CodeProjectionDiverged, "wage claim status does not conserve worker slots")
	}
	result.Outstanding = result.Due - result.Paid
	var actor string
	err = conn.QueryRowContext(ctx, `SELECT actor_id FROM m2_bankruptcy_proceedings WHERE proceeding_id = ?`, m2ProceedingID).Scan(&actor)
	if err != nil && err != sql.ErrNoRows {
		return M2WageClaimStatus{}, core.WrapError(core.CodeStorageFailure, "read wage claim proceeding", err)
	}
	if err == nil {
		if err := verifyM2BankruptcyClaims(ctx, conn, actor); err != nil {
			return M2WageClaimStatus{}, err
		}
		rows, err := conn.QueryContext(ctx, `SELECT slot_index, origin_kind, origin_id, claimant_kind, claimant_id, due_minor, paid_at_open_minor, outstanding_at_open_minor FROM m2_bankruptcy_slot_claims WHERE proceeding_id = ? AND obligation_id = ? ORDER BY slot_index`, m2ProceedingID, obligationID)
		if err != nil {
			return M2WageClaimStatus{}, core.WrapError(core.CodeStorageFailure, "read wage claim opening status", err)
		}
		for rows.Next() {
			var line M2WageClaimBalance
			if err := rows.Scan(&line.SlotIndex, &line.OriginKind, &line.OriginID, &line.ClaimantKind, &line.ClaimantID, &line.Due, &line.Paid, &line.Outstanding); err != nil {
				rows.Close()
				return M2WageClaimStatus{}, core.WrapError(core.CodeStorageFailure, "scan wage claim opening status", err)
			}
			result.Opening = append(result.Opening, line)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return M2WageClaimStatus{}, core.WrapError(core.CodeStorageFailure, "iterate wage claim opening status", err)
		}
		rows.Close()
	}
	return result, nil
}
