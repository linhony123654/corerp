package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"corerp.local/backend/internal/core"
)

func recordM2ArrearsCase(ctx context.Context, conn *sql.Conn, obligationID, contractID, kind string, day int, expectedGrace int64, eventID string, sequence int64) error {
	var grace, fee int64
	if err := conn.QueryRowContext(ctx, `SELECT grace_days, late_fee_minor FROM m2_arrears_terms WHERE contract_id = ?`, contractID).Scan(&grace, &fee); err != nil {
		return classifyMissing(err, "M2 contract arrears terms")
	}
	if grace != expectedGrace || fee != 0 {
		return core.NewError(core.CodeProjectionDiverged, "M2 arrears terms differ from declared contract")
	}
	return execAgentOne(ctx, conn, "open M2 arrears case", `INSERT INTO m2_arrears_cases(obligation_id, contract_id, kind, grace_expires_at, status, shortage_event_id, last_event_sequence) VALUES (?, ?, ?, ?, 'open', ?, ?)`, obligationID, contractID, kind, m2WageTime(day+int(grace), 7, 11), eventID, sequence)
}

func prepareM2MaintenanceService(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if item.PhaseID != m2ServicePhase || payload.Kind != "m2_maintenance_service" || payload.SubjectID != m2ServiceOrderID || payload.Day != 15 || item.WorldTime != m2WageTime(15, 7, 8) || item.SchedulerItemID != "sched_m2_maintenance_service_day_15" {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 maintenance service task")
	}
	var buyer, seller, code, currency, due string
	var price int64
	if err := conn.QueryRowContext(ctx, `SELECT o.buyer_actor_id, o.seller_actor_id, o.service_code, o.currency_id, o.due_at, o.price_minor FROM m2_service_orders o JOIN m2_economic_actors b ON b.actor_id = o.buyer_actor_id JOIN m2_economic_actors s ON s.actor_id = o.seller_actor_id WHERE o.order_id = ? AND b.kind = 'landlord' AND s.kind = 'employer' AND b.instance_id = ? AND b.branch_id = ? AND s.instance_id = b.instance_id AND s.branch_id = b.branch_id`, m2ServiceOrderID, M2DemoInstanceID, M2DemoBranchID).Scan(&buyer, &seller, &code, &currency, &due, &price); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 maintenance service order")
	}
	if buyer != "actor_m2_landlord" || seller != "actor_m2_coop_employer" || code != "property_maintenance_demo" || currency != M2DemoCurrencyID || due != item.WorldTime || price != 60 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 maintenance order differs from declared fixture")
	}
	if err := ensureM2EmployerOperating(ctx, conn, seller); err != nil {
		return scheduledMutation{}, err
	}
	accounts := []string{m2EconomyLandlordCash, m2EconomyEmployerCash, m2LandlordServiceExpense, m2EmployerServiceIncome}
	balances := make([]balanceMutation, 0, 4)
	for _, accountID := range accounts {
		if err := verifyM2AccountProjection(ctx, conn, accountID, currency); err != nil {
			return scheduledMutation{}, err
		}
		balance, version, err := readScheduledBalance(ctx, conn, accountID)
		if err != nil {
			return scheduledMutation{}, err
		}
		balances = append(balances, balanceMutation{AccountID: accountID, ExpectedVersion: version, NewBalance: balance})
	}
	if balances[0].NewBalance < price {
		return scheduledMutation{EventType: "M2MaintenanceServiceRejected", EventPayload: struct {
			OrderID   string `json:"order_id"`
			Reason    string `json:"reason_code"`
			Required  int64  `json:"required_minor"`
			Available int64  `json:"available_minor"`
		}{m2ServiceOrderID, "insufficient_landlord_funds", price, balances[0].NewBalance}, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
			return execAgentOne(ctx, conn, "record unfunded M2 maintenance order", `INSERT INTO m2_service_settlements(order_id, scheduler_item_id, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, 'rejected', 'insufficient_landlord_funds', 0, ?, ?)`, m2ServiceOrderID, item.SchedulerItemID, eventID, sequence)
		}}, nil
	}
	balances[0].NewBalance -= price
	for index, delta := range []int64{price, price, -price} {
		value, ok := checkedAdd(balances[index+1].NewBalance, delta)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 maintenance service account overflows")
		}
		balances[index+1].NewBalance = value
	}
	return scheduledMutation{
		EventType: "M2MaintenanceServicePaid",
		EventPayload: struct {
			OrderID string `json:"order_id"`
			Buyer   string `json:"buyer_actor_id"`
			Seller  string `json:"seller_actor_id"`
			Amount  int64  `json:"amount_minor"`
		}{m2ServiceOrderID, buyer, seller, price},
		Postings: []scheduledPosting{{accounts[0], currency, -price, "M2 maintenance cash paid"}, {accounts[1], currency, price, "M2 maintenance cash earned"}, {accounts[2], currency, price, "M2 landlord maintenance expense"}, {accounts[3], currency, -price, "M2 employer service income"}},
		Balances: balances,
		ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
			return execAgentOne(ctx, conn, "settle M2 maintenance order", `INSERT INTO m2_service_settlements(order_id, scheduler_item_id, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, 'paid', 'none', ?, ?, ?)`, m2ServiceOrderID, item.SchedulerItemID, price, eventID, sequence)
		},
	}, nil
}

