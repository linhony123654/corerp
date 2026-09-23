package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"corerp.local/backend/internal/core"
)

// A slot is a deterministic accounting share of the immutable wage contract,
// not a newly invented personal employment history.
type m2WageClaimSlot struct {
	ObligationID string `json:"obligation_id"`
	Index        int64  `json:"slot_index"`
	OriginKind   string `json:"origin_kind"`
	OriginID     string `json:"origin_id"`
	OwnerKind    string `json:"owner_kind"`
	OwnerID      string `json:"owner_id"`
	Due          int64  `json:"due_minor"`
	Paid         int64  `json:"paid_minor"`
}

type m2WageClaimTransition struct {
	ObligationID string `json:"obligation_id"`
	SlotIndex    int64  `json:"slot_index"`
	FromKind     string `json:"from_kind"`
	FromID       string `json:"from_id"`
	ToKind       string `json:"to_kind"`
	ToID         string `json:"to_id"`
	Outstanding  int64  `json:"outstanding_minor"`
}

type m2WageSlotReceipt struct {
	SlotIndex int64  `json:"slot_index"`
	Kind      string `json:"claimant_kind"`
	ID        string `json:"claimant_id"`
	Amount    int64  `json:"amount_minor"`
}

type m2WageSlotPayment struct {
	Postings []scheduledPosting
	Balances []balanceMutation
	Receipts []m2WageSlotReceipt
}

func m2WageSlotPaid(cumulative, slotIndex, workers int64) int64 {
	paid := cumulative / workers
	if slotIndex < cumulative%workers {
		paid++
	}
	return paid
}

func m2WageOwnerAccounts(ctx context.Context, conn *sql.Conn, kind, owner string) (string, string, error) {
	if kind == "cohort" {
		if owner != M2DemoCohortID {
			return "", "", core.NewError(core.CodeProjectionDiverged, "unknown wage Cohort claimant")
		}
		return M2DemoCohortAssetAccountID, M2DemoCohortReceivableID, nil
	}
	if kind != "entity" {
		return "", "", core.NewError(core.CodeProjectionDiverged, "unknown wage claimant kind")
	}
	var asset, receivable string
	if err := conn.QueryRowContext(ctx, `SELECT asset_account_id, receivable_account_id FROM materialized_entities WHERE entity_id = ? AND status = 'active'`, owner).Scan(&asset, &receivable); err != nil {
		return "", "", classifyMissing(err, "active wage claimant")
	}
	return asset, receivable, nil
}

