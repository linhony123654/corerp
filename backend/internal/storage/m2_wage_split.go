package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

type m2WageParticipation struct {
	ContractID    string `json:"contract_id"`
	EffectiveFrom string `json:"effective_from"`
	WorkerCount   int64  `json:"worker_count"`
}

const (
	m2WageAllocationPolicyID      = "policy_m2_wage_round_robin_v1"
	m2WageAllocationPolicyVersion = "worker_round_robin_v1"
)

func readM2WageAllocationPolicy(ctx context.Context, conn *sql.Conn, at string) (string, error) {
	var policyID, version, definitionEvent string
	err := conn.QueryRowContext(ctx, `SELECT policy_id, policy_version, definition_event_id FROM m2_wage_allocation_policies WHERE contract_id = ? AND effective_from <= ? ORDER BY effective_from DESC LIMIT 1`, m2EconomyContractID, at).Scan(&policyID, &version, &definitionEvent)
	if err != nil {
		return "", classifyMissing(err, "M2 wage allocation policy")
	}
	if policyID != m2WageAllocationPolicyID || version != m2WageAllocationPolicyVersion || definitionEvent != m2EconomyEventID {
		return "", core.NewError(core.CodeProjectionDiverged, "M2 wage allocation policy differs from declared fixture")
	}
	return version, nil
}

func verifyM2WageParticipationReturns(ctx context.Context, conn *sql.Conn) error {
	var invalid int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_participation_splits s JOIN cohort_materializations m ON m.materialization_id = s.materialization_id LEFT JOIN m2_wage_participation_returns r ON r.materialization_id = s.materialization_id LEFT JOIN events e ON e.event_id = r.return_event_id AND e.event_sequence = r.return_event_sequence AND e.event_type = 'CohortDematerialized' WHERE s.contract_id = ? AND ((m.status = 'active' AND r.materialization_id IS NOT NULL) OR (m.status = 'dematerialized' AND (r.materialization_id IS NULL OR r.return_event_id != m.dematerialize_event_id OR r.return_event_sequence != m.dematerialize_sequence OR e.event_id IS NULL OR r.effective_from != e.world_time)) OR m.status NOT IN ('active', 'dematerialized'))`, m2EconomyContractID).Scan(&invalid); err != nil {
		return core.WrapError(core.CodeStorageFailure, "verify wage participation return lineage", err)
	}
	if invalid != 0 {
		return core.NewError(core.CodeProjectionDiverged, "wage participation return differs from T09 lineage")
	}
	return nil
}

// Participation starts at the first scheduled, unaccrued period. Earlier
// obligations are not rewritten into a newly materialized person's history.
func planM2WageParticipation(ctx context.Context, conn *sql.Conn, command core.MaterializeCohortCommand, priorPopulation, remaining int64) (*m2WageParticipation, error) {
	parsedTime, err := time.Parse(time.RFC3339, command.WorldTime)
	if err != nil {
		return nil, core.WrapError(core.CodeInvalidArgument, "invalid wage participation time", err)
	}
	worldTime := parsedTime.UTC().Format(time.RFC3339)
	var contractID, starts, ends string
	var participants, existing int64
	err = conn.QueryRowContext(ctx, `SELECT contract_id, participant_count, effective_from, effective_until FROM m2_cohort_contracts WHERE cohort_id = ? AND kind = 'wage' AND effective_from <= ? AND effective_until > ?`, command.SourceCohortID, worldTime, worldTime).Scan(&contractID, &participants, &starts, &ends)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read active wage participation", err)
	}
	if contractID != m2EconomyContractID || command.PopulationCount != 1 || remaining <= 0 {
		return nil, core.NewError(core.CodeMaterializationConflict, "active wage split requires one worker and a continuing Cohort")
	}
	if err := verifyM2WageParticipationReturns(ctx, conn); err != nil {
		return nil, err
	}
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(s.worker_count), 0) FROM m2_wage_participation_splits s JOIN cohort_materializations m ON m.materialization_id = s.materialization_id AND m.status = 'active' WHERE s.contract_id = ?`, contractID).Scan(&existing); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "count named wage participants", err)
	}
	if priorPopulation+existing != participants {
		return nil, core.NewError(core.CodeProjectionDiverged, "wage participants differ from contract")
	}
	var next string
	err = conn.QueryRowContext(ctx, `SELECT world_time FROM scheduler_items WHERE instance_id = ? AND branch_id = ? AND phase_id = ? AND status = 'pending' AND world_time > ? AND world_time < ? ORDER BY world_time, scheduler_item_id LIMIT 1`, command.InstanceID, command.BranchID, m2EconomyPhaseAccrue, worldTime, ends).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewError(core.CodeMaterializationConflict, "active wage contract has no unaccrued period to split")
	}
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "find next wage accrual", err)
	}
	return &m2WageParticipation{contractID, next, 1}, nil
}