func verifyM2ObligationPaid(ctx context.Context, conn *sql.Conn, obligationID, kind string, recorded int64) error {
	var duePaid, latePaid, estatePaid int64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(CAST(json_extract(e.payload, '$.paid_minor') AS INTEGER)), 0) FROM events e WHERE e.instance_id = ? AND e.branch_id = ? AND e.event_type = ? AND json_extract(e.payload, '$.obligation_id') = ?`, M2DemoInstanceID, M2DemoBranchID, "M2"+map[string]string{"wage": "Wage", "rent": "Rent"}[kind]+"Settled", obligationID).Scan(&duePaid); err != nil {
		return core.WrapError(core.CodeStorageFailure, "read M2 obligation due payment facts", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_arrears_attempts WHERE obligation_id = ? AND status IN ('partial', 'paid')`, obligationID).Scan(&latePaid); err != nil {
		return core.WrapError(core.CodeStorageFailure, "read M2 obligation late payment facts", err)
	}
	if kind == "wage" {
		var err error
		estatePaid, err = verifyM2EstatePaymentFacts(ctx, conn, obligationID)
		if err != nil {
			return err
		}
	}
	actual, ok := checkedAdd(duePaid, latePaid)
	if ok {
		actual, ok = checkedAdd(actual, estatePaid)
	}
	if !ok || actual != recorded {
		return core.NewError(core.CodeProjectionDiverged, "M2 obligation paid projection differs from payment facts")
	}
	if kind == "wage" {
		if err := verifyM2WageSplitReceipts(ctx, conn, obligationID, recorded); err != nil {
			return err
		}
	}
	return nil
}