func planM2WageSlotPayment(ctx context.Context, conn *sql.Conn, obligationID string, due, priorPaid, payment int64) (m2WageSlotPayment, error) {
	var plan m2WageSlotPayment
	if due <= 0 || priorPaid < 0 || payment < 0 || priorPaid > due || payment > due-priorPaid {
		return plan, core.NewError(core.CodeProjectionDiverged, "invalid slot wage payment total")
	}
	slots, err := loadM2WageClaimSlots(ctx, conn, obligationID, math.MaxInt64)
	if err != nil {
		return plan, err
	}
	if len(slots) == 0 {
		return plan, core.NewError(core.CodeProjectionDiverged, "wage payment has no worker slots")
	}
	workers := int64(len(slots))
	var computedDue, computedPaid int64
	for _, slot := range slots {
		computedDue += slot.Due
		computedPaid += slot.Paid
	}
	if computedDue != due || computedPaid != priorPaid {
		return plan, core.NewError(core.CodeProjectionDiverged, "wage slot totals differ from obligation")
	}
	if err := verifyM2AccountProjection(ctx, conn, m2EconomyEmployerCash, M2DemoCurrencyID); err != nil {
		return plan, err
	}
	if err := verifyM2AccountProjection(ctx, conn, m2EconomyEmployerPayable, M2DemoCurrencyID); err != nil {
		return plan, err
	}
	employer, employerVersion, err := readScheduledBalance(ctx, conn, m2EconomyEmployerCash)
	if err != nil {
		return plan, err
	}
	payable, payableVersion, err := readScheduledBalance(ctx, conn, m2EconomyEmployerPayable)
	if err != nil {
		return plan, err
	}
	if employer < payment || employer < 0 || payable > -(due-priorPaid) {
		return plan, core.NewError(core.CodeProjectionDiverged, "wage slot employer ledger cannot cover payment")
	}
	type destination struct {
		kind, owner, asset, receivable string
		amount                         int64
	}
	destinations := map[string]*destination{}
	plan.Receipts = []m2WageSlotReceipt{}
	var allocated int64
	for _, slot := range slots {
		before := m2WageSlotPaid(priorPaid, slot.Index, workers)
		after := m2WageSlotPaid(priorPaid+payment, slot.Index, workers)
		share := after - before
		if share < 0 || after > slot.Due {
			return m2WageSlotPayment{}, core.NewError(core.CodeProjectionDiverged, "wage slot payment exceeds worker claim")
		}
		if share == 0 {
			continue
		}
		asset, receivable, err := m2WageOwnerAccounts(ctx, conn, slot.OwnerKind, slot.OwnerID)
		if err != nil {
			return m2WageSlotPayment{}, err
		}
		key := slot.OwnerKind + ":" + slot.OwnerID
		entry := destinations[key]
		if entry == nil {
			entry = &destination{kind: slot.OwnerKind, owner: slot.OwnerID, asset: asset, receivable: receivable}
			destinations[key] = entry
		}
		if entry.asset != asset || entry.receivable != receivable {
			return m2WageSlotPayment{}, core.NewError(core.CodeProjectionDiverged, "wage slot claimant account changed")
		}
		entry.amount += share
		allocated += share
		plan.Receipts = append(plan.Receipts, m2WageSlotReceipt{slot.Index, slot.OwnerKind, slot.OwnerID, share})
	}
	if allocated != payment {
		return m2WageSlotPayment{}, core.NewError(core.CodeProjectionDiverged, "wage slot payment is not conserved")
	}
	plan.Postings = []scheduledPosting{}
	plan.Balances = []balanceMutation{}
	if payment == 0 {
		return plan, nil
	}
	plan.Postings = append(plan.Postings, scheduledPosting{m2EconomyEmployerCash, M2DemoCurrencyID, -payment, "M2 slot wage paid"}, scheduledPosting{m2EconomyEmployerPayable, M2DemoCurrencyID, payment, "M2 slot wage payable cleared"})
	plan.Balances = append(plan.Balances, balanceMutation{m2EconomyEmployerCash, employerVersion, employer - payment}, balanceMutation{m2EconomyEmployerPayable, payableVersion, payable + payment})
	keys := make([]string, 0, len(destinations))
	for key := range destinations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		destination := destinations[key]
		if err := verifyM2AccountProjection(ctx, conn, destination.asset, M2DemoCurrencyID); err != nil {
			return m2WageSlotPayment{}, err
		}
		if err := verifyM2AccountProjection(ctx, conn, destination.receivable, M2DemoCurrencyID); err != nil {
			return m2WageSlotPayment{}, err
		}
		asset, assetVersion, err := readScheduledBalance(ctx, conn, destination.asset)
		if err != nil {
			return m2WageSlotPayment{}, err
		}
		receivable, receivableVersion, err := readScheduledBalance(ctx, conn, destination.receivable)
		if err != nil {
			return m2WageSlotPayment{}, err
		}
		newAsset, assetOK := checkedAdd(asset, destination.amount)
		newReceivable, receivableOK := checkedSubtract(receivable, destination.amount)
		if !assetOK || !receivableOK || newReceivable < 0 || asset < 0 {
			return m2WageSlotPayment{}, core.NewError(core.CodeProjectionDiverged, "wage slot recipient ledger cannot cover payment")
		}
		plan.Postings = append(plan.Postings, scheduledPosting{destination.asset, M2DemoCurrencyID, destination.amount, "M2 slot wage received"}, scheduledPosting{destination.receivable, M2DemoCurrencyID, -destination.amount, "M2 slot wage receivable cleared"})
		plan.Balances = append(plan.Balances, balanceMutation{destination.asset, assetVersion, newAsset}, balanceMutation{destination.receivable, receivableVersion, newReceivable})
	}
	return plan, nil
}