type m2WageSlice struct {
	kind, claimant, asset, receivable, income string
	due                                       int64
}

type m2WageReceipt struct {
	Kind   string `json:"claimant_kind"`
	ID     string `json:"claimant_id"`
	Amount int64  `json:"amount_minor"`
}

// Cumulative allocation is monotone: every paid monetary unit walks one
// worker slot, with current Cohort slots first and named entities sorted by
// stable ID. Recomputing at a later cumulative total and subtracting the
// earlier total gives an exact retry delta with no timing-dependent rounding.
func allocateM2WageCumulative(policy string, slices []m2WageSlice, rate, totalDue, cumulativePaid int64) ([]int64, error) {
	if policy != m2WageAllocationPolicyVersion || rate <= 0 || totalDue <= 0 || cumulativePaid < 0 || cumulativePaid > totalDue || len(slices) == 0 || slices[0].kind != "cohort" {
		return nil, core.NewError(core.CodeProjectionDiverged, "invalid split wage allocation policy or amount")
	}
	var workers, sumDue int64
	for index, slice := range slices {
		if slice.due <= 0 || slice.due%rate != 0 || (index > 0 && (slice.kind != "entity" || slice.due != rate || slices[index-1].claimant >= slice.claimant && index > 1)) {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid split wage worker slots")
		}
		var ok bool
		workers, ok = checkedAdd(workers, slice.due/rate)
		if !ok {
			return nil, core.NewError(core.CodeIntegerOverflow, "split wage worker count overflows")
		}
		sumDue, ok = checkedAdd(sumDue, slice.due)
		if !ok {
			return nil, core.NewError(core.CodeIntegerOverflow, "split wage due total overflows")
		}
	}
	if sumDue != totalDue || workers <= 0 {
		return nil, core.NewError(core.CodeProjectionDiverged, "split wage slices do not conserve obligation")
	}
	cycles, slotsLeft := cumulativePaid/workers, cumulativePaid%workers
	result := make([]int64, len(slices))
	var allocated int64
	for index, slice := range slices {
		slots := slice.due / rate
		base, ok := checkedMultiplyPositive(cycles, slots)
		if cycles == 0 {
			base, ok = 0, true
		}
		if !ok {
			return nil, core.NewError(core.CodeIntegerOverflow, "split wage cumulative allocation overflows")
		}
		extra := slotsLeft
		if extra > slots {
			extra = slots
		}
		value, ok := checkedAdd(base, extra)
		if !ok || value > slice.due {
			return nil, core.NewError(core.CodeProjectionDiverged, "split wage claimant allocation exceeds due")
		}
		result[index] = value
		slotsLeft -= extra
		allocated, ok = checkedAdd(allocated, value)
		if !ok {
			return nil, core.NewError(core.CodeIntegerOverflow, "split wage allocation total overflows")
		}
	}
	if allocated != cumulativePaid || slotsLeft != 0 {
		return nil, core.NewError(core.CodeProjectionDiverged, "split wage paid amount is not conserved")
	}
	return result, nil
}

