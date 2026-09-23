package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"corerp.local/backend/internal/core"
)

type m2BankruptcyClaim struct {
	ObligationID string `json:"obligation_id"`
	CohortID     string `json:"claimant_cohort_id"`
	CurrencyID   string `json:"currency_id"`
	Due          int64  `json:"due_minor"`
	PaidAtOpen   int64  `json:"paid_at_open_minor"`
	Outstanding  int64  `json:"outstanding_at_open_minor"`
}

type m2BankruptcySlotClaim struct {
	ObligationID string `json:"obligation_id"`
	SlotIndex    int64  `json:"slot_index"`
	OriginKind   string `json:"origin_kind"`
	OriginID     string `json:"origin_id"`
	ClaimantKind string `json:"claimant_kind"`
	ClaimantID   string `json:"claimant_id"`
	Due          int64  `json:"due_minor"`
	PaidAtOpen   int64  `json:"paid_at_open_minor"`
	Outstanding  int64  `json:"outstanding_at_open_minor"`
}

// This is a fictional, fixture-local opening rule. It neither distributes
// assets nor changes the priority, ownership, or amount of any debt.
func prepareM2InsolvencyReview(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if item.PhaseID != m2InsolvencyPhase || payload.Kind != "m2_insolvency_review" || payload.SubjectID != m2InsolvencyPolicyID || payload.Day != 30 || item.WorldTime != m2WageTime(30, 7, 12) || item.SchedulerItemID != "sched_m2_insolvency_review_day_30" {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 insolvency review task")
	}
	var actor, decisionAt, cashAccount, definitionEvent string
	var threshold, zeroCash int64
	if err := conn.QueryRowContext(ctx, `SELECT p.actor_id, p.decision_at, p.min_unpaid_wage_minor, p.require_zero_cash, p.definition_event_id, a.cash_account_id FROM m2_insolvency_policies p JOIN m2_economic_actors a ON a.actor_id = p.actor_id WHERE p.policy_id = ? AND a.kind = 'employer' AND a.instance_id = ? AND a.branch_id = ?`, m2InsolvencyPolicyID, M2DemoInstanceID, M2DemoBranchID).Scan(&actor, &decisionAt, &threshold, &zeroCash, &definitionEvent, &cashAccount); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 insolvency policy")
	}
	if actor != "actor_m2_coop_employer" || cashAccount != m2EconomyEmployerCash || decisionAt != item.WorldTime || threshold != 360 || zeroCash != 1 || definitionEvent != m2EconomyEventID {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 insolvency policy differs from declared fixture")
	}
	if err := verifyM2AccountProjection(ctx, conn, cashAccount, M2DemoCurrencyID); err != nil {
		return scheduledMutation{}, err
	}
	if err := verifyM2AccountProjection(ctx, conn, m2EconomyEmployerPayable, M2DemoCurrencyID); err != nil {
		return scheduledMutation{}, err
	}
	cash, _, err := readScheduledBalance(ctx, conn, cashAccount)
	if err != nil {
		return scheduledMutation{}, err
	}
	payable, _, err := readScheduledBalance(ctx, conn, m2EconomyEmployerPayable)
	if err != nil {
		return scheduledMutation{}, err
	}
	if cash < 0 || payable > 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 insolvency accounts have impossible signs")
	}
	rows, err := conn.QueryContext(ctx, `SELECT o.obligation_id, k.cohort_id, k.currency_id, o.amount_due_minor, o.amount_paid_minor, c.status, c.grace_expires_at FROM m2_economic_obligations o JOIN m2_cohort_contracts k ON k.contract_id = o.contract_id LEFT JOIN m2_arrears_cases c ON c.obligation_id = o.obligation_id WHERE k.actor_id = ? AND k.kind = 'wage' AND o.kind = 'wage' ORDER BY o.obligation_id`, actor)
	if err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "read M2 employer wage debt", err)
	}
	var unpaid, expired int64
	claims := make([]m2BankruptcyClaim, 0)
	slotClaims := make([]m2BankruptcySlotClaim, 0)
	for rows.Next() {
		var obligationID, cohortID, currencyID string
		var due, paid int64
		var caseStatus, graceExpires sql.NullString
		if err := rows.Scan(&obligationID, &cohortID, &currencyID, &due, &paid, &caseStatus, &graceExpires); err != nil {
			rows.Close()
			return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "scan M2 employer wage debt", err)
		}
		if cohortID != M2DemoCohortID || currencyID != M2DemoCurrencyID || due <= 0 || paid < 0 || paid > due {
			rows.Close()
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 employer wage obligation has impossible amount")
		}
		if err := verifyM2ObligationPaid(ctx, conn, obligationID, "wage", paid); err != nil {
			rows.Close()
			return scheduledMutation{}, err
		}
		if paid == due {
			if caseStatus.Valid && caseStatus.String != "cured" {
				rows.Close()
				return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "paid M2 wage still has active arrears case")
			}
			continue
		}
		var split int64
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id = ?`, obligationID).Scan(&split); err != nil {
			rows.Close()
			return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "check split wage insolvency origin", err)
		}
		transferred, err := hasM2WageOwnerTransitions(ctx, conn, obligationID)
		if err != nil {
			rows.Close()
			return scheduledMutation{}, err
		}
		if !caseStatus.Valid || !graceExpires.Valid || (caseStatus.String != "open" && caseStatus.String != "grace_expired") {
			rows.Close()
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "unpaid M2 wage is missing active arrears case")
		}
		if graceExpires.String <= item.WorldTime && caseStatus.String != "grace_expired" {
			rows.Close()
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 wage grace review is missing before insolvency review")
		}
		if caseStatus.String == "grace_expired" {
			if graceExpires.String > item.WorldTime {
				rows.Close()
				return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 wage grace expired before its deadline")
			}
			expired++
		}
		var ok bool
		unpaid, ok = checkedAdd(unpaid, due-paid)
		if !ok {
			rows.Close()
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 wage debt total overflows")
		}
		if split > 0 || transferred {
			slots, err := loadM2WageClaimSlots(ctx, conn, obligationID, math.MaxInt64)
			if err != nil {
				rows.Close()
				return scheduledMutation{}, err
			}
			var slotOutstanding int64
			for _, slot := range slots {
				outstanding := slot.Due - slot.Paid
				if outstanding == 0 {
					continue
				}
				slotOutstanding += outstanding
				slotClaims = append(slotClaims, m2BankruptcySlotClaim{obligationID, slot.Index, slot.OriginKind, slot.OriginID, slot.OwnerKind, slot.OwnerID, slot.Due, slot.Paid, outstanding})
			}
			if slotOutstanding != due-paid {
				rows.Close()
				return scheduledMutation{}, core.NewError(core.CodeConservationFailed, "bankruptcy slot claims do not conserve wage debt")
			}
		} else {
			claims = append(claims, m2BankruptcyClaim{obligationID, cohortID, currencyID, due, paid, due - paid})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "iterate M2 employer wage debt", err)
	}
	if err := rows.Close(); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "close M2 employer wage debt", err)
	}
	if payable != -unpaid {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 employer payable differs from wage obligations")
	}
	var prior int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings WHERE actor_id = ?`, actor).Scan(&prior); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "check M2 prior proceeding", err)
	}
	if prior != 0 {
		return scheduledMutation{}, core.NewError(core.CodeBranchConflict, "M2 employer proceeding already exists")
	}
	status, reason, eventType := "proceeding_opened", "declared_employer_insolvency", "M2BankruptcyProceedingOpened"
	if cash > 0 {
		status, reason, eventType = "no_action", "available_cash", "M2InsolvencyReviewed"
	} else if unpaid < threshold {
		status, reason, eventType = "no_action", "below_debt_threshold", "M2InsolvencyReviewed"
	} else if expired == 0 {
		status, reason, eventType = "no_action", "no_expired_case", "M2InsolvencyReviewed"
	}
	claimSetHash := ""
	claimCount := 0
	slotClaimHash := ""
	slotClaimCount := 0
	if status == "proceeding_opened" {
		claimSetHash, err = core.HashJSON(claims)
		if err != nil {
			return scheduledMutation{}, err
		}
		claimCount = len(claims)
		slotClaimHash, err = core.HashJSON(slotClaims)
		if err != nil {
			return scheduledMutation{}, err
		}
		slotClaimCount = len(slotClaims)
	}
	return scheduledMutation{EventType: eventType, EventPayload: struct {
		PolicyID         string `json:"policy_id"`
		ActorID          string `json:"actor_id"`
		Status           string `json:"status"`
		Reason           string `json:"reason_code"`
		Cash             int64  `json:"cash_minor"`
		UnpaidWage       int64  `json:"unpaid_wage_minor"`
		ExpiredCases     int64  `json:"expired_case_count"`
		ClaimCount       int    `json:"claim_count"`
		ClaimSetHash     string `json:"claim_set_hash"`
		SlotClaimCount   int    `json:"slot_claim_count"`
		SlotClaimSetHash string `json:"slot_claim_set_hash"`
		DebtPreserved    bool   `json:"debt_preserved"`
		NoLiquidation    bool   `json:"no_liquidation"`
	}{m2InsolvencyPolicyID, actor, status, reason, cash, unpaid, expired, claimCount, claimSetHash, slotClaimCount, slotClaimHash, true, true}, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "record M2 insolvency review", `INSERT INTO m2_insolvency_reviews(scheduler_item_id, policy_id, status, reason_code, cash_minor, unpaid_wage_minor, expired_case_count, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, m2InsolvencyPolicyID, status, reason, cash, unpaid, expired, eventID, sequence); err != nil {
			return err
		}
		if status == "proceeding_opened" {
			if err := execAgentOne(ctx, conn, "open M2 employer bankruptcy proceeding", `INSERT INTO m2_bankruptcy_proceedings(proceeding_id, actor_id, opening_review_item_id, opened_world_time, status, cash_at_open_minor, unpaid_wage_at_open_minor, opening_event_id, opening_event_sequence) VALUES (?, ?, ?, ?, 'open', 0, ?, ?, ?)`, m2ProceedingID, actor, item.SchedulerItemID, item.WorldTime, unpaid, eventID, sequence); err != nil {
				return err
			}
			for _, claim := range claims {
				if err := execAgentOne(ctx, conn, "register M2 bankruptcy wage claim", `INSERT INTO m2_bankruptcy_claims(proceeding_id, obligation_id, claimant_cohort_id, currency_id, due_minor, paid_at_open_minor, outstanding_at_open_minor, opening_event_id, opening_event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, m2ProceedingID, claim.ObligationID, claim.CohortID, claim.CurrencyID, claim.Due, claim.PaidAtOpen, claim.Outstanding, eventID, sequence); err != nil {
					return err
				}
			}
			for _, claim := range slotClaims {
				if err := execAgentOne(ctx, conn, "register M2 bankruptcy slot claim", `INSERT INTO m2_bankruptcy_slot_claims(proceeding_id, obligation_id, slot_index, origin_kind, origin_id, claimant_kind, claimant_id, due_minor, paid_at_open_minor, outstanding_at_open_minor, opening_event_id, opening_event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, m2ProceedingID, claim.ObligationID, claim.SlotIndex, claim.OriginKind, claim.OriginID, claim.ClaimantKind, claim.ClaimantID, claim.Due, claim.PaidAtOpen, claim.Outstanding, eventID, sequence); err != nil {
					return err
				}
			}
		}
		return nil
	}}, nil
}

func ensureM2EmployerOperating(ctx context.Context, conn *sql.Conn, actorID string) error {
	var total, linked int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(e.event_id) FROM m2_bankruptcy_proceedings p LEFT JOIN events e ON e.event_id = p.opening_event_id AND e.event_sequence = p.opening_event_sequence AND e.event_type = 'M2BankruptcyProceedingOpened' WHERE p.actor_id = ?`, actorID).Scan(&total, &linked); err != nil {
		return core.WrapError(core.CodeStorageFailure, "check M2 employer legal state", err)
	}
	if total != linked {
		return core.NewError(core.CodeProjectionDiverged, fmt.Sprintf("M2 employer %s proceeding lacks opening event", actorID))
	}
	if total > 0 {
		if err := verifyM2BankruptcyClaims(ctx, conn, actorID); err != nil {
			return err
		}
		return core.NewError(core.CodeBranchConflict, "M2 employer has an open bankruptcy proceeding")
	}
	return nil
}