func recordM2WageSlotReceipts(ctx context.Context, conn *sql.Conn, itemID, obligationID, eventID string, sequence int64, receipts []m2WageSlotReceipt) error {
	for _, receipt := range receipts {
		if err := execAgentOne(ctx, conn, "record actual wage slot claimant receipt", `INSERT INTO m2_wage_slot_receipts(scheduler_item_id, obligation_id, slot_index, claimant_kind, claimant_id, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, itemID, obligationID, receipt.SlotIndex, receipt.Kind, receipt.ID, receipt.Amount, eventID, sequence); err != nil {
			return err
		}
	}
	return nil
}

func prepareM2SlotAwareWageDue(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload, obligationID string, expectedDue int64) (scheduledMutation, error) {
	var due, paid int64
	if err := conn.QueryRowContext(ctx, `SELECT amount_due_minor, amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = ? AND contract_id = ? AND kind = 'wage' AND status = 'accrued'`, obligationID, m2EconomyContractID).Scan(&due, &paid); err != nil {
		return scheduledMutation{}, classifyMissing(err, "slot-aware wage due obligation")
	}
	if due != expectedDue || paid != 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "slot-aware wage due differs from contract")
	}
	if err := verifyM2ObligationPaid(ctx, conn, obligationID, "wage", 0); err != nil {
		return scheduledMutation{}, err
	}
	policy, err := readM2WageAllocationPolicy(ctx, conn, item.WorldTime)
	if err != nil {
		return scheduledMutation{}, err
	}
	if err := verifyM2AccountProjection(ctx, conn, m2EconomyEmployerCash, M2DemoCurrencyID); err != nil {
		return scheduledMutation{}, err
	}
	available, _, err := readScheduledBalance(ctx, conn, m2EconomyEmployerCash)
	if err != nil {
		return scheduledMutation{}, err
	}
	if available < 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "slot-aware wage employer cash is negative")
	}
	payment := due
	if available < payment {
		payment = available
	}
	plan, err := planM2WageSlotPayment(ctx, conn, obligationID, due, 0, payment)
	if err != nil {
		return scheduledMutation{}, err
	}
	receiptHash, err := core.HashJSON(plan.Receipts)
	if err != nil {
		return scheduledMutation{}, err
	}
	status, reason := "paid", ""
	if payment < due {
		status, reason = "partial", "insufficient_employer_liquidity"
	}
	if payment == 0 {
		status = "overdue"
	}
	return scheduledMutation{EventType: "M2WageSettled", EventPayload: struct {
		ObligationID  string `json:"obligation_id"`
		Status        string `json:"status"`
		Reason        string `json:"reason_code,omitempty"`
		Due           int64  `json:"due_minor"`
		Paid          int64  `json:"paid_minor"`
		Remaining     int64  `json:"remaining_minor"`
		Split         bool   `json:"split"`
		Policy        string `json:"allocation_policy"`
		ReceiptFormat string `json:"receipt_format"`
		ReceiptCount  int    `json:"receipt_count"`
		ReceiptHash   string `json:"receipt_hash"`
	}{obligationID, status, reason, due, payment, due - payment, true, policy, "slot_owner_v1", len(plan.Receipts), receiptHash}, Postings: plan.Postings, Balances: plan.Balances, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "settle slot-aware wage due", `UPDATE m2_economic_obligations SET amount_paid_minor = ?, status = ?, last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor = 0 AND status = 'accrued'`, payment, status, sequence, obligationID); err != nil {
			return err
		}
		if err := recordM2WageSlotReceipts(ctx, conn, item.SchedulerItemID, obligationID, eventID, sequence, plan.Receipts); err != nil {
			return err
		}
		if payment < due {
			return recordM2ArrearsCase(ctx, conn, obligationID, m2EconomyContractID, "wage", payload.Day, 3, eventID, sequence)
		}
		return nil
	}}, nil
}

func prepareM2SlotAwareWageRetry(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload, obligationID string, due, paid int64, graceExpires string) (scheduledMutation, error) {
	if err := verifyM2ObligationPaid(ctx, conn, obligationID, "wage", paid); err != nil {
		return scheduledMutation{}, err
	}
	policy, err := readM2WageAllocationPolicy(ctx, conn, item.WorldTime)
	if err != nil {
		return scheduledMutation{}, err
	}
	if err := verifyM2AccountProjection(ctx, conn, m2EconomyEmployerCash, M2DemoCurrencyID); err != nil {
		return scheduledMutation{}, err
	}
	available, _, err := readScheduledBalance(ctx, conn, m2EconomyEmployerCash)
	if err != nil {
		return scheduledMutation{}, err
	}
	if available < 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "slot-aware retry employer cash is negative")
	}
	remaining := due - paid
	payment := remaining
	if available < payment {
		payment = available
	}
	plan, err := planM2WageSlotPayment(ctx, conn, obligationID, due, paid, payment)
	if err != nil {
		return scheduledMutation{}, err
	}
	receiptHash, err := core.HashJSON(plan.Receipts)
	if err != nil {
		return scheduledMutation{}, err
	}
	status, reason := "paid", "none"
	if payment == 0 {
		status, reason = "deferred", "insufficient_liquidity"
	} else if payment < remaining {
		status = "partial"
	}
	return scheduledMutation{EventType: "M2ArrearsRetried", EventPayload: struct {
		ContractID    string `json:"contract_id"`
		ObligationID  string `json:"obligation_id"`
		Day           int    `json:"day"`
		Status        string `json:"status"`
		Reason        string `json:"reason_code"`
		PreviousPaid  int64  `json:"previously_paid_minor"`
		Paid          int64  `json:"paid_minor"`
		Remaining     int64  `json:"remaining_minor"`
		GraceExpires  string `json:"grace_expires_at"`
		Split         bool   `json:"split"`
		Policy        string `json:"allocation_policy"`
		ReceiptFormat string `json:"receipt_format"`
		ReceiptCount  int    `json:"receipt_count"`
		ReceiptHash   string `json:"receipt_hash"`
	}{m2EconomyContractID, obligationID, payload.Day, status, reason, paid, payment, remaining - payment, graceExpires, true, policy, "slot_owner_v1", len(plan.Receipts), receiptHash}, Postings: plan.Postings, Balances: plan.Balances, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "record slot-aware arrears attempt", `INSERT INTO m2_arrears_attempts(scheduler_item_id, contract_id, obligation_id, day, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, m2EconomyContractID, obligationID, payload.Day, status, reason, payment, eventID, sequence); err != nil {
			return err
		}
		if err := recordM2WageSlotReceipts(ctx, conn, item.SchedulerItemID, obligationID, eventID, sequence, plan.Receipts); err != nil {
			return err
		}
		if payment == 0 {
			return nil
		}
		if err := execAgentOne(ctx, conn, "update original slot-aware wage after retry", `UPDATE m2_economic_obligations SET amount_paid_minor = ?, status = ?, last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor = ? AND amount_due_minor = ?`, paid+payment, status, sequence, obligationID, paid, due); err != nil {
			return err
		}
		if status == "paid" {
			return execAgentOne(ctx, conn, "cure slot-aware wage arrears", `UPDATE m2_arrears_cases SET status = 'cured', last_event_sequence = ? WHERE obligation_id = ? AND status IN ('open', 'grace_expired')`, sequence, obligationID)
		}
		return execAgentOne(ctx, conn, "checkpoint slot-aware wage arrears", `UPDATE m2_arrears_cases SET last_event_sequence = ? WHERE obligation_id = ? AND status IN ('open', 'grace_expired')`, sequence, obligationID)
	}}, nil
}

