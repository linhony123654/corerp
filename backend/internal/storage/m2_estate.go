package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type m2EstatePolicy struct {
	Actor, Donor, ContributionAt, DistributionAt, DefinitionEvent, EmployerCash, DonorCash string
	Amount, PerWorker                                                                      int64
}

type m2EstateReceipt struct {
	Kind       string `json:"claimant_kind"`
	ID         string `json:"claimant_id"`
	Asset      string `json:"asset_account_id"`
	Receivable string `json:"receivable_account_id"`
	Amount     int64  `json:"amount_minor"`
}

// A paid obligation must agree with the event's declared recipient set and
// the immutable normalized receipts, not just with an aggregate paid amount.
func verifyM2EstatePaymentFacts(ctx context.Context, conn *sql.Conn, obligationID string) (int64, error) {
	type distribution struct {
		itemID, eventID, policyID, eventType, eventTime, payloadPolicy, payloadObligation, payloadHash, payloadReceipts string
		sequence, amount, payloadAmount, payloadCount                                                                   int64
	}
	rows, err := conn.QueryContext(ctx, `SELECT d.scheduler_item_id, d.event_id, d.event_sequence, d.policy_id, d.amount_minor, e.event_type, e.world_time, json_extract(e.payload, '$.policy_id'), json_extract(e.payload, '$.obligation_id'), json_extract(e.payload, '$.amount_minor'), json_extract(e.payload, '$.receipt_count'), json_extract(e.payload, '$.receipt_hash'), json_extract(e.payload, '$.receipts') FROM m2_estate_distributions d JOIN events e ON e.event_id = d.event_id AND e.event_sequence = d.event_sequence WHERE d.obligation_id = ? AND d.status = 'paid' ORDER BY d.event_sequence`, obligationID)
	if err != nil {
		return 0, core.WrapError(core.CodeStorageFailure, "read M2 estate distributions", err)
	}
	distributions := make([]distribution, 0, 1)
	for rows.Next() {
		var d distribution
		if err := rows.Scan(&d.itemID, &d.eventID, &d.sequence, &d.policyID, &d.amount, &d.eventType, &d.eventTime, &d.payloadPolicy, &d.payloadObligation, &d.payloadAmount, &d.payloadCount, &d.payloadHash, &d.payloadReceipts); err != nil {
			rows.Close()
			return 0, core.WrapError(core.CodeStorageFailure, "scan M2 estate distribution", err)
		}
		distributions = append(distributions, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, core.WrapError(core.CodeStorageFailure, "iterate M2 estate distributions", err)
	}
	if err := rows.Close(); err != nil {
		return 0, core.WrapError(core.CodeStorageFailure, "close M2 estate distributions", err)
	}
	var total int64
	for _, d := range distributions {
		if d.itemID != "sched_m2_estate_distribution_day_31" || d.policyID != m2EstatePolicyID || d.eventType != "M2EstateWageDistributed" || d.eventTime != m2WageTime(31, 7, 1) || d.amount <= 0 || d.payloadPolicy != d.policyID || d.payloadObligation != obligationID || d.payloadAmount != d.amount || d.payloadCount <= 0 {
			return 0, core.NewError(core.CodeProjectionDiverged, "M2 estate distribution differs from its event")
		}
		var declared []m2EstateReceipt
		if err := json.Unmarshal([]byte(d.payloadReceipts), &declared); err != nil {
			return 0, core.WrapError(core.CodeProjectionDiverged, "decode M2 estate receipt event", err)
		}
		declaredHash, err := core.HashJSON(declared)
		if err != nil {
			return 0, err
		}
		if declaredHash != d.payloadHash || int64(len(declared)) != d.payloadCount {
			return 0, core.NewError(core.CodeProjectionDiverged, "M2 estate event recipient hash differs")
		}
		receiptRows, err := conn.QueryContext(ctx, `SELECT r.claimant_kind, r.claimant_id, r.asset_account_id, r.receivable_account_id, r.amount_minor, r.event_id, r.event_sequence, asset.owner_id, asset.account_type, due.owner_id, due.account_type FROM m2_estate_distribution_receipts r JOIN accounts asset ON asset.account_id = r.asset_account_id JOIN accounts due ON due.account_id = r.receivable_account_id WHERE r.scheduler_item_id = ? ORDER BY r.claimant_kind, r.claimant_id`, d.itemID)
		if err != nil {
			return 0, core.WrapError(core.CodeStorageFailure, "read M2 estate recipient facts", err)
		}
		receipts := make([]m2EstateReceipt, 0, len(declared))
		var receiptTotal int64
		for receiptRows.Next() {
			var r m2EstateReceipt
			var receiptEvent, assetOwner, assetType, dueOwner, dueType string
			var receiptSequence int64
			if err := receiptRows.Scan(&r.Kind, &r.ID, &r.Asset, &r.Receivable, &r.Amount, &receiptEvent, &receiptSequence, &assetOwner, &assetType, &dueOwner, &dueType); err != nil {
				receiptRows.Close()
				return 0, core.WrapError(core.CodeStorageFailure, "scan M2 estate recipient fact", err)
			}
			if (r.Kind != "cohort" && r.Kind != "entity") || r.ID != assetOwner || r.ID != dueOwner || assetType != "asset" || dueType != "receivable" || receiptEvent != d.eventID || receiptSequence != d.sequence || r.Amount <= 0 {
				receiptRows.Close()
				return 0, core.NewError(core.CodeProjectionDiverged, "M2 estate recipient differs from event or owner")
			}
			var ok bool
			receiptTotal, ok = checkedAdd(receiptTotal, r.Amount)
			if !ok {
				receiptRows.Close()
				return 0, core.NewError(core.CodeIntegerOverflow, "M2 estate receipts overflow")
			}
			receipts = append(receipts, r)
		}
		if err := receiptRows.Err(); err != nil {
			receiptRows.Close()
			return 0, core.WrapError(core.CodeStorageFailure, "iterate M2 estate recipient facts", err)
		}
		if err := receiptRows.Close(); err != nil {
			return 0, core.WrapError(core.CodeStorageFailure, "close M2 estate recipient facts", err)
		}
		actualHash, err := core.HashJSON(receipts)
		if err != nil {
			return 0, err
		}
		if receiptTotal != d.amount || int64(len(receipts)) != d.payloadCount || actualHash != d.payloadHash {
			return 0, core.NewError(core.CodeProjectionDiverged, "M2 estate receipts differ from distribution event")
		}
		var ok bool
		total, ok = checkedAdd(total, d.amount)
		if !ok {
			return 0, core.NewError(core.CodeIntegerOverflow, "M2 estate distributions overflow")
		}
	}
	return total, nil
}

func readM2EstatePolicy(ctx context.Context, conn *sql.Conn) (m2EstatePolicy, error) {
	var p m2EstatePolicy
	err := conn.QueryRowContext(ctx, `SELECT p.actor_id, p.donor_actor_id, p.contribution_minor, p.contribution_at, p.distribution_at, p.per_worker_minor, p.definition_event_id, employer.cash_account_id, donor.cash_account_id FROM m2_estate_policies p JOIN m2_economic_actors employer ON employer.actor_id = p.actor_id JOIN m2_economic_actors donor ON donor.actor_id = p.donor_actor_id WHERE p.policy_id = ? AND employer.instance_id = ? AND employer.branch_id = ? AND donor.instance_id = employer.instance_id AND donor.branch_id = employer.branch_id AND employer.kind = 'employer' AND donor.kind = 'landlord'`, m2EstatePolicyID, M2DemoInstanceID, M2DemoBranchID).Scan(&p.Actor, &p.Donor, &p.Amount, &p.ContributionAt, &p.DistributionAt, &p.PerWorker, &p.DefinitionEvent, &p.EmployerCash, &p.DonorCash)
	if err != nil {
		return p, classifyMissing(err, "M2 estate policy")
	}
	if p.Actor != "actor_m2_coop_employer" || p.Donor != "actor_m2_landlord" || p.Amount != 18 || p.PerWorker != 1 || p.ContributionAt != m2WageTime(31, 7, 0) || p.DistributionAt != m2WageTime(31, 7, 1) || p.DefinitionEvent != m2EconomyEventID || p.EmployerCash != m2EconomyEmployerCash || p.DonorCash != m2EconomyLandlordCash {
		return p, core.NewError(core.CodeProjectionDiverged, "M2 estate policy differs from declared fixture")
	}
	return p, nil
}

func m2EstateAccount(ctx context.Context, conn *sql.Conn, id string, delta int64) (balanceMutation, error) {
	if err := verifyM2AccountProjection(ctx, conn, id, M2DemoCurrencyID); err != nil {
		return balanceMutation{}, err
	}
	balance, version, err := readScheduledBalance(ctx, conn, id)
	if err != nil {
		return balanceMutation{}, err
	}
	next, ok := checkedAdd(balance, delta)
	if !ok {
		return balanceMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 estate account balance overflows")
	}
	return balanceMutation{id, version, next}, nil
}

func prepareM2EstateContribution(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if item.PhaseID != m2EstateContributionPhase || item.SchedulerItemID != "sched_m2_estate_contribution_day_31" || item.WorldTime != m2WageTime(31, 7, 0) || payload.Kind != "m2_estate_contribution" || payload.Day != 31 || payload.SubjectID != m2EstatePolicyID {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 estate contribution task")
	}
	p, err := readM2EstatePolicy(ctx, conn)
	if err != nil {
		return scheduledMutation{}, err
	}
	if err := verifyM2BankruptcyClaims(ctx, conn, p.Actor); err != nil {
		return scheduledMutation{}, err
	}
	donor, err := m2EstateAccount(ctx, conn, p.DonorCash, -p.Amount)
	if err != nil {
		return scheduledMutation{}, err
	}
	estate, err := m2EstateAccount(ctx, conn, p.EmployerCash, p.Amount)
	if err != nil {
		return scheduledMutation{}, err
	}
	if donor.NewBalance < 0 || estate.NewBalance < p.Amount {
		return scheduledMutation{EventType: "M2EstateContributionDeferred", EventPayload: struct {
			PolicyID, Reason string
			Amount           int64
		}{m2EstatePolicyID, "insufficient_donor_cash", 0}, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
			return execAgentOne(ctx, conn, "record unfunded M2 estate contribution", `INSERT INTO m2_estate_contributions(scheduler_item_id, policy_id, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, 'deferred', 'insufficient_donor_cash', 0, ?, ?)`, item.SchedulerItemID, m2EstatePolicyID, eventID, sequence)
		}}, nil
	}
	return scheduledMutation{EventType: "M2EstateContributionPaid", EventPayload: struct {
		PolicyID, DonorID, ActorID, Reason string
		Amount                             int64
	}{m2EstatePolicyID, p.Donor, p.Actor, "voluntary_no_recourse", p.Amount},
		Postings: []scheduledPosting{{p.DonorCash, M2DemoCurrencyID, -p.Amount, "voluntary no-recourse estate contribution"}, {p.EmployerCash, M2DemoCurrencyID, p.Amount, "estate cash received"}},
		Balances: []balanceMutation{donor, estate},
		ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
			return execAgentOne(ctx, conn, "record M2 estate contribution", `INSERT INTO m2_estate_contributions(scheduler_item_id, policy_id, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, 'paid', 'voluntary_no_recourse', ?, ?, ?)`, item.SchedulerItemID, m2EstatePolicyID, p.Amount, eventID, sequence)
		},
	}, nil
}