func verifyM2WageSplitReceipts(ctx context.Context, conn *sql.Conn, obligationID string, recorded int64) error {
	hasTransitions, err := hasM2WageOwnerTransitions(ctx, conn, obligationID)
	if err != nil {
		return err
	}
	if hasTransitions {
		return verifyM2WageSlotHistory(ctx, conn, obligationID, recorded)
	}
	slices, err := loadM2WageObligationSlices(ctx, conn, obligationID)
	if err != nil || len(slices) == 0 {
		return err
	}
	var due, rate int64
	var periodEnd string
	if err := conn.QueryRowContext(ctx, `SELECT o.amount_due_minor, c.unit_rate_minor, o.period_end FROM m2_economic_obligations o JOIN m2_cohort_contracts c ON c.contract_id = o.contract_id WHERE o.obligation_id = ? AND o.kind = 'wage'`, obligationID).Scan(&due, &rate, &periodEnd); err != nil {
		return classifyMissing(err, "split wage obligation authority")
	}
	policy, err := readM2WageAllocationPolicy(ctx, conn, periodEnd)
	if err != nil {
		return err
	}
	type paymentEvent struct {
		ID, Type, Payload string
		Sequence          int64
	}
	rows, err := conn.QueryContext(ctx, `SELECT event_id, event_sequence, event_type, payload FROM events WHERE instance_id = ? AND branch_id = ? AND event_type IN ('M2WageSettled', 'M2ArrearsRetried') AND json_extract(payload, '$.obligation_id') = ? ORDER BY event_sequence`, M2DemoInstanceID, M2DemoBranchID, obligationID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read split wage payment events", err)
	}
	events := []paymentEvent{}
	for rows.Next() {
		var e paymentEvent
		if err := rows.Scan(&e.ID, &e.Sequence, &e.Type, &e.Payload); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan split wage payment event", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate split wage payment events", err)
	}
	if err := rows.Close(); err != nil {
		return core.WrapError(core.CodeStorageFailure, "close split wage payment events", err)
	}
	if len(events) == 0 && recorded == 0 {
		var status string
		var receipts int64
		if err := conn.QueryRowContext(ctx, `SELECT status FROM m2_economic_obligations WHERE obligation_id = ?`, obligationID).Scan(&status); err != nil {
			return classifyMissing(err, "unpaid split wage obligation")
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_split_receipts WHERE obligation_id = ?`, obligationID).Scan(&receipts); err != nil {
			return core.WrapError(core.CodeStorageFailure, "count unaccrued wage receipts", err)
		}
		if status == "accrued" && receipts == 0 {
			return nil
		}
	}
	var cumulative, receiptTotal, receiptCount int64
	seenDue := false
	for _, event := range events {
		var payload struct {
			ObligationID string `json:"obligation_id"`
			Status       string `json:"status"`
			Paid         int64  `json:"paid_minor"`
			PreviousPaid int64  `json:"previously_paid_minor"`
			Remaining    int64  `json:"remaining_minor"`
			Split        bool   `json:"split"`
			Policy       string `json:"allocation_policy"`
			ReceiptCount int    `json:"receipt_count"`
			ReceiptHash  string `json:"receipt_hash"`
		}
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			return core.WrapError(core.CodeProjectionDiverged, "decode split wage payment event", err)
		}
		legacyFull := event.Type == "M2WageSettled" && payload.Policy == "" && payload.Paid == due
		if !payload.Split || payload.ObligationID != obligationID || (payload.Policy != policy && !legacyFull) || payload.Paid < 0 || payload.Paid > due-cumulative || payload.Remaining != due-cumulative-payload.Paid {
			return core.NewError(core.CodeProjectionDiverged, "split wage event does not conserve debt")
		}
		if event.Type == "M2WageSettled" {
			if seenDue || cumulative != 0 {
				return core.NewError(core.CodeProjectionDiverged, "duplicate split wage due payment")
			}
			seenDue = true
		} else {
			if !seenDue || payload.PreviousPaid != cumulative {
				return core.NewError(core.CodeProjectionDiverged, "split wage retry has wrong prior payment")
			}
			var attemptAmount int64
			if err := conn.QueryRowContext(ctx, `SELECT amount_minor FROM m2_arrears_attempts WHERE obligation_id = ? AND event_id = ? AND event_sequence = ?`, obligationID, event.ID, event.Sequence).Scan(&attemptAmount); err != nil {
				return classifyMissing(err, "split wage retry attempt")
			}
			if attemptAmount != payload.Paid {
				return core.NewError(core.CodeProjectionDiverged, "split wage retry differs from attempt")
			}
		}
		before, err := allocateM2WageCumulative(policy, slices, rate, due, cumulative)
		if err != nil {
			return err
		}
		after, err := allocateM2WageCumulative(policy, slices, rate, due, cumulative+payload.Paid)
		if err != nil {
			return err
		}
		expected := make([]m2WageReceipt, 0, len(slices))
		for index, slice := range slices {
			if after[index] > before[index] {
				expected = append(expected, m2WageReceipt{slice.kind, slice.claimant, after[index] - before[index]})
			}
		}
		actual := make([]m2WageReceipt, 0, len(expected))
		receiptRows, err := conn.QueryContext(ctx, `SELECT claimant_kind, claimant_id, amount_minor, event_sequence FROM m2_wage_split_receipts WHERE obligation_id = ? AND event_id = ? ORDER BY CASE claimant_kind WHEN 'cohort' THEN 0 ELSE 1 END, claimant_id`, obligationID, event.ID)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "read split wage event receipts", err)
		}
		for receiptRows.Next() {
			var r m2WageReceipt
			var sequence int64
			if err := receiptRows.Scan(&r.Kind, &r.ID, &r.Amount, &sequence); err != nil {
				receiptRows.Close()
				return core.WrapError(core.CodeStorageFailure, "scan split wage event receipt", err)
			}
			if sequence != event.Sequence {
				receiptRows.Close()
				return core.NewError(core.CodeProjectionDiverged, "split wage receipt sequence differs from event")
			}
			actual = append(actual, r)
		}
		if err := receiptRows.Err(); err != nil {
			receiptRows.Close()
			return core.WrapError(core.CodeStorageFailure, "iterate split wage event receipts", err)
		}
		if err := receiptRows.Close(); err != nil {
			return core.WrapError(core.CodeStorageFailure, "close split wage event receipts", err)
		}
		if len(actual) != len(expected) {
			return core.NewError(core.CodeProjectionDiverged, "split wage recipient count differs from policy")
		}
		for i := range expected {
			if actual[i] != expected[i] {
				return core.NewError(core.CodeProjectionDiverged, "split wage recipient differs from policy")
			}
		}
		actualHash, err := core.HashJSON(actual)
		if err != nil {
			return err
		}
		if !legacyFull && (payload.ReceiptCount != len(actual) || payload.ReceiptHash != actualHash) {
			return core.NewError(core.CodeProjectionDiverged, "split wage receipt hash differs from event")
		}
		cumulative += payload.Paid
		receiptTotal += payload.Paid
		receiptCount += int64(len(actual))
	}
	if !seenDue || cumulative != recorded {
		return core.NewError(core.CodeProjectionDiverged, "split wage payment history differs from obligation")
	}
	var allCount, allTotal int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(amount_minor), 0) FROM m2_wage_split_receipts WHERE obligation_id = ?`, obligationID).Scan(&allCount, &allTotal); err != nil {
		return core.WrapError(core.CodeStorageFailure, "count all split wage receipts", err)
	}
	if allCount != receiptCount || allTotal != receiptTotal {
		return core.NewError(core.CodeProjectionDiverged, "orphan or duplicate split wage receipt")
	}
	return nil
}