func loadM2WageClaimSlots(ctx context.Context, conn *sql.Conn, obligationID string, beforeSequence int64) ([]m2WageClaimSlot, error) {
	var due, paid, rate, workers int64
	var cohortID string
	if err := conn.QueryRowContext(ctx, `SELECT o.amount_due_minor, o.amount_paid_minor, c.unit_rate_minor, c.participant_count, c.cohort_id FROM m2_economic_obligations o JOIN m2_cohort_contracts c ON c.contract_id = o.contract_id WHERE o.obligation_id = ? AND o.kind = 'wage' AND c.kind = 'wage'`, obligationID).Scan(&due, &paid, &rate, &workers, &cohortID); err != nil {
		return nil, classifyMissing(err, "wage claim slot obligation")
	}
	if due <= 0 || paid < 0 || paid > due || rate <= 0 || workers <= 0 || due != rate*workers || cohortID != M2DemoCohortID {
		return nil, core.NewError(core.CodeProjectionDiverged, "wage claim slot contract differs from obligation")
	}
	slices, err := loadM2WageObligationSlices(ctx, conn, obligationID)
	if err != nil {
		return nil, err
	}
	if len(slices) == 0 {
		slices = []m2WageSlice{{kind: "cohort", claimant: cohortID, due: due}}
	}
	var sliceDue int64
	for index, slice := range slices {
		if slice.due <= 0 || slice.due%rate != 0 || (index == 0 && (slice.kind != "cohort" || slice.claimant != cohortID)) || (index > 0 && (slice.kind != "entity" || slice.due != rate)) {
			return nil, core.NewError(core.CodeProjectionDiverged, "wage claim origin slices are invalid")
		}
		sliceDue += slice.due
	}
	if sliceDue != due || workers > 10000 {
		return nil, core.NewError(core.CodeProjectionDiverged, "wage claim origin slices do not conserve contract")
	}
	cycles, extra := paid/workers, paid%workers
	slots := make([]m2WageClaimSlot, 0, workers)
	for _, slice := range slices {
		for n := int64(0); n < slice.due/rate; n++ {
			index := int64(len(slots))
			paidSlot := cycles
			if index < extra {
				paidSlot++
			}
			if paidSlot > rate {
				return nil, core.NewError(core.CodeProjectionDiverged, "wage claim slot was overpaid")
			}
			slots = append(slots, m2WageClaimSlot{obligationID, index, slice.kind, slice.claimant, slice.kind, slice.claimant, rate, paidSlot})
		}
	}
	if int64(len(slots)) != workers {
		return nil, core.NewError(core.CodeProjectionDiverged, "wage claim slot count differs from contract")
	}
	rows, err := conn.QueryContext(ctx, `SELECT t.slot_index, t.transition_kind, t.from_kind, t.from_id, t.to_kind, t.to_id, t.outstanding_minor, t.event_id, t.event_sequence, m.entity_id, m.source_cohort_id, m.materialize_event_id, m.materialize_sequence, m.dematerialize_event_id, m.dematerialize_sequence, e.event_type FROM m2_wage_claim_owner_transitions t JOIN cohort_materializations m ON m.materialization_id = t.materialization_id JOIN events e ON e.event_id = t.event_id AND e.event_sequence = t.event_sequence WHERE t.obligation_id = ? AND t.event_sequence < ? ORDER BY t.event_sequence, t.slot_index`, obligationID, beforeSequence)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read wage claim owner transitions", err)
	}
	var lastSequence int64
	type transitionFact struct {
		index, outstanding, sequence int64
		eventID                      string
	}
	facts := []transitionFact{}
	for rows.Next() {
		var index, outstanding, sequence, materializeSequence int64
		var dematerializeSequence sql.NullInt64
		var kind, fromKind, fromID, toKind, toID, eventID, entityID, sourceID, materializeEvent, eventType string
		var dematerializeEvent sql.NullString
		if err := rows.Scan(&index, &kind, &fromKind, &fromID, &toKind, &toID, &outstanding, &eventID, &sequence, &entityID, &sourceID, &materializeEvent, &materializeSequence, &dematerializeEvent, &dematerializeSequence, &eventType); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan wage claim owner transition", err)
		}
		if index < 0 || index >= int64(len(slots)) || sourceID != cohortID || sequence < lastSequence {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "wage claim owner transition has invalid lineage")
		}
		slot := &slots[index]
		if slot.OwnerKind != fromKind || slot.OwnerID != fromID || outstanding <= 0 || outstanding > rate {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "wage claim owner transition has invalid source")
		}
		if kind == "materialize" {
			if fromKind != "cohort" || toKind != "entity" || toID != entityID || eventID != materializeEvent || sequence != materializeSequence || eventType != "CohortMaterialized" {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "wage claim materialization lacks event authority")
			}
		} else if kind == "dematerialize" {
			if fromKind != "entity" || fromID != entityID || toKind != "cohort" || toID != cohortID || !dematerializeEvent.Valid || !dematerializeSequence.Valid || eventID != dematerializeEvent.String || sequence != dematerializeSequence.Int64 || eventType != "CohortDematerialized" {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "wage claim return lacks event authority")
			}
		} else {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "unknown wage claim owner transition")
		}
		slot.OwnerKind, slot.OwnerID = toKind, toID
		facts = append(facts, transitionFact{index, outstanding, sequence, eventID})
		lastSequence = sequence
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, core.WrapError(core.CodeStorageFailure, "iterate wage claim owner transitions", err)
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close wage claim owner transitions", err)
	}
	verifiedEvents := map[string]bool{}
	for _, fact := range facts {
		var historicalPaid int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(CAST(json_extract(payload, '$.paid_minor') AS INTEGER)), 0) FROM events WHERE instance_id = ? AND branch_id = ? AND event_type IN ('M2WageSettled', 'M2ArrearsRetried') AND json_extract(payload, '$.obligation_id') = ? AND event_sequence < ?`, M2DemoInstanceID, M2DemoBranchID, obligationID, fact.sequence).Scan(&historicalPaid); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "read wage claim transfer payment history", err)
		}
		if historicalPaid < 0 || historicalPaid > due || fact.outstanding != rate-m2WageSlotPaid(historicalPaid, fact.index, workers) {
			return nil, core.NewError(core.CodeProjectionDiverged, "wage claim transition amount differs from historical debt")
		}
		if !verifiedEvents[fact.eventID] {
			if err := verifyM2WageClaimTransitionEvent(ctx, conn, fact.eventID, fact.sequence); err != nil {
				return nil, err
			}
			verifiedEvents[fact.eventID] = true
		}
	}
	return slots, nil
}

func verifyM2WageClaimTransitionEvent(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
	var eventType, payloadJSON string
	if err := conn.QueryRowContext(ctx, `SELECT event_type, payload FROM events WHERE event_id = ? AND event_sequence = ?`, eventID, sequence).Scan(&eventType, &payloadJSON); err != nil {
		return classifyMissing(err, "wage claim transition event")
	}
	var payload struct {
		Receivable    int64  `json:"receivable_minor"`
		TransferCount int    `json:"wage_claim_transfer_count"`
		TransferHash  string `json:"wage_claim_transfer_hash"`
		ReturnCount   int    `json:"wage_claim_return_count"`
		ReturnHash    string `json:"wage_claim_return_hash"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return core.WrapError(core.CodeProjectionDiverged, "decode wage claim transition event", err)
	}
	rows, err := conn.QueryContext(ctx, `SELECT t.obligation_id, t.slot_index, t.from_kind, t.from_id, t.to_kind, t.to_id, t.outstanding_minor, t.transition_kind, t.event_sequence FROM m2_wage_claim_owner_transitions t JOIN m2_economic_obligations o ON o.obligation_id = t.obligation_id WHERE t.event_id = ? ORDER BY o.period_end, t.obligation_id, t.slot_index`, eventID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read wage claim event transitions", err)
	}
	transitions := []m2WageClaimTransition{}
	var total int64
	for rows.Next() {
		var tr m2WageClaimTransition
		var kind string
		var rowSequence int64
		if err := rows.Scan(&tr.ObligationID, &tr.SlotIndex, &tr.FromKind, &tr.FromID, &tr.ToKind, &tr.ToID, &tr.Outstanding, &kind, &rowSequence); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan wage claim event transition", err)
		}
		if rowSequence != sequence || (kind == "materialize" && eventType != "CohortMaterialized") || (kind == "dematerialize" && eventType != "CohortDematerialized") {
			rows.Close()
			return core.NewError(core.CodeProjectionDiverged, "wage claim transition event type differs")
		}
		var ok bool
		total, ok = checkedAdd(total, tr.Outstanding)
		if !ok {
			rows.Close()
			return core.NewError(core.CodeIntegerOverflow, "wage claim transition event amount overflows")
		}
		transitions = append(transitions, tr)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate wage claim event transitions", err)
	}
	rows.Close()
	hash, err := core.HashJSON(transitions)
	if err != nil {
		return err
	}
	if total != payload.Receivable || (eventType == "CohortMaterialized" && (len(transitions) != payload.TransferCount || hash != payload.TransferHash)) || (eventType == "CohortDematerialized" && (len(transitions) != payload.ReturnCount || hash != payload.ReturnHash)) {
		return core.NewError(core.CodeProjectionDiverged, "wage claim owner transitions differ from command event")
	}
	return nil
}