func prepareM2EstateDistribution(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if item.PhaseID != m2EstateDistributionPhase || item.SchedulerItemID != "sched_m2_estate_distribution_day_31" || item.WorldTime != m2WageTime(31, 7, 1) || payload.Kind != "m2_estate_distribution" || payload.Day != 31 || payload.SubjectID != m2EstatePolicyID {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 estate distribution task")
	}
	p, err := readM2EstatePolicy(ctx, conn)
	if err != nil {
		return scheduledMutation{}, err
	}
	if err := verifyM2BankruptcyClaims(ctx, conn, p.Actor); err != nil {
		return scheduledMutation{}, err
	}
	var contributionStatus, contributionEvent string
	var contributionAmount, contributionSequence int64
	err = conn.QueryRowContext(ctx, `SELECT c.status, c.amount_minor, c.event_id, c.event_sequence FROM m2_estate_contributions c JOIN events e ON e.event_id = c.event_id AND e.event_sequence = c.event_sequence WHERE c.scheduler_item_id = 'sched_m2_estate_contribution_day_31' AND c.policy_id = ?`, m2EstatePolicyID).Scan(&contributionStatus, &contributionAmount, &contributionEvent, &contributionSequence)
	if err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 estate contribution authority")
	}
	if contributionStatus != "paid" && contributionStatus != "deferred" {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "invalid M2 estate contribution state")
	}
	if contributionStatus == "paid" && contributionAmount != p.Amount || contributionStatus == "deferred" && contributionAmount != 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 estate contribution differs from policy")
	}
	var slotClaimCount int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_bankruptcy_slot_claims c JOIN m2_bankruptcy_proceedings p ON p.proceeding_id = c.proceeding_id WHERE p.actor_id = ?`, p.Actor).Scan(&slotClaimCount); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "check estate slot claimant routing", err)
	}
	if slotClaimCount > 0 {
		return scheduledMutation{EventType: "M2EstateDistributionDeferred", EventPayload: struct {
			PolicyID string `json:"policy_id"`
			Reason   string `json:"reason_code"`
			Amount   int64  `json:"amount_minor"`
		}{m2EstatePolicyID, "slot_claim_liquidation_deferred", 0}, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
			return execAgentOne(ctx, conn, "defer estate distribution for named creditors", `INSERT INTO m2_estate_distributions(scheduler_item_id, policy_id, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, 'deferred', 'slot_claim_liquidation_deferred', 0, ?, ?)`, item.SchedulerItemID, m2EstatePolicyID, eventID, sequence)
		}}, nil
	}
	estate, err := m2EstateAccount(ctx, conn, p.EmployerCash, -p.Amount)
	if err != nil {
		return scheduledMutation{}, err
	}
	if estate.NewBalance < 0 || contributionStatus != "paid" {
		reason := "insufficient_estate_cash"
		if contributionStatus != "paid" {
			reason = "contribution_deferred"
		}
		return scheduledMutation{EventType: "M2EstateDistributionDeferred", EventPayload: struct {
			PolicyID, Reason string
			Amount           int64
		}{m2EstatePolicyID, reason, 0},
			ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
				return execAgentOne(ctx, conn, "record unfunded M2 estate distribution", `INSERT INTO m2_estate_distributions(scheduler_item_id, policy_id, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, 'deferred', ?, 0, ?, ?)`, item.SchedulerItemID, m2EstatePolicyID, reason, eventID, sequence)
			},
		}, nil
	}
	var obligationID, cohortID, currency, caseStatus string
	var due, paid, participants, openingOutstanding int64
	err = conn.QueryRowContext(ctx, `SELECT o.obligation_id, k.cohort_id, k.currency_id, o.amount_due_minor, o.amount_paid_minor, k.participant_count, c.outstanding_at_open_minor, a.status FROM m2_economic_obligations o JOIN m2_cohort_contracts k ON k.contract_id = o.contract_id JOIN m2_bankruptcy_claims c ON c.obligation_id = o.obligation_id JOIN m2_arrears_cases a ON a.obligation_id = o.obligation_id WHERE k.actor_id = ? AND k.kind = 'wage' AND o.kind = 'wage' AND o.amount_paid_minor < o.amount_due_minor AND o.period_end < ? ORDER BY o.period_end, o.obligation_id LIMIT 1`, p.Actor, item.WorldTime).Scan(&obligationID, &cohortID, &currency, &due, &paid, &participants, &openingOutstanding, &caseStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "declared M2 estate wage claim is absent")
	}
	if err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "select oldest M2 estate wage claim", err)
	}
	if cohortID != M2DemoCohortID || currency != M2DemoCurrencyID || participants != 18 || p.Amount != participants*p.PerWorker || due-paid < p.Amount || openingOutstanding < p.Amount || caseStatus != "grace_expired" {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 estate claim is inconsistent with declared distribution")
	}
	if err := verifyM2ObligationPaid(ctx, conn, obligationID, "wage", paid); err != nil {
		return scheduledMutation{}, err
	}
	if err := verifyM2ClaimAllocationLineagesForCohort(ctx, conn, cohortID); err != nil {
		return scheduledMutation{}, err
	}
	var population int64
	if err := conn.QueryRowContext(ctx, `SELECT population_count FROM cohorts WHERE cohort_id = ? AND status = 'active'`, cohortID).Scan(&population); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 estate Cohort")
	}
	receipts := make([]m2EstateReceipt, 0, 2)
	rows, err := conn.QueryContext(ctx, `SELECT a.entity_id, a.amount_minor, e.population_count, e.asset_account_id, e.receivable_account_id FROM m2_bankruptcy_claim_allocations a JOIN cohort_materializations m ON m.materialization_id = a.materialization_id AND m.status = 'active' JOIN materialized_entities e ON e.entity_id = a.entity_id AND e.status = 'active' LEFT JOIN m2_bankruptcy_claim_returns r ON r.materialization_id = a.materialization_id AND r.obligation_id = a.obligation_id WHERE a.obligation_id = ? AND r.materialization_id IS NULL ORDER BY a.entity_id`, obligationID)
	if err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "read M2 estate claimants", err)
	}
	var assignedWorkers int64
	for rows.Next() {
		var receipt m2EstateReceipt
		var assigned, workers int64
		if err := rows.Scan(&receipt.ID, &assigned, &workers, &receipt.Asset, &receipt.Receivable); err != nil {
			rows.Close()
			return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "scan M2 estate claimant", err)
		}
		if workers <= 0 || assigned < workers*p.PerWorker {
			rows.Close()
			return scheduledMutation{}, core.NewError(core.CodeConservationFailed, "M2 estate assignment cannot cover worker share")
		}
		receipt.Kind, receipt.Amount = "entity", workers*p.PerWorker
		assignedWorkers += workers
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "iterate M2 estate claimants", err)
	}
	if err := rows.Close(); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "close M2 estate claimants", err)
	}
	if population <= 0 || population+assignedWorkers != participants {
		return scheduledMutation{}, core.NewError(core.CodeConservationFailed, "M2 estate claimant population differs from wage contract")
	}
	receipts = append(receipts, m2EstateReceipt{"cohort", cohortID, M2DemoCohortAssetAccountID, M2DemoCohortReceivableID, population * p.PerWorker})
	sort.Slice(receipts, func(i, j int) bool {
		if receipts[i].Kind != receipts[j].Kind {
			return receipts[i].Kind < receipts[j].Kind
		}
		return receipts[i].ID < receipts[j].ID
	})
	receiptHash, err := core.HashJSON(receipts)
	if err != nil {
		return scheduledMutation{}, err
	}
	payable, err := m2EstateAccount(ctx, conn, m2EconomyEmployerPayable, p.Amount)
	if err != nil {
		return scheduledMutation{}, err
	}
	if payable.NewBalance > 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 estate payable lacks wage debt")
	}
	mutation := scheduledMutation{EventType: "M2EstateWageDistributed", EventPayload: struct {
		PolicyID            string            `json:"policy_id"`
		ObligationID        string            `json:"obligation_id"`
		ContributionEventID string            `json:"contribution_event_id"`
		Amount              int64             `json:"amount_minor"`
		ReceiptCount        int               `json:"receipt_count"`
		ReceiptHash         string            `json:"receipt_hash"`
		Receipts            []m2EstateReceipt `json:"receipts"`
	}{m2EstatePolicyID, obligationID, contributionEvent, p.Amount, len(receipts), receiptHash, receipts},
		Postings: []scheduledPosting{{p.EmployerCash, currency, -p.Amount, "estate wage cash distributed"}, {m2EconomyEmployerPayable, currency, p.Amount, "estate wage payable cleared"}},
		Balances: []balanceMutation{estate, payable},
	}
	var receiptTotal int64
	for _, receipt := range receipts {
		if receipt.Amount <= 0 {
			return scheduledMutation{}, core.NewError(core.CodeConservationFailed, "M2 estate receipt is empty")
		}
		asset, err := m2EstateAccount(ctx, conn, receipt.Asset, receipt.Amount)
		if err != nil {
			return scheduledMutation{}, err
		}
		receivable, err := m2EstateAccount(ctx, conn, receipt.Receivable, -receipt.Amount)
		if err != nil {
			return scheduledMutation{}, err
		}
		if receivable.NewBalance < 0 {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 estate claimant lacks receivable")
		}
		mutation.Balances = append(mutation.Balances, asset, receivable)
		mutation.Postings = append(mutation.Postings, scheduledPosting{receipt.Asset, currency, receipt.Amount, "estate wage received"}, scheduledPosting{receipt.Receivable, currency, -receipt.Amount, "estate wage receivable cleared"})
		receiptTotal += receipt.Amount
	}
	if receiptTotal != p.Amount {
		return scheduledMutation{}, core.NewError(core.CodeConservationFailed, "M2 estate receipts do not conserve distribution")
	}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		status := "partial"
		if paid+p.Amount == due {
			status = "paid"
		}
		if err := execAgentOne(ctx, conn, "record M2 estate distribution", `INSERT INTO m2_estate_distributions(scheduler_item_id, policy_id, obligation_id, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, 'paid', 'declared_oldest_wage_claim', ?, ?, ?)`, item.SchedulerItemID, m2EstatePolicyID, obligationID, p.Amount, eventID, sequence); err != nil {
			return err
		}
		for _, receipt := range receipts {
			if err := execAgentOne(ctx, conn, "record M2 estate claimant receipt", `INSERT INTO m2_estate_distribution_receipts(scheduler_item_id, claimant_kind, claimant_id, asset_account_id, receivable_account_id, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, receipt.Kind, receipt.ID, receipt.Asset, receipt.Receivable, receipt.Amount, eventID, sequence); err != nil {
				return err
			}
		}
		if err := execAgentOne(ctx, conn, "update M2 estate wage obligation", `UPDATE m2_economic_obligations SET amount_paid_minor = ?, status = ?, last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor = ? AND amount_due_minor = ?`, paid+p.Amount, status, sequence, obligationID, paid, due); err != nil {
			return err
		}
		if status == "paid" {
			return execAgentOne(ctx, conn, "cure M2 estate wage arrears", `UPDATE m2_arrears_cases SET status = 'cured', last_event_sequence = ? WHERE obligation_id = ? AND status = 'grace_expired'`, sequence, obligationID)
		}
		return execAgentOne(ctx, conn, "checkpoint M2 estate wage arrears", `UPDATE m2_arrears_cases SET last_event_sequence = ? WHERE obligation_id = ? AND status = 'grace_expired'`, sequence, obligationID)
	}
	return mutation, nil
}
