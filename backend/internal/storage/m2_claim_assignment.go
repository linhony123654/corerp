package storage

import (
	"context"
	"database/sql"
	"errors"

	"corerp.local/backend/internal/core"
)

type m2ClaimShare struct {
	ObligationID string `json:"obligation_id"`
	Amount       int64  `json:"amount_minor"`
}

// Allocate only proven, unpaid, post-term wage claims. A claim's residual
// Cohort ownership is divided among the current members, with no rounding or
// inferred personal work history. A paid share is removed only from the
// claimant who actually received it; opening snapshots remain immutable.
func planM2ClaimAllocation(ctx context.Context, conn *sql.Conn, cohortID string, cohortPopulation, extractedPopulation, receivableMinor int64) ([]m2ClaimShare, error) {
	var actorID string
	err := conn.QueryRowContext(ctx, `SELECT p.actor_id FROM m2_bankruptcy_proceedings p JOIN m2_cohort_contracts k ON k.actor_id = p.actor_id AND k.kind = 'wage' WHERE k.cohort_id = ?`, cohortID).Scan(&actorID)
	if errors.Is(err, sql.ErrNoRows) {
		var unpaid int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(o.amount_due_minor - o.amount_paid_minor), 0) FROM m2_economic_obligations o JOIN m2_cohort_contracts k ON k.contract_id = o.contract_id WHERE k.cohort_id = ? AND o.kind = 'wage'`, cohortID).Scan(&unpaid); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "check unregistered M2 wage claims", err)
		}
		if unpaid > 0 {
			return nil, core.NewError(core.CodeMaterializationConflict, "unpaid M2 wage claims lack an assignable proceeding snapshot")
		}
		return nil, nil
	}
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "find M2 wage claim proceeding", err)
	}
	if err := verifyM2BankruptcyClaims(ctx, conn, actorID); err != nil {
		return nil, err
	}
	if err := verifyM2ClaimAllocationLineagesForCohort(ctx, conn, cohortID); err != nil {
		return nil, err
	}
	if cohortPopulation <= 0 || extractedPopulation != 1 || extractedPopulation > cohortPopulation {
		return nil, core.NewError(core.CodeMaterializationConflict, "M2 claim-bearing refinement requires one current member")
	}
	rows, err := conn.QueryContext(ctx, `SELECT c.obligation_id, c.outstanding_at_open_minor, c.paid_at_open_minor, o.amount_paid_minor, COALESCE((SELECT SUM(a.amount_minor) FROM m2_bankruptcy_claim_allocations a LEFT JOIN m2_bankruptcy_claim_returns r ON r.materialization_id = a.materialization_id AND r.obligation_id = a.obligation_id WHERE a.obligation_id = c.obligation_id AND r.materialization_id IS NULL), 0), COALESCE((SELECT SUM(receipt.amount_minor) FROM m2_estate_distribution_receipts receipt JOIN m2_estate_distributions d ON d.scheduler_item_id = receipt.scheduler_item_id AND d.status = 'paid' WHERE d.obligation_id = c.obligation_id AND receipt.claimant_kind = 'cohort' AND receipt.claimant_id = c.claimant_cohort_id), 0) FROM m2_bankruptcy_claims c JOIN m2_economic_obligations o ON o.obligation_id = c.obligation_id WHERE c.claimant_cohort_id = ? ORDER BY c.obligation_id`, cohortID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read M2 assignable wage claims", err)
	}
	shares := make([]m2ClaimShare, 0)
	var total int64
	for rows.Next() {
		var claimID string
		var outstanding, paidAtOpen, currentPaid, assigned, cohortPaid int64
		if err := rows.Scan(&claimID, &outstanding, &paidAtOpen, &currentPaid, &assigned, &cohortPaid); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan M2 assignable wage claim", err)
		}
		estatePaid, err := verifyM2EstatePaymentFacts(ctx, conn, claimID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if err := verifyM2ObligationPaid(ctx, conn, claimID, "wage", currentPaid); err != nil {
			rows.Close()
			return nil, err
		}
		paidAfterOpen, ok := checkedSubtract(currentPaid, paidAtOpen)
		remaining, remainingOK := checkedSubtract(outstanding, assigned)
		remaining, cohortOK := checkedSubtract(remaining, cohortPaid)
		if !ok || !remainingOK || !cohortOK || paidAfterOpen != estatePaid || cohortPaid > estatePaid || remaining < 0 || remaining%cohortPopulation != 0 {
			rows.Close()
			return nil, core.NewError(core.CodeConservationFailed, "M2 wage claim cannot be divided among current Cohort members")
		}
		share := remaining / cohortPopulation
		if share == 0 {
			continue
		}
		total, ok = checkedAdd(total, share)
		if !ok {
			rows.Close()
			return nil, core.NewError(core.CodeIntegerOverflow, "M2 claim allocation total overflows")
		}
		shares = append(shares, m2ClaimShare{claimID, share})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, core.WrapError(core.CodeStorageFailure, "iterate M2 assignable wage claims", err)
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close M2 assignable wage claims", err)
	}
	if total != receivableMinor {
		return nil, core.NewError(core.CodeConservationFailed, "materialized receivable must equal assigned M2 wage claims")
	}
	return shares, nil
}

func recordM2ClaimAllocation(ctx context.Context, conn *sql.Conn, materializationID, entityID, eventID string, sequence int64, shares []m2ClaimShare) error {
	for _, share := range shares {
		if err := execAgentOne(ctx, conn, "assign M2 wage claim", `INSERT INTO m2_bankruptcy_claim_allocations(materialization_id, obligation_id, entity_id, amount_minor, allocation_event_id, allocation_event_sequence) VALUES (?, ?, ?, ?, ?, ?)`, materializationID, share.ObligationID, entityID, share.Amount, eventID, sequence); err != nil {
			return err
		}
	}
	return nil
}

func planM2ClaimReturn(ctx context.Context, conn *sql.Conn, materializationID, entityID string, receivableMinor int64) ([]m2ClaimShare, error) {
	if err := verifyM2ClaimAllocationLineage(ctx, conn, materializationID); err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT a.obligation_id, a.amount_minor, a.entity_id, o.amount_paid_minor, r.materialization_id, COALESCE((SELECT SUM(receipt.amount_minor) FROM m2_estate_distribution_receipts receipt JOIN m2_estate_distributions d ON d.scheduler_item_id = receipt.scheduler_item_id AND d.status = 'paid' WHERE d.obligation_id = a.obligation_id AND d.event_sequence > a.allocation_event_sequence AND receipt.claimant_kind = 'entity' AND receipt.claimant_id = a.entity_id), 0) FROM m2_bankruptcy_claim_allocations a JOIN m2_economic_obligations o ON o.obligation_id = a.obligation_id LEFT JOIN m2_bankruptcy_claim_returns r ON r.materialization_id = a.materialization_id AND r.obligation_id = a.obligation_id WHERE a.materialization_id = ? ORDER BY a.obligation_id`, materializationID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read M2 claim return", err)
	}
	shares := make([]m2ClaimShare, 0)
	var total int64
	for rows.Next() {
		var obligationID, owner string
		var amount, currentPaid, entityPaid int64
		var returned sql.NullString
		if err := rows.Scan(&obligationID, &amount, &owner, &currentPaid, &returned, &entityPaid); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan M2 claim return", err)
		}
		if owner != entityID || returned.Valid || entityPaid != 0 {
			rows.Close()
			return nil, core.NewError(core.CodeConservationFailed, "M2 assigned claim changed before dematerialization")
		}
		if err := verifyM2ObligationPaid(ctx, conn, obligationID, "wage", currentPaid); err != nil {
			rows.Close()
			return nil, err
		}
		var ok bool
		total, ok = checkedAdd(total, amount)
		if !ok {
			rows.Close()
			return nil, core.NewError(core.CodeIntegerOverflow, "M2 returned claims overflow")
		}
		shares = append(shares, m2ClaimShare{obligationID, amount})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, core.WrapError(core.CodeStorageFailure, "iterate M2 claim return", err)
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close M2 claim return", err)
	}
	if len(shares) > 0 && total != receivableMinor {
		return nil, core.NewError(core.CodeConservationFailed, "returned M2 wage claims differ from materialized receivable")
	}
	return shares, nil
}