func planM2WageClaimSlotTransfer(ctx context.Context, conn *sql.Conn, command core.MaterializeCohortCommand) ([]m2WageClaimTransition, error) {
	rows, err := conn.QueryContext(ctx, `SELECT o.obligation_id, o.amount_paid_minor FROM m2_economic_obligations o JOIN m2_cohort_contracts c ON c.contract_id = o.contract_id WHERE c.cohort_id = ? AND c.kind = 'wage' AND o.kind = 'wage' AND o.amount_paid_minor < o.amount_due_minor ORDER BY o.period_end, o.obligation_id`, command.SourceCohortID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read transferable live wage claims", err)
	}
	type debt struct {
		id   string
		paid int64
	}
	debts := []debt{}
	for rows.Next() {
		var d debt
		if err := rows.Scan(&d.id, &d.paid); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan transferable live wage claim", err)
		}
		debts = append(debts, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, core.WrapError(core.CodeStorageFailure, "iterate transferable live wage claims", err)
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close transferable live wage claims", err)
	}
	transfers := []m2WageClaimTransition{}
	var total int64
	for _, debt := range debts {
		if err := verifyM2ObligationPaid(ctx, conn, debt.id, "wage", debt.paid); err != nil {
			return nil, err
		}
		slots, err := loadM2WageClaimSlots(ctx, conn, debt.id, math.MaxInt64)
		if err != nil {
			return nil, err
		}
		for index := len(slots) - 1; index >= 0; index-- {
			slot := slots[index]
			outstanding := slot.Due - slot.Paid
			if slot.OwnerKind != "cohort" || slot.OwnerID != command.SourceCohortID || outstanding == 0 {
				continue
			}
			var ok bool
			total, ok = checkedAdd(total, outstanding)
			if !ok {
				return nil, core.NewError(core.CodeIntegerOverflow, "wage claim slot transfer overflows")
			}
			transfers = append(transfers, m2WageClaimTransition{debt.id, slot.Index, "cohort", command.SourceCohortID, "entity", command.EntityID, outstanding})
			break
		}
	}
	if total != command.ReceivableMinor {
		return nil, core.NewError(core.CodeConservationFailed, fmt.Sprintf("materialized live wage receivable must equal selected claim slots (%d)", total))
	}
	return transfers, nil
}