func loadM2WageObligationSlices(ctx context.Context, conn *sql.Conn, obligationID string) ([]m2WageSlice, error) {
	rows, err := conn.QueryContext(ctx, `SELECT s.claimant_kind, s.claimant_id, s.asset_account_id, s.receivable_account_id, s.income_account_id, s.due_minor, a.owner_id, a.account_type, r.owner_id, r.account_type, i.owner_id, i.account_type FROM m2_wage_split_obligations s JOIN accounts a ON a.account_id = s.asset_account_id JOIN accounts r ON r.account_id = s.receivable_account_id JOIN accounts i ON i.account_id = s.income_account_id WHERE s.obligation_id = ? ORDER BY CASE s.claimant_kind WHEN 'cohort' THEN 0 ELSE 1 END, s.claimant_id`, obligationID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "load split wage obligation slices", err)
	}
	slices := []m2WageSlice{}
	for rows.Next() {
		var slice m2WageSlice
		var assetOwner, assetType, receivableOwner, receivableType, incomeOwner, incomeType string
		if err := rows.Scan(&slice.kind, &slice.claimant, &slice.asset, &slice.receivable, &slice.income, &slice.due, &assetOwner, &assetType, &receivableOwner, &receivableType, &incomeOwner, &incomeType); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan split wage obligation slice", err)
		}
		if slice.claimant != assetOwner || slice.claimant != receivableOwner || slice.claimant != incomeOwner || assetType != "asset" || receivableType != "receivable" || incomeType != "income" {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "split wage slice account ownership differs")
		}
		slices = append(slices, slice)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, core.WrapError(core.CodeStorageFailure, "iterate split wage obligation slices", err)
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close split wage obligation slices", err)
	}
	return slices, nil
}