func recordM2ClaimReturn(ctx context.Context, conn *sql.Conn, materializationID, eventID string, sequence int64, shares []m2ClaimShare) error {
	for _, share := range shares {
		if err := execAgentOne(ctx, conn, "return M2 wage claim to Cohort", `INSERT INTO m2_bankruptcy_claim_returns(materialization_id, obligation_id, amount_minor, return_event_id, return_event_sequence) VALUES (?, ?, ?, ?, ?)`, materializationID, share.ObligationID, share.Amount, eventID, sequence); err != nil {
			return err
		}
	}
	return nil
}

func verifyM2ClaimAllocationLineagesForCohort(ctx context.Context, conn *sql.Conn, cohortID string) error {
	rows, err := conn.QueryContext(ctx, `SELECT DISTINCT a.materialization_id FROM m2_bankruptcy_claim_allocations a JOIN m2_bankruptcy_claims c ON c.obligation_id = a.obligation_id WHERE c.claimant_cohort_id = ? ORDER BY a.materialization_id`, cohortID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read M2 claim allocation lineages", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan M2 claim allocation lineage", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate M2 claim allocation lineages", err)
	}
	if err := rows.Close(); err != nil {
		return core.WrapError(core.CodeStorageFailure, "close M2 claim allocation lineages", err)
	}
	for _, id := range ids {
		if err := verifyM2ClaimAllocationLineage(ctx, conn, id); err != nil {
			return err
		}
	}
	return nil
}