func planM2WageClaimReturn(ctx context.Context, conn *sql.Conn, entityID, cohortID string) ([]m2WageClaimTransition, int64, int64, error) {
	if err := verifyM2WageParticipationReturns(ctx, conn); err != nil {
		return nil, 0, 0, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT o.obligation_id, o.amount_paid_minor FROM m2_economic_obligations o JOIN m2_cohort_contracts c ON c.contract_id = o.contract_id WHERE c.cohort_id = ? AND c.kind = 'wage' AND o.kind = 'wage' ORDER BY o.period_end, o.obligation_id`, cohortID)
	if err != nil {
		return nil, 0, 0, core.WrapError(core.CodeStorageFailure, "read returning wage claims", err)
	}
	type debt struct {
		id   string
		paid int64
	}
	debts := []debt{}
	for rows.Next() {
		var d debt
		if err := rows.Scan(&d.id, &d.paid); err != nil {
			rows.Close()
			return nil, 0, 0, core.WrapError(core.CodeStorageFailure, "scan returning wage claim", err)
		}
		debts = append(debts, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, 0, core.WrapError(core.CodeStorageFailure, "iterate returning wage claims", err)
	}
	rows.Close()
	transitions := []m2WageClaimTransition{}
	var receivable int64
	for _, debt := range debts {
		if err := verifyM2ObligationPaid(ctx, conn, debt.id, "wage", debt.paid); err != nil {
			return nil, 0, 0, err
		}
		slots, err := loadM2WageClaimSlots(ctx, conn, debt.id, math.MaxInt64)
		if err != nil {
			return nil, 0, 0, err
		}
		for _, slot := range slots {
			if slot.OwnerKind != "entity" || slot.OwnerID != entityID {
				continue
			}
			outstanding := slot.Due - slot.Paid
			if outstanding == 0 {
				continue
			}
			var ok bool
			receivable, ok = checkedAdd(receivable, outstanding)
			if !ok {
				return nil, 0, 0, core.NewError(core.CodeIntegerOverflow, "returning wage claim overflows")
			}
			transitions = append(transitions, m2WageClaimTransition{debt.id, slot.Index, "entity", entityID, "cohort", cohortID, outstanding})
		}
	}
	var oldCash, slotCash int64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_split_receipts WHERE claimant_kind = 'entity' AND claimant_id = ?`, entityID).Scan(&oldCash); err != nil {
		return nil, 0, 0, core.WrapError(core.CodeStorageFailure, "sum returning original wage receipts", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE claimant_kind = 'entity' AND claimant_id = ?`, entityID).Scan(&slotCash); err != nil {
		return nil, 0, 0, core.WrapError(core.CodeStorageFailure, "sum returning current-owner wage receipts", err)
	}
	cash, ok := checkedAdd(oldCash, slotCash)
	if !ok {
		return nil, 0, 0, core.NewError(core.CodeIntegerOverflow, "returning wage cash overflows")
	}
	return transitions, receivable, cash, nil
}

func hasM2WageOwnerTransitions(ctx context.Context, conn *sql.Conn, obligationID string) (bool, error) {
	var count int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_claim_owner_transitions WHERE obligation_id = ?`, obligationID).Scan(&count); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "check wage claim owner transitions", err)
	}
	return count > 0, nil
}