func prepareM2ArrearsRetry(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	kind, contractID, source, destination, payable, receivable, phase, payloadKind := "wage", m2EconomyContractID, m2EconomyEmployerCash, M2DemoCohortAssetAccountID, m2EconomyEmployerPayable, M2DemoCohortReceivableID, m2WageRetryPhase, "m2_wage_arrears_retry"
	if item.PhaseID == m2RentRetryPhase {
		kind, contractID, source, destination, payable, receivable, phase, payloadKind = "rent", m2EconomyRentContract, M2DemoCohortAssetAccountID, m2EconomyLandlordCash, m2EconomyCohortRentDue, m2EconomyLandlordRentDue, m2RentRetryPhase, "m2_rent_arrears_retry"
	}
	minute := 9
	if kind == "rent" {
		minute = 10
	}
	if item.PhaseID != phase || payload.Kind != payloadKind || payload.SubjectID != contractID || payload.Day < 2 || payload.Day > 30 || item.WorldTime != m2WageTime(payload.Day, 7, minute) || item.SchedulerItemID != fmt.Sprintf("sched_%s_day_%d", payloadKind, payload.Day) {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 arrears retry task")
	}
	if kind == "wage" {
		if err := verifyM2WageParticipationReturns(ctx, conn); err != nil {
			return scheduledMutation{}, err
		}
		var proceedings int64
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings p JOIN m2_cohort_contracts k ON k.actor_id = p.actor_id WHERE k.contract_id = ?`, contractID).Scan(&proceedings); err != nil {
			return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "check M2 wage claim payout routing", err)
		}
		if proceedings > 0 {
			return scheduledMutation{}, core.NewError(core.CodeBranchConflict, "post-proceeding wage payout requires claimant routing")
		}
	}
	var grace, fee int64
	if err := conn.QueryRowContext(ctx, `SELECT grace_days, late_fee_minor FROM m2_arrears_terms WHERE contract_id = ?`, contractID).Scan(&grace, &fee); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 arrears retry terms")
	}
	if (kind == "wage" && grace != 3) || (kind == "rent" && grace != 2) || fee != 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 arrears retry terms differ from definition")
	}
	var obligationID, caseStatus, graceExpires string
	var due, paid int64
	err := conn.QueryRowContext(ctx, `SELECT o.obligation_id, o.amount_due_minor, o.amount_paid_minor, c.status, c.grace_expires_at FROM m2_economic_obligations o JOIN m2_arrears_cases c ON c.obligation_id = o.obligation_id WHERE o.contract_id = ? AND o.kind = ? AND o.amount_paid_minor < o.amount_due_minor AND o.period_end < ? AND c.status IN ('open', 'grace_expired') ORDER BY o.period_end, o.obligation_id LIMIT 1`, contractID, kind, item.WorldTime).Scan(&obligationID, &due, &paid, &caseStatus, &graceExpires)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "select M2 oldest arrears case", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return scheduledMutation{EventType: "M2ArrearsRetrySkipped", EventPayload: struct {
			ContractID string `json:"contract_id"`
			Day        int    `json:"day"`
			Reason     string `json:"reason_code"`
		}{contractID, payload.Day, "none_due"}, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
			return execAgentOne(ctx, conn, "record M2 no-arrears retry", `INSERT INTO m2_arrears_attempts(scheduler_item_id, contract_id, day, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, 'no_arrears', 'none_due', 0, ?, ?)`, item.SchedulerItemID, contractID, payload.Day, eventID, sequence)
		}}, nil
	}
	if due <= 0 || paid < 0 || paid >= due || (caseStatus != "open" && caseStatus != "grace_expired") || graceExpires == "" {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 arrears case differs from obligation")
	}
	if kind == "wage" {
		hasTransfers, err := hasM2WageOwnerTransitions(ctx, conn, obligationID)
		if err != nil {
			return scheduledMutation{}, err
		}
		if hasTransfers {
			return prepareM2SlotAwareWageRetry(ctx, conn, item, payload, obligationID, due, paid, graceExpires)
		}
		var split int64
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id = ?`, obligationID).Scan(&split); err != nil {
			return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "check split wage arrears", err)
		}
		if split > 0 {
			return prepareM2SplitWageArrearsRetry(ctx, conn, item, payload, obligationID, due, paid, caseStatus, graceExpires)
		}
	}
	if err := verifyM2ObligationPaid(ctx, conn, obligationID, kind, paid); err != nil {
		return scheduledMutation{}, err
	}
	for _, accountID := range []string{source, destination, payable, receivable} {
		if err := verifyM2AccountProjection(ctx, conn, accountID, M2DemoCurrencyID); err != nil {
			return scheduledMutation{}, err
		}
	}
	sourceBalance, sourceVersion, err := readScheduledBalance(ctx, conn, source)
	if err != nil {
		return scheduledMutation{}, err
	}
	destinationBalance, destinationVersion, err := readScheduledBalance(ctx, conn, destination)
	if err != nil {
		return scheduledMutation{}, err
	}
	payableBalance, payableVersion, err := readScheduledBalance(ctx, conn, payable)
	if err != nil {
		return scheduledMutation{}, err
	}
	receivableBalance, receivableVersion, err := readScheduledBalance(ctx, conn, receivable)
	if err != nil {
		return scheduledMutation{}, err
	}
	remaining := due - paid
	if sourceBalance < 0 || destinationBalance < 0 || payableBalance > -remaining || receivableBalance < remaining {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 arrears accounts cannot cover recorded debt")
	}
	payment := remaining
	if sourceBalance < payment {
		payment = sourceBalance
	}
	status, reason := "paid", "none"
	if payment == 0 {
		status, reason = "deferred", "insufficient_liquidity"
	} else if payment < remaining {
		status = "partial"
	}
	mutation := scheduledMutation{EventType: "M2ArrearsRetried", EventPayload: struct {
		ContractID   string `json:"contract_id"`
		ObligationID string `json:"obligation_id"`
		Day          int    `json:"day"`
		Status       string `json:"status"`
		Reason       string `json:"reason_code"`
		PreviousPaid int64  `json:"previously_paid_minor"`
		Paid         int64  `json:"paid_minor"`
		Remaining    int64  `json:"remaining_minor"`
		GraceExpires string `json:"grace_expires_at"`
	}{contractID, obligationID, payload.Day, status, reason, paid, payment, remaining - payment, graceExpires}}
	if payment > 0 {
		newDestination, ok := checkedAdd(destinationBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 arrears destination credit overflows")
		}
		newPayable, ok := checkedAdd(payableBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 arrears payable release overflows")
		}
		newReceivable, ok := checkedSubtract(receivableBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 arrears receivable release overflows")
		}
		mutation.Postings = []scheduledPosting{{source, M2DemoCurrencyID, -payment, "M2 late obligation payment"}, {destination, M2DemoCurrencyID, payment, "M2 late obligation receipt"}, {payable, M2DemoCurrencyID, payment, "M2 late payable cleared"}, {receivable, M2DemoCurrencyID, -payment, "M2 late receivable cleared"}}
		mutation.Balances = []balanceMutation{{source, sourceVersion, sourceBalance - payment}, {destination, destinationVersion, newDestination}, {payable, payableVersion, newPayable}, {receivable, receivableVersion, newReceivable}}
	}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "record M2 linked arrears attempt", `INSERT INTO m2_arrears_attempts(scheduler_item_id, contract_id, obligation_id, day, status, reason_code, amount_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, contractID, obligationID, payload.Day, status, reason, payment, eventID, sequence); err != nil {
			return err
		}
		if payment > 0 {
			obligationStatus := "partial"
			if payment == remaining {
				obligationStatus = "paid"
			}
			if err := execAgentOne(ctx, conn, "update original M2 obligation after late payment", `UPDATE m2_economic_obligations SET amount_paid_minor = ?, status = ?, last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor = ? AND amount_due_minor = ?`, paid+payment, obligationStatus, sequence, obligationID, paid, due); err != nil {
				return err
			}
			if obligationStatus == "paid" {
				return execAgentOne(ctx, conn, "cure M2 arrears case", `UPDATE m2_arrears_cases SET status = 'cured', last_event_sequence = ? WHERE obligation_id = ? AND status IN ('open', 'grace_expired')`, sequence, obligationID)
			}
			return execAgentOne(ctx, conn, "checkpoint M2 partially paid arrears case", `UPDATE m2_arrears_cases SET last_event_sequence = ? WHERE obligation_id = ? AND status IN ('open', 'grace_expired')`, sequence, obligationID)
		}
		return nil
	}
	return mutation, nil
}

func prepareM2DefaultReview(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if item.PhaseID != m2DefaultReviewPhase || payload.Kind != "m2_default_review" || payload.SubjectID != M2DemoCohortID || payload.Day < 1 || payload.Day > 30 || item.WorldTime != m2WageTime(payload.Day, 7, 11) || item.SchedulerItemID != fmt.Sprintf("sched_m2_default_review_day_%d", payload.Day) {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 default review task")
	}
	rows, err := conn.QueryContext(ctx, `SELECT c.obligation_id, c.kind, o.amount_due_minor, o.amount_paid_minor FROM m2_arrears_cases c JOIN m2_economic_obligations o ON o.obligation_id = c.obligation_id WHERE c.status = 'open' AND c.grace_expires_at <= ? ORDER BY c.grace_expires_at, c.obligation_id`, item.WorldTime)
	if err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "read M2 grace expiries", err)
	}
	type decision struct {
		ObligationID string `json:"obligation_id"`
		Kind         string `json:"kind"`
		Decision     string `json:"decision"`
		Reason       string `json:"reason_code"`
	}
	decisions := []decision{}
	for rows.Next() {
		var obligationID, kind string
		var due, paid int64
		if err := rows.Scan(&obligationID, &kind, &due, &paid); err != nil {
			rows.Close()
			return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "scan M2 grace expiry", err)
		}
		if paid >= due || due <= 0 {
			rows.Close()
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 grace expiry has no debt")
		}
		if err := verifyM2ObligationPaid(ctx, conn, obligationID, kind, paid); err != nil {
			rows.Close()
			return scheduledMutation{}, err
		}
		if kind == "wage" {
			decisions = append(decisions, decision{obligationID, kind, "refinement_blocked_contract_split", "aggregate_contract_has_open_debt"})
		} else if kind == "rent" {
			decisions = append(decisions, decision{obligationID, kind, "rent_grace_expired_review", "tenant_rent_unpaid_after_grace"})
		} else {
			rows.Close()
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "unknown M2 arrears kind")
		}
	}
	if err := rows.Close(); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "close M2 grace expiries", err)
	}
	if err := rows.Err(); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "iterate M2 grace expiries", err)
	}
	mutation := scheduledMutation{EventType: "M2DefaultReviewed", EventPayload: struct {
		Day         int        `json:"day"`
		NewExpiries []decision `json:"new_expiries"`
		Reason      string     `json:"reason_code"`
	}{payload.Day, decisions, "contract_local_grace_review"}}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		for _, d := range decisions {
			if err := execAgentOne(ctx, conn, "mark M2 grace expired", `UPDATE m2_arrears_cases SET status = 'grace_expired', last_event_sequence = ? WHERE obligation_id = ? AND status = 'open'`, sequence, d.ObligationID); err != nil {
				return err
			}
			if err := execAgentOne(ctx, conn, "record M2 reasoned default transition", `INSERT INTO m2_default_transitions(obligation_id, decision, reason_code, event_id, event_sequence) VALUES (?, ?, ?, ?, ?)`, d.ObligationID, d.Decision, d.Reason, eventID, sequence); err != nil {
				return err
			}
		}
		return execAgentOne(ctx, conn, "record M2 default review", `INSERT INTO m2_default_reviews(scheduler_item_id, day, expired_count, event_id, event_sequence) VALUES (?, ?, ?, ?, ?)`, item.SchedulerItemID, payload.Day, len(decisions), eventID, sequence)
	}
	return mutation, nil
}