func verifyM2ClaimAllocationLineage(ctx context.Context, conn *sql.Conn, materializationID string) error {
	var entityID, eventID, expectedHash string
	var sequence, receivable, expectedCount int64
	if err := conn.QueryRowContext(ctx, `SELECT m.entity_id, m.materialize_event_id, m.materialize_sequence, m.receivable_minor, COALESCE(json_extract(e.payload, '$.claim_allocation_count'), 0), COALESCE(json_extract(e.payload, '$.claim_allocation_hash'), '') FROM cohort_materializations m JOIN events e ON e.event_id = m.materialize_event_id AND e.event_sequence = m.materialize_sequence AND e.event_type = 'CohortMaterialized' WHERE m.materialization_id = ?`, materializationID).Scan(&entityID, &eventID, &sequence, &receivable, &expectedCount, &expectedHash); err != nil {
		return classifyMissing(err, "M2 claim materialization authority")
	}
	rows, err := conn.QueryContext(ctx, `SELECT obligation_id, amount_minor, entity_id, allocation_event_id, allocation_event_sequence FROM m2_bankruptcy_claim_allocations WHERE materialization_id = ? ORDER BY obligation_id`, materializationID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read M2 claim allocation facts", err)
	}
	shares := make([]m2ClaimShare, 0)
	var total int64
	for rows.Next() {
		var share m2ClaimShare
		var owner, factEvent string
		var factSequence int64
		if err := rows.Scan(&share.ObligationID, &share.Amount, &owner, &factEvent, &factSequence); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan M2 claim allocation fact", err)
		}
		if owner != entityID || factEvent != eventID || factSequence != sequence {
			rows.Close()
			return core.NewError(core.CodeProjectionDiverged, "M2 claim allocation lacks its materialization event")
		}
		var ok bool
		total, ok = checkedAdd(total, share.Amount)
		if !ok {
			rows.Close()
			return core.NewError(core.CodeIntegerOverflow, "M2 claim allocation total overflows")
		}
		shares = append(shares, share)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate M2 claim allocation facts", err)
	}
	if err := rows.Close(); err != nil {
		return core.WrapError(core.CodeStorageFailure, "close M2 claim allocation facts", err)
	}
	if len(shares) == 0 {
		if expectedCount != 0 || expectedHash != "" {
			return core.NewError(core.CodeProjectionDiverged, "M2 claim allocation event lacks its facts")
		}
		return verifyM2ClaimReturnLineage(ctx, conn, materializationID, 0)
	}
	actualHash, err := core.HashJSON(shares)
	if err != nil {
		return err
	}
	if expectedCount != int64(len(shares)) || expectedHash != actualHash || total != receivable {
		return core.NewError(core.CodeProjectionDiverged, "M2 claim allocation does not conserve materialized receivable")
	}
	return verifyM2ClaimReturnLineage(ctx, conn, materializationID, len(shares))
}