func prepareM2SplitWage(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload, workers, rate int64, currency, employer string) (scheduledMutation, error) {
	obligationID := fmt.Sprintf("obligation_m2_wage_day_%d", payload.Day)
	rows, err := conn.QueryContext(ctx, `SELECT s.materialization_id, s.cohort_id, s.entity_id, s.worker_count, s.effective_from, e.asset_account_id, e.receivable_account_id, s.income_account_id, v.payload
		FROM m2_wage_participation_splits s JOIN materialized_entities e ON e.entity_id = s.entity_id AND e.materialization_id = s.materialization_id
		JOIN cohort_materializations m ON m.materialization_id = s.materialization_id AND m.materialize_event_id = s.split_event_id AND m.materialize_sequence = s.split_event_sequence AND m.status = 'active'
		JOIN events v ON v.event_id = s.split_event_id AND v.event_sequence = s.split_event_sequence AND v.event_type = 'CohortMaterialized' AND v.instance_id = ? AND v.branch_id = ?
		WHERE s.contract_id = ? AND s.effective_from <= ? AND e.status = 'active' ORDER BY s.entity_id`, M2DemoInstanceID, M2DemoBranchID, m2EconomyContractID, m2WageTime(payload.Day, 7, 0))
	if err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "load wage participants", err)
	}
	slices := []m2WageSlice{}
	for rows.Next() {
		var s m2WageSlice
		var materializationID, cohortID, effectiveFrom, eventJSON string
		var count int64
		if err := rows.Scan(&materializationID, &cohortID, &s.claimant, &count, &effectiveFrom, &s.asset, &s.receivable, &s.income, &eventJSON); err != nil {
			rows.Close()
			return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "scan wage participant", err)
		}
		var event materializationEventPayload
		if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
			rows.Close()
			return scheduledMutation{}, core.WrapError(core.CodeProjectionDiverged, "decode wage split event", err)
		}
		expectedHash, err := core.HashJSON(m2WageParticipation{m2EconomyContractID, effectiveFrom, 1})
		if err != nil {
			rows.Close()
			return scheduledMutation{}, err
		}
		if cohortID != M2DemoCohortID || count != 1 || event.MaterializationID != materializationID || event.SourceCohortID != cohortID || event.EntityID != s.claimant || event.PopulationCount != 1 || event.WageParticipationHash != expectedHash {
			rows.Close()
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "named wage participation lacks materialization authority")
		}
		s.kind, s.due = "entity", rate
		slices = append(slices, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "iterate wage participants", err)
	}
	if err := rows.Close(); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "close wage participants", err)
	}
	var population int64
	if err := conn.QueryRowContext(ctx, `SELECT population_count FROM cohorts WHERE cohort_id = ?`, M2DemoCohortID).Scan(&population); err != nil {
		return scheduledMutation{}, classifyMissing(err, "wage Cohort")
	}
	if population <= 0 || population+int64(len(slices)) != workers {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "split worker count differs from wage contract")
	}
	var totalSplits int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_participation_splits s JOIN cohort_materializations m ON m.materialization_id = s.materialization_id AND m.status = 'active' WHERE s.contract_id = ? AND s.effective_from <= ?`, m2EconomyContractID, m2WageTime(payload.Day, 7, 0)).Scan(&totalSplits); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "count recorded wage participation", err)
	}
	if totalSplits != int64(len(slices)) {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "wage participation lineage is incomplete")
	}
	cohortDue, ok := checkedMultiplyPositive(population, rate)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "Cohort wage slice overflows")
	}
	slices = append([]m2WageSlice{{"cohort", M2DemoCohortID, M2DemoCohortAssetAccountID, M2DemoCohortReceivableID, m2EconomyCohortIncome, cohortDue}}, slices...)
	amount, ok := checkedMultiplyPositive(workers, rate)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "aggregate wage overflows")
	}
	policy, err := readM2WageAllocationPolicy(ctx, conn, item.WorldTime)
	if err != nil {
		return scheduledMutation{}, err
	}
	for _, id := range []string{employer, m2EconomyEmployerExpense, m2EconomyEmployerPayable} {
		if err := verifyM2AccountProjection(ctx, conn, id, currency); err != nil {
			return scheduledMutation{}, err
		}
	}
	for _, slice := range slices {
		for _, id := range []string{slice.asset, slice.receivable, slice.income} {
			if err := verifyM2AccountProjection(ctx, conn, id, currency); err != nil {
				return scheduledMutation{}, err
			}
		}
	}
	postings := []scheduledPosting{}
	balances := []balanceMutation{}
	add := func(id string, delta int64, memo string) error {
		old, version, err := readScheduledBalance(ctx, conn, id)
		if err != nil {
			return err
		}
		newValue, ok := checkedAdd(old, delta)
		if !ok {
			return core.NewError(core.CodeIntegerOverflow, "split wage account overflows")
		}
		postings = append(postings, scheduledPosting{id, currency, delta, memo})
		balances = append(balances, balanceMutation{id, version, newValue})
		return nil
	}
	if item.PhaseID == m2EconomyPhaseAccrue {
		if err := add(m2EconomyEmployerExpense, amount, "M2 split wage expense"); err != nil {
			return scheduledMutation{}, err
		}
		if err := add(m2EconomyEmployerPayable, -amount, "M2 split wage payable"); err != nil {
			return scheduledMutation{}, err
		}
		for _, slice := range slices {
			if err := add(slice.receivable, slice.due, "M2 split wage receivable"); err != nil {
				return scheduledMutation{}, err
			}
			if err := add(slice.income, -slice.due, "M2 split wage income"); err != nil {
				return scheduledMutation{}, err
			}
		}
		return scheduledMutation{EventType: "M2WageAccrued", EventPayload: struct {
			ContractID string `json:"contract_id"`
			Workers    int64  `json:"worker_count"`
			Rate       int64  `json:"unit_wage_minor"`
			Amount     int64  `json:"amount_minor"`
			Split      bool   `json:"split"`
			Policy     string `json:"allocation_policy"`
		}{m2EconomyContractID, workers, rate, amount, true, policy}, Postings: postings, Balances: balances, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
			if err := execAgentOne(ctx, conn, "record aggregate split wage obligation", `INSERT INTO m2_economic_obligations(obligation_id, contract_id, period_start, period_end, kind, amount_due_minor, status, defining_event_id, last_event_sequence) VALUES (?, ?, ?, ?, 'wage', ?, 'accrued', ?, ?)`, obligationID, m2EconomyContractID, m2WageTime(payload.Day-1, 7, 0), item.WorldTime, amount, eventID, sequence); err != nil {
				return err
			}
			for _, slice := range slices {
				if err := execAgentOne(ctx, conn, "record wage claimant slice", `INSERT INTO m2_wage_split_obligations(obligation_id, claimant_kind, claimant_id, due_minor, asset_account_id, receivable_account_id, income_account_id, accrual_event_id, accrual_event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, obligationID, slice.kind, slice.claimant, slice.due, slice.asset, slice.receivable, slice.income, eventID, sequence); err != nil {
					return err
				}
			}
			return nil
		}}, nil
	}
	var due, paid, sliceDue, recordedReceipts int64
	if err := conn.QueryRowContext(ctx, `SELECT amount_due_minor, amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = ?`, obligationID).Scan(&due, &paid); err != nil {
		return scheduledMutation{}, classifyMissing(err, "split wage obligation")
	}
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(due_minor), 0) FROM m2_wage_split_obligations WHERE obligation_id = ?`, obligationID).Scan(&sliceDue); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "sum wage slices", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_split_receipts WHERE obligation_id = ?`, obligationID).Scan(&recordedReceipts); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "sum wage receipts", err)
	}
	if due != amount || paid != 0 || sliceDue != due || recordedReceipts != 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "split wage obligation differs from accrual")
	}
	var actualSlices int64
	for _, slice := range slices {
		var recorded int64
		if err := conn.QueryRowContext(ctx, `SELECT due_minor FROM m2_wage_split_obligations WHERE obligation_id = ? AND claimant_kind = ? AND claimant_id = ? AND asset_account_id = ? AND receivable_account_id = ? AND income_account_id = ?`, obligationID, slice.kind, slice.claimant, slice.asset, slice.receivable, slice.income).Scan(&recorded); err != nil {
			return scheduledMutation{}, classifyMissing(err, "wage claimant slice")
		}
		if recorded != slice.due {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "wage slice amount differs from contract")
		}
		actualSlices++
	}
	var count int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id = ?`, obligationID).Scan(&count); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "count wage slices", err)
	}
	if count != actualSlices {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "unexpected wage claimant slice")
	}
	available, _, err := readScheduledBalance(ctx, conn, employer)
	if err != nil {
		return scheduledMutation{}, err
	}
	payable, _, err := readScheduledBalance(ctx, conn, m2EconomyEmployerPayable)
	if err != nil {
		return scheduledMutation{}, err
	}
	if available < 0 || payable > -amount {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "split wage employer ledger cannot cover accrued debt")
	}
	for _, slice := range slices {
		asset, _, err := readScheduledBalance(ctx, conn, slice.asset)
		if err != nil {
			return scheduledMutation{}, err
		}
		receivable, _, err := readScheduledBalance(ctx, conn, slice.receivable)
		if err != nil {
			return scheduledMutation{}, err
		}
		if asset < 0 || receivable < slice.due {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "split wage claimant ledger cannot cover accrued debt")
		}
	}
	payment := amount
	if available < payment {
		payment = available
	}
	allocations, err := allocateM2WageCumulative(policy, slices, rate, amount, payment)
	if err != nil {
		return scheduledMutation{}, err
	}
	receipts := make([]m2WageReceipt, 0, len(slices))
	if payment > 0 {
		if err := add(employer, -payment, "M2 split wage paid"); err != nil {
			return scheduledMutation{}, err
		}
		if err := add(m2EconomyEmployerPayable, payment, "M2 split wage payable cleared"); err != nil {
			return scheduledMutation{}, err
		}
		for index, slice := range slices {
			share := allocations[index]
			if share == 0 {
				continue
			}
			if err := add(slice.asset, share, "M2 split wage received"); err != nil {
				return scheduledMutation{}, err
			}
			if err := add(slice.receivable, -share, "M2 split wage receivable cleared"); err != nil {
				return scheduledMutation{}, err
			}
			receipts = append(receipts, m2WageReceipt{slice.kind, slice.claimant, share})
		}
	}
	receiptHash, err := core.HashJSON(receipts)
	if err != nil {
		return scheduledMutation{}, err
	}
	status, reason := "paid", ""
	if payment < amount {
		status, reason = "partial", "insufficient_employer_liquidity"
	}
	if payment == 0 {
		status = "overdue"
	}
	return scheduledMutation{EventType: "M2WageSettled", EventPayload: struct {
		ObligationID string `json:"obligation_id"`
		Status       string `json:"status"`
		Reason       string `json:"reason_code,omitempty"`
		Due          int64  `json:"due_minor"`
		Paid         int64  `json:"paid_minor"`
		Remaining    int64  `json:"remaining_minor"`
		Split        bool   `json:"split"`
		Policy       string `json:"allocation_policy"`
		ReceiptCount int    `json:"receipt_count"`
		ReceiptHash  string `json:"receipt_hash"`
	}{obligationID, status, reason, amount, payment, amount - payment, true, policy, len(receipts), receiptHash}, Postings: postings, Balances: balances, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "settle aggregate split wage", `UPDATE m2_economic_obligations SET amount_paid_minor = ?, status = ?, last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor = 0 AND status = 'accrued'`, payment, status, sequence, obligationID); err != nil {
			return err
		}
		for _, receipt := range receipts {
			if err := execAgentOne(ctx, conn, "record wage claimant receipt", `INSERT INTO m2_wage_split_receipts(scheduler_item_id, obligation_id, claimant_kind, claimant_id, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, obligationID, receipt.Kind, receipt.ID, receipt.Amount, eventID, sequence); err != nil {
				return err
			}
		}
		if payment < amount {
			return recordM2ArrearsCase(ctx, conn, obligationID, m2EconomyContractID, "wage", payload.Day, 3, eventID, sequence)
		}
		return nil
	}}, nil
}