func verifyM2BankruptcyClaims(ctx context.Context, conn *sql.Conn, actorID string) error {
	var proceedingID, eventID, expectedHash, expectedSlotHash string
	var sequence, expectedAmount, expectedCount, expectedSlotCount int64
	err := conn.QueryRowContext(ctx, `SELECT p.proceeding_id, p.opening_event_id, p.opening_event_sequence, p.unpaid_wage_at_open_minor, COALESCE(json_extract(e.payload, '$.claim_set_hash'), ''), COALESCE(json_extract(e.payload, '$.claim_count'), 0), COALESCE(json_extract(e.payload, '$.slot_claim_set_hash'), ''), COALESCE(json_extract(e.payload, '$.slot_claim_count'), 0) FROM m2_bankruptcy_proceedings p JOIN events e ON e.event_id = p.opening_event_id AND e.event_sequence = p.opening_event_sequence AND e.event_type = 'M2BankruptcyProceedingOpened' WHERE p.actor_id = ?`, actorID).Scan(&proceedingID, &eventID, &sequence, &expectedAmount, &expectedHash, &expectedCount, &expectedSlotHash, &expectedSlotCount)
	if errors.Is(err, sql.ErrNoRows) {
		return core.NewError(core.CodeProjectionDiverged, "M2 bankruptcy proceeding lacks opening event")
	}
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read M2 bankruptcy claim authority", err)
	}
	rows, err := conn.QueryContext(ctx, `SELECT c.obligation_id, c.claimant_cohort_id, c.currency_id, c.due_minor, c.paid_at_open_minor, c.outstanding_at_open_minor, c.opening_event_id, c.opening_event_sequence, o.amount_due_minor, o.amount_paid_minor, k.cohort_id, k.currency_id, k.actor_id FROM m2_bankruptcy_claims c JOIN m2_economic_obligations o ON o.obligation_id = c.obligation_id JOIN m2_cohort_contracts k ON k.contract_id = o.contract_id WHERE c.proceeding_id = ? ORDER BY c.obligation_id`, proceedingID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read M2 bankruptcy claims", err)
	}
	claims := make([]m2BankruptcyClaim, 0)
	var total int64
	for rows.Next() {
		var claim m2BankruptcyClaim
		var claimEvent, obligationCohort, obligationCurrency, obligationActor string
		var claimSequence, currentDue, currentPaid int64
		if err := rows.Scan(&claim.ObligationID, &claim.CohortID, &claim.CurrencyID, &claim.Due, &claim.PaidAtOpen, &claim.Outstanding, &claimEvent, &claimSequence, &currentDue, &currentPaid, &obligationCohort, &obligationCurrency, &obligationActor); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan M2 bankruptcy claim", err)
		}
		openingDue, ok := checkedAdd(claim.PaidAtOpen, claim.Outstanding)
		if claimEvent != eventID || claimSequence != sequence || claim.CohortID != obligationCohort || claim.CurrencyID != obligationCurrency || obligationActor != actorID || claim.Due != currentDue || currentPaid > currentDue || claim.PaidAtOpen > currentPaid || !ok || openingDue != claim.Due {
			rows.Close()
			return core.NewError(core.CodeProjectionDiverged, "M2 bankruptcy claim differs from opening obligation")
		}
		total, ok = checkedAdd(total, claim.Outstanding)
		if !ok {
			rows.Close()
			return core.NewError(core.CodeIntegerOverflow, "M2 bankruptcy claim total overflows")
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate M2 bankruptcy claims", err)
	}
	if err := rows.Close(); err != nil {
		return core.WrapError(core.CodeStorageFailure, "close M2 bankruptcy claims", err)
	}
	actualHash, err := core.HashJSON(claims)
	if err != nil {
		return err
	}
	if int64(len(claims)) != expectedCount || actualHash != expectedHash {
		return core.NewError(core.CodeProjectionDiverged, "M2 bankruptcy claims do not conserve opening wage debt")
	}
	slotRows, err := conn.QueryContext(ctx, `SELECT obligation_id, slot_index, origin_kind, origin_id, claimant_kind, claimant_id, due_minor, paid_at_open_minor, outstanding_at_open_minor, opening_event_id, opening_event_sequence FROM m2_bankruptcy_slot_claims WHERE proceeding_id = ? ORDER BY obligation_id, slot_index`, proceedingID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read M2 bankruptcy slot claims", err)
	}
	slotClaims := []m2BankruptcySlotClaim{}
	for slotRows.Next() {
		var c m2BankruptcySlotClaim
		var claimEvent string
		var claimSequence int64
		if err := slotRows.Scan(&c.ObligationID, &c.SlotIndex, &c.OriginKind, &c.OriginID, &c.ClaimantKind, &c.ClaimantID, &c.Due, &c.PaidAtOpen, &c.Outstanding, &claimEvent, &claimSequence); err != nil {
			slotRows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan M2 bankruptcy slot claim", err)
		}
		if claimEvent != eventID || claimSequence != sequence || c.Due <= 0 || c.PaidAtOpen < 0 || c.Outstanding <= 0 || c.PaidAtOpen+c.Outstanding != c.Due {
			slotRows.Close()
			return core.NewError(core.CodeProjectionDiverged, "M2 bankruptcy slot claim has invalid opening authority")
		}
		slotClaims = append(slotClaims, c)
	}
	if err := slotRows.Err(); err != nil {
		slotRows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate M2 bankruptcy slot claims", err)
	}
	slotRows.Close()
	actualSlotHash, err := core.HashJSON(slotClaims)
	if err != nil {
		return err
	}
	if expectedSlotCount != int64(len(slotClaims)) || (expectedSlotHash != actualSlotHash && !(expectedSlotCount == 0 && expectedSlotHash == "")) || expectedCount+expectedSlotCount <= 0 {
		return core.NewError(core.CodeProjectionDiverged, "M2 bankruptcy slot claim set differs from opening event")
	}
	for _, claim := range slotClaims {
		slots, err := loadM2WageClaimSlots(ctx, conn, claim.ObligationID, sequence)
		if err != nil {
			return err
		}
		if claim.SlotIndex < 0 || claim.SlotIndex >= int64(len(slots)) {
			return core.NewError(core.CodeProjectionDiverged, "M2 bankruptcy slot index is invalid")
		}
		slot := slots[claim.SlotIndex]
		if slot.OriginKind != claim.OriginKind || slot.OriginID != claim.OriginID || slot.OwnerKind != claim.ClaimantKind || slot.OwnerID != claim.ClaimantID || slot.Due != claim.Due || slot.Paid != claim.PaidAtOpen || slot.Due-slot.Paid != claim.Outstanding {
			return core.NewError(core.CodeProjectionDiverged, "M2 bankruptcy slot creditor differs from opening debt")
		}
		var ok bool
		total, ok = checkedAdd(total, claim.Outstanding)
		if !ok {
			return core.NewError(core.CodeIntegerOverflow, "M2 bankruptcy debt total overflows")
		}
	}
	if total != expectedAmount {
		return core.NewError(core.CodeProjectionDiverged, "M2 bankruptcy snapshot does not conserve opening wage debt")
	}
	return nil
}