func verifyM2ClaimReturnLineage(ctx context.Context, conn *sql.Conn, materializationID string, allocationCount int) error {
	var status, expectedHash string
	var eventID, linkedEvent sql.NullString
	var sequence sql.NullInt64
	var expectedCount int64
	if err := conn.QueryRowContext(ctx, `SELECT m.status, m.dematerialize_event_id, m.dematerialize_sequence, e.event_id, COALESCE(json_extract(e.payload, '$.claim_return_count'), 0), COALESCE(json_extract(e.payload, '$.claim_return_hash'), '') FROM cohort_materializations m LEFT JOIN events e ON e.event_id = m.dematerialize_event_id AND e.event_sequence = m.dematerialize_sequence AND e.event_type = 'CohortDematerialized' WHERE m.materialization_id = ?`, materializationID).Scan(&status, &eventID, &sequence, &linkedEvent, &expectedCount, &expectedHash); err != nil {
		return classifyMissing(err, "M2 claim return authority")
	}
	rows, err := conn.QueryContext(ctx, `SELECT obligation_id, amount_minor, return_event_id, return_event_sequence FROM m2_bankruptcy_claim_returns WHERE materialization_id = ? ORDER BY obligation_id`, materializationID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read M2 claim return facts", err)
	}
	shares := make([]m2ClaimShare, 0)
	for rows.Next() {
		var share m2ClaimShare
		var factEvent string
		var factSequence int64
		if err := rows.Scan(&share.ObligationID, &share.Amount, &factEvent, &factSequence); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan M2 claim return fact", err)
		}
		if !eventID.Valid || !sequence.Valid || factEvent != eventID.String || factSequence != sequence.Int64 {
			rows.Close()
			return core.NewError(core.CodeProjectionDiverged, "M2 claim return lacks its dematerialization event")
		}
		shares = append(shares, share)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate M2 claim return facts", err)
	}
	if err := rows.Close(); err != nil {
		return core.WrapError(core.CodeStorageFailure, "close M2 claim return facts", err)
	}
	if status == "active" {
		if len(shares) != 0 || eventID.Valid || expectedCount != 0 || expectedHash != "" {
			return core.NewError(core.CodeProjectionDiverged, "active M2 claim assignment has a return")
		}
		return nil
	}
	if status != "dematerialized" || !eventID.Valid || !sequence.Valid || !linkedEvent.Valid || linkedEvent.String != eventID.String || len(shares) != allocationCount || expectedCount != int64(len(shares)) {
		return core.NewError(core.CodeProjectionDiverged, "M2 claim return count differs from materialization")
	}
	if len(shares) == 0 {
		if expectedHash != "" {
			return core.NewError(core.CodeProjectionDiverged, "empty M2 claim return has a hash")
		}
		return nil
	}
	actualHash, err := core.HashJSON(shares)
	if err != nil {
		return err
	}
	if actualHash != expectedHash {
		return core.NewError(core.CodeProjectionDiverged, "M2 claim return facts differ from event")
	}
	return nil
}