func prepareM2SplitWageArrearsRetry(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload, obligationID string, due, paid int64, caseStatus, graceExpires string) (scheduledMutation, error) {
	if err := verifyM2ObligationPaid(ctx, conn, obligationID, "wage", paid); err != nil {
		return scheduledMutation{}, err
	}
	slices, err := loadM2WageObligationSlices(ctx, conn, obligationID)
	if err != nil {
		return scheduledMutation{}, err
	}
	if len(slices) < 2 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "split wage is missing a named claimant")
	}
	var rate int64
	var periodEnd string
	if err := conn.QueryRowContext(ctx, `SELECT c.unit_rate_minor, o.period_end FROM m2_economic_obligations o JOIN m2_cohort_contracts c ON c.contract_id = o.contract_id WHERE o.obligation_id = ? AND o.contract_id = ?`, obligationID, m2EconomyContractID).Scan(&rate, &periodEnd); err != nil {
		return scheduledMutation{}, classifyMissing(err, "split wage contract for arrears")
	}
	policy, err := readM2WageAllocationPolicy(ctx, conn, periodEnd)
	if err != nil {
		return scheduledMutation{}, err
	}
	before, err := allocateM2WageCumulative(policy, slices, rate, due, paid)
	if err != nil {
		return scheduledMutation{}, err
	}
	for _, accountID := range []string{m2EconomyEmployerCash, m2EconomyEmployerPayable} {
		if err := verifyM2AccountProjection(ctx, conn, accountID, M2DemoCurrencyID); err != nil {
			return scheduledMutation{}, err
		}
	}
	for _, slice := range slices {
		for _, accountID := range []string{slice.asset, slice.receivable} {
			if err := verifyM2AccountProjection(ctx, conn, accountID, M2DemoCurrencyID); err != nil {
				return scheduledMutation{}, err
			}
		}
	}
	available, employerVersion, err := readScheduledBalance(ctx, conn, m2EconomyEmployerCash)
	if err != nil {
		return scheduledMutation{}, err
	}
	payable, payableVersion, err := readScheduledBalance(ctx, conn, m2EconomyEmployerPayable)
	if err != nil {
		return scheduledMutation{}, err
	}
	remaining := due - paid
	if available < 0 || payable > -remaining {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "split wage employer arrears ledger is inconsistent")
	}
	for index, slice := range slices {
		asset, _, err := readScheduledBalance(ctx, conn, slice.asset)
		if err != nil {
			return scheduledMutation{}, err
		}
		receivable, _, err := readScheduledBalance(ctx, conn, slice.receivable)
		if err != nil {
			return scheduledMutation{}, err
		}
		if asset < 0 || receivable < slice.due-before[index] {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "split wage claimant arrears ledger is inconsistent")
		}
	}
	payment := remaining
	if available < payment {
		payment = available
	}
	after, err := allocateM2WageCumulative(policy, slices, rate, due, paid+payment)
	if err != nil {
		return scheduledMutation{}, err
	}
	status, reason := "paid", "none"
	if payment == 0 {
		status, reason = "deferred", "insufficient_liquidity"
	} else if payment < remaining {
		status = "partial"
	}
	postings := []scheduledPosting{}
	balances := []balanceMutation{}
	receipts := make([]m2WageReceipt, 0, len(slices))
	if payment > 0 {
		postings = append(postings, scheduledPosting{m2EconomyEmployerCash, M2DemoCurrencyID, -payment, "M2 split wage arrears paid"}, scheduledPosting{m2EconomyEmployerPayable, M2DemoCurrencyID, payment, "M2 split wage payable cleared"})
		balances = append(balances, balanceMutation{m2EconomyEmployerCash, employerVersion, available - payment}, balanceMutation{m2EconomyEmployerPayable, payableVersion, payable + payment})
		for index, slice := range slices {
			share := after[index] - before[index]
			if share == 0 {
				continue
			}
			asset, assetVersion, err := readScheduledBalance(ctx, conn, slice.asset)
			if err != nil {
				return scheduledMutation{}, err
			}
			receivable, receivableVersion, err := readScheduledBalance(ctx, conn, slice.receivable)
			if err != nil {
				return scheduledMutation{}, err
			}
			newAsset, assetOK := checkedAdd(asset, share)
			newReceivable, receivableOK := checkedSubtract(receivable, share)
			if !assetOK || !receivableOK || newReceivable < 0 {
				return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "split wage arrears payment exceeds claimant ledger")
			}
			postings = append(postings, scheduledPosting{slice.asset, M2DemoCurrencyID, share, "M2 split wage arrears received"}, scheduledPosting{slice.receivable, M2DemoCurrencyID, -share, "M2 split wage receivable cleared"})
			balances = append(balances, balanceMutation{slice.asset, assetVersion, newAsset}, balanceMutation{slice.receivable, receivableVersion, newReceivable})
			receipts = append(receipts, m2WageReceipt{slice.kind, slice.claimant, share})
		}
	}
	receiptHash, err := core.HashJSON(receipts)
	if err != nil {
		return scheduledMutation{}, err
	}
	return scheduledMutation{EventType: "M2ArrearsRetried", EventPayload: struct {
		ContractID   string `json:"contract_id"`
		ObligationID string `json:"obligation_id"`
		Day          int    `json:"day"`
		Status       string `json:"status"`
		Reason       string `json:"reason_code"`
		PreviousPaid int64  `json:"previously_paid_minor"`
		Paid         int64  `json:"paid_minor"`
		Remaining    int64  `json:"remaining_minor"`
		GraceExpires string `json:"grace_expires_at"`
		Split        bool   `json:"split"`
		Policy       string `json:"allocation_policy"`
		ReceiptCount int    `json:"receipt_count"`
		ReceiptHash  string `json:"receipt_hash"`
	}{m2EconomyContractID, obligationID, payload.Day, status, reason, paid, payment, remaining - payment, graceExpires, true, policy, len(receipts), receiptHash}, Postings: postings, Balances: balances, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "record split wage arrears retry", `INSERT INTO m2_arrears_attempts(scheduler_item_id, contract_id, obligation_id, day, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, m2EconomyContractID, obligationID, payload.Day, status, reason, payment, eventID, sequence); err != nil {
			return err
		}
		for _, receipt := range receipts {
			if err := execAgentOne(ctx, conn, "record split wage arrears receipt", `INSERT INTO m2_wage_split_receipts(scheduler_item_id, obligation_id, claimant_kind, claimant_id, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, obligationID, receipt.Kind, receipt.ID, receipt.Amount, eventID, sequence); err != nil {
				return err
			}
		}
		if payment == 0 {
			return nil
		}
		if err := execAgentOne(ctx, conn, "update original split wage after retry", `UPDATE m2_economic_obligations SET amount_paid_minor = ?, status = ?, last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor = ? AND amount_due_minor = ?`, paid+payment, status, sequence, obligationID, paid, due); err != nil {
			return err
		}
		if status == "paid" {
			return execAgentOne(ctx, conn, "cure split wage arrears", `UPDATE m2_arrears_cases SET status = 'cured', last_event_sequence = ? WHERE obligation_id = ? AND status IN ('open', 'grace_expired')`, sequence, obligationID)
		}
		return execAgentOne(ctx, conn, "checkpoint split wage arrears", `UPDATE m2_arrears_cases SET last_event_sequence = ? WHERE obligation_id = ? AND status IN ('open', 'grace_expired')`, sequence, obligationID)
	}}, nil
}