func recordM2WageClaimTransitions(ctx context.Context, conn *sql.Conn, materializationID, eventID string, sequence int64, kind string, transitions []m2WageClaimTransition) error {
	for _, transition := range transitions {
		id := fmt.Sprintf("wage_claim_%s_%s_%d_%s", materializationID, transition.ObligationID, transition.SlotIndex, kind)
		if err := execAgentOne(ctx, conn, "record wage claim owner transition", `INSERT INTO m2_wage_claim_owner_transitions(transition_id, materialization_id, obligation_id, slot_index, transition_kind, from_kind, from_id, to_kind, to_id, outstanding_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, materializationID, transition.ObligationID, transition.SlotIndex, kind, transition.FromKind, transition.FromID, transition.ToKind, transition.ToID, transition.Outstanding, eventID, sequence); err != nil {
			return err
		}
	}
	return nil
}

// Verify the whole payment chain across the legacy aggregate-receipt and
// current-owner slot-receipt formats. Neither format may rewrite the other.
func verifyM2WageSlotHistory(ctx context.Context, conn *sql.Conn, obligationID string, recorded int64) error {
	slots, err := loadM2WageClaimSlots(ctx, conn, obligationID, math.MaxInt64)
	if err != nil {
		return err
	}
	slices, err := loadM2WageObligationSlices(ctx, conn, obligationID)
	if err != nil {
		return err
	}
	var due, rate int64
	var periodEnd string
	if err := conn.QueryRowContext(ctx, `SELECT o.amount_due_minor, c.unit_rate_minor, o.period_end FROM m2_economic_obligations o JOIN m2_cohort_contracts c ON c.contract_id = o.contract_id WHERE o.obligation_id = ?`, obligationID).Scan(&due, &rate, &periodEnd); err != nil {
		return classifyMissing(err, "slot wage payment authority")
	}
	policy, err := readM2WageAllocationPolicy(ctx, conn, periodEnd)
	if err != nil {
		return err
	}
	type paymentEvent struct {
		id, kind, payload string
		sequence          int64
	}
	rows, err := conn.QueryContext(ctx, `SELECT event_id, event_sequence, event_type, payload FROM events WHERE instance_id = ? AND branch_id = ? AND event_type IN ('M2WageSettled', 'M2ArrearsRetried') AND json_extract(payload, '$.obligation_id') = ? ORDER BY event_sequence`, M2DemoInstanceID, M2DemoBranchID, obligationID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read slot wage payment history", err)
	}
	events := []paymentEvent{}
	for rows.Next() {
		var e paymentEvent
		if err := rows.Scan(&e.id, &e.sequence, &e.kind, &e.payload); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan slot wage payment history", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate slot wage payment history", err)
	}
	if err := rows.Close(); err != nil {
		return core.WrapError(core.CodeStorageFailure, "close slot wage payment history", err)
	}
	var cumulative, oldCount, oldTotal, slotCount, slotTotal int64
	seenDue := false
	for _, event := range events {
		var p struct {
			ObligationID string `json:"obligation_id"`
			Paid         int64  `json:"paid_minor"`
			Previous     int64  `json:"previously_paid_minor"`
			Remaining    int64  `json:"remaining_minor"`
			Split        bool   `json:"split"`
			Policy       string `json:"allocation_policy"`
			Format       string `json:"receipt_format"`
			Count        int    `json:"receipt_count"`
			Hash         string `json:"receipt_hash"`
		}
		if err := json.Unmarshal([]byte(event.payload), &p); err != nil {
			return core.WrapError(core.CodeProjectionDiverged, "decode slot wage payment", err)
		}
		if p.ObligationID != obligationID || p.Paid < 0 || p.Paid > due-cumulative || p.Remaining != due-cumulative-p.Paid {
			return core.NewError(core.CodeProjectionDiverged, "slot wage payment does not conserve debt")
		}
		if event.kind == "M2WageSettled" {
			if seenDue || cumulative != 0 {
				return core.NewError(core.CodeProjectionDiverged, "duplicate slot wage due payment")
			}
			seenDue = true
		} else {
			if !seenDue || p.Previous != cumulative {
				return core.NewError(core.CodeProjectionDiverged, "slot wage retry has wrong prior paid")
			}
			var amount int64
			if err := conn.QueryRowContext(ctx, `SELECT amount_minor FROM m2_arrears_attempts WHERE obligation_id = ? AND event_id = ? AND event_sequence = ?`, obligationID, event.id, event.sequence).Scan(&amount); err != nil {
				return classifyMissing(err, "slot wage retry fact")
			}
			if amount != p.Paid {
				return core.NewError(core.CodeProjectionDiverged, "slot wage retry amount differs from fact")
			}
		}
		current, err := loadM2WageClaimSlots(ctx, conn, obligationID, event.sequence)
		if err != nil {
			return err
		}
		if p.Format == "slot_owner_v1" {
			if !p.Split || p.Policy != policy {
				return core.NewError(core.CodeProjectionDiverged, "slot wage receipt policy differs")
			}
			expected := []m2WageSlotReceipt{}
			for _, slot := range current {
				amount := m2WageSlotPaid(cumulative+p.Paid, slot.Index, int64(len(current))) - m2WageSlotPaid(cumulative, slot.Index, int64(len(current)))
				if amount > 0 {
					expected = append(expected, m2WageSlotReceipt{slot.Index, slot.OwnerKind, slot.OwnerID, amount})
				}
			}
			actual := []m2WageSlotReceipt{}
			receipts, err := conn.QueryContext(ctx, `SELECT slot_index, claimant_kind, claimant_id, amount_minor, event_sequence FROM m2_wage_slot_receipts WHERE obligation_id = ? AND event_id = ? ORDER BY slot_index`, obligationID, event.id)
			if err != nil {
				return core.WrapError(core.CodeStorageFailure, "read slot wage receipts", err)
			}
			for receipts.Next() {
				var r m2WageSlotReceipt
				var sequence int64
				if err := receipts.Scan(&r.SlotIndex, &r.Kind, &r.ID, &r.Amount, &sequence); err != nil {
					receipts.Close()
					return core.WrapError(core.CodeStorageFailure, "scan slot wage receipt", err)
				}
				if sequence != event.sequence {
					receipts.Close()
					return core.NewError(core.CodeProjectionDiverged, "slot wage receipt sequence differs")
				}
				actual = append(actual, r)
			}
			if err := receipts.Err(); err != nil {
				receipts.Close()
				return core.WrapError(core.CodeStorageFailure, "iterate slot wage receipts", err)
			}
			receipts.Close()
			if len(actual) != len(expected) {
				return core.NewError(core.CodeProjectionDiverged, "slot wage receipt count differs")
			}
			for i := range expected {
				if expected[i] != actual[i] {
					return core.NewError(core.CodeProjectionDiverged, "slot wage paid wrong claimant")
				}
			}
			hash, err := core.HashJSON(actual)
			if err != nil {
				return err
			}
			if p.Count != len(actual) || p.Hash != hash {
				return core.NewError(core.CodeProjectionDiverged, "slot wage receipt hash differs")
			}
			slotCount += int64(len(actual))
			slotTotal += p.Paid
		} else {
			for _, slot := range current {
				if slot.OwnerKind != slot.OriginKind || slot.OwnerID != slot.OriginID {
					return core.NewError(core.CodeProjectionDiverged, "legacy wage payment after claim transfer")
				}
			}
			if len(slices) == 0 {
				if p.Split || p.Format != "" || p.Policy != "" {
					return core.NewError(core.CodeProjectionDiverged, "legacy aggregate wage has unexpected split evidence")
				}
			} else {
				if !p.Split || p.Format != "" || (p.Policy != policy && !(event.kind == "M2WageSettled" && p.Policy == "" && p.Paid == due)) {
					return core.NewError(core.CodeProjectionDiverged, "legacy split wage policy differs")
				}
				before, err := allocateM2WageCumulative(policy, slices, rate, due, cumulative)
				if err != nil {
					return err
				}
				after, err := allocateM2WageCumulative(policy, slices, rate, due, cumulative+p.Paid)
				if err != nil {
					return err
				}
				expected := []m2WageReceipt{}
				for i, slice := range slices {
					if after[i] > before[i] {
						expected = append(expected, m2WageReceipt{slice.kind, slice.claimant, after[i] - before[i]})
					}
				}
				actual := []m2WageReceipt{}
				receipts, err := conn.QueryContext(ctx, `SELECT claimant_kind, claimant_id, amount_minor, event_sequence FROM m2_wage_split_receipts WHERE obligation_id = ? AND event_id = ? ORDER BY CASE claimant_kind WHEN 'cohort' THEN 0 ELSE 1 END, claimant_id`, obligationID, event.id)
				if err != nil {
					return core.WrapError(core.CodeStorageFailure, "read legacy wage receipts", err)
				}
				for receipts.Next() {
					var r m2WageReceipt
					var sequence int64
					if err := receipts.Scan(&r.Kind, &r.ID, &r.Amount, &sequence); err != nil {
						receipts.Close()
						return core.WrapError(core.CodeStorageFailure, "scan legacy wage receipt", err)
					}
					if sequence != event.sequence {
						receipts.Close()
						return core.NewError(core.CodeProjectionDiverged, "legacy wage receipt sequence differs")
					}
					actual = append(actual, r)
				}
				if err := receipts.Err(); err != nil {
					receipts.Close()
					return core.WrapError(core.CodeStorageFailure, "iterate legacy wage receipts", err)
				}
				receipts.Close()
				if len(expected) != len(actual) {
					return core.NewError(core.CodeProjectionDiverged, "legacy wage recipient count differs")
				}
				for i := range expected {
					if expected[i] != actual[i] {
						return core.NewError(core.CodeProjectionDiverged, "legacy wage paid wrong claimant")
					}
				}
				hash, err := core.HashJSON(actual)
				if err != nil {
					return err
				}
				if p.Policy != "" && (p.Count != len(actual) || p.Hash != hash) {
					return core.NewError(core.CodeProjectionDiverged, "legacy wage receipt hash differs")
				}
				oldCount += int64(len(actual))
				oldTotal += p.Paid
			}
		}
		cumulative += p.Paid
	}
	if (len(events) == 0 && recorded != 0) || (len(events) > 0 && !seenDue) || cumulative != recorded {
		return core.NewError(core.CodeProjectionDiverged, "slot wage history differs from obligation")
	}
	var allOldCount, allOldTotal, allSlotCount, allSlotTotal int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(amount_minor), 0) FROM m2_wage_split_receipts WHERE obligation_id = ?`, obligationID).Scan(&allOldCount, &allOldTotal); err != nil {
		return core.WrapError(core.CodeStorageFailure, "count legacy wage receipts", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE obligation_id = ?`, obligationID).Scan(&allSlotCount, &allSlotTotal); err != nil {
		return core.WrapError(core.CodeStorageFailure, "count slot wage receipts", err)
	}
	if allOldCount != oldCount || allOldTotal != oldTotal || allSlotCount != slotCount || allSlotTotal != slotTotal {
		return core.NewError(core.CodeProjectionDiverged, "orphan or duplicate wage receipt")
	}
	_ = slots
	return nil
}
