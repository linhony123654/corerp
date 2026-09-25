package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

type rpRentQueueProjection struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
	Branch   string `json:"branch"`
	At       string `json:"at"`
	Phase    string `json:"phase"`
	Priority int64  `json:"priority"`
	Status   string `json:"status"`
	Payload  string `json:"payload"`
}

type rpRentObligationProjection struct {
	ID        string `json:"id"`
	Contract  string `json:"contract"`
	StartDay  int    `json:"start_day"`
	EndDay    int    `json:"end_day"`
	DueAt     string `json:"due_at"`
	GraceAt   string `json:"grace_at"`
	DueMinor  int64  `json:"due_minor"`
	PaidMinor int64  `json:"paid_minor"`
	Status    string `json:"status"`
	LastSeq   int64  `json:"last_sequence"`
}

type rpRentSettlementProjection struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Obligation string `json:"obligation"`
	Attempted  int64  `json:"attempted"`
	Paid       int64  `json:"paid"`
	Remaining  int64  `json:"remaining"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	Event      string `json:"event"`
	Sequence   int64  `json:"sequence"`
}

type rpRentRuntimeExpected struct {
	Queues      map[string]rpRentQueueProjection
	Obligations map[string]rpRentObligationProjection
	Settlements map[string]rpRentSettlementProjection
	Journals    map[string]rpRentJournalProjection
}

type rpRentPostingProjection struct {
	ID       string `json:"id"`
	Account  string `json:"account"`
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

type rpRentJournalProjection struct {
	ID       string                    `json:"id"`
	Event    string                    `json:"event"`
	Status   string                    `json:"status"`
	Purpose  string                    `json:"purpose"`
	Postings []rpRentPostingProjection `json:"postings"`
}

func rpRentScheduleFromProjection(row rpHouseholdRentProjection) (rpHouseholdRentSchedule, error) {
	start, err := time.Parse(time.RFC3339, row.Starts)
	if err != nil {
		return rpHouseholdRentSchedule{}, err
	}
	return rpHouseholdRentSchedule{
		AgreementID: row.ID, ContractID: row.Contract, InstanceID: row.Instance, BranchID: row.Branch,
		SourceEvent: row.Source, StartTime: start, StartDay: row.StartsDay, PeriodDays: row.PeriodDays,
		GraceDays: row.GraceDays, RentMinor: row.Rent, CurrencyID: row.Currency,
		RentAccount: row.RentAccount, LandlordCash: row.LandlordAccount,
	}, nil
}

func rpRentQueueFromItem(item SchedulerItem, instance, branch string) rpRentQueueProjection {
	return rpRentQueueProjection{item.SchedulerItemID, instance, branch, item.WorldTime, item.PhaseID, item.DeclaredPriority, item.Status, item.Payload}
}

func rpRentExpectedPeriodQueues(target *rpRentRuntimeExpected, row rpHouseholdRentSchedule, period int) error {
	for _, kind := range []string{rpHouseholdRentAccrue, rpHouseholdRentPay, rpHouseholdRentPastDue} {
		item, _, err := rpHouseholdRentItem(row, period, kind)
		if err != nil {
			return err
		}
		if _, duplicate := target.Queues[item.SchedulerItemID]; duplicate {
			return core.NewError(core.CodeProjectionDiverged, "duplicate sourced rent queue")
		}
		target.Queues[item.SchedulerItemID] = rpRentQueueFromItem(item, row.InstanceID, row.BranchID)
	}
	return nil
}

func rpHouseholdRentRuntimeExpectedRows(ctx context.Context, q replayQuerier, instance, branch string, through int64) (rpRentRuntimeExpected, error) {
	agreements, err := rpHouseholdRentExpected(ctx, q, instance, branch, through)
	if err != nil {
		return rpRentRuntimeExpected{}, err
	}
	want := rpRentRuntimeExpected{
		Queues: map[string]rpRentQueueProjection{}, Obligations: map[string]rpRentObligationProjection{},
		Settlements: map[string]rpRentSettlementProjection{}, Journals: map[string]rpRentJournalProjection{},
	}
	models := map[string]rpHouseholdRentSchedule{}
	for key, agreement := range agreements {
		model, err := rpRentScheduleFromProjection(agreement)
		if err != nil {
			return rpRentRuntimeExpected{}, err
		}
		models[key] = model
		if err := rpRentExpectedPeriodQueues(&want, model, 1); err != nil {
			return rpRentRuntimeExpected{}, err
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,event_type,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPHouseholdRentAccrued','RPHouseholdRentPaid','RPHouseholdRentPaymentFailed','RPHouseholdRentPastDue','RPHouseholdRentPastDueSkipped') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return rpRentRuntimeExpected{}, err
	}
	for rows.Next() {
		var eventID, kind, at, raw string
		var sequence int64
		if err := rows.Scan(&eventID, &sequence, &kind, &at, &raw); err != nil {
			rows.Close()
			return rpRentRuntimeExpected{}, err
		}
		var agreementID, obligationID string
		var period int
		var journalPaid int64
		if kind == "RPHouseholdRentAccrued" {
			var source struct {
				AgreementID  string `json:"agreement_id"`
				ContractID   string `json:"contract_id"`
				ObligationID string `json:"obligation_id"`
				PeriodIndex  int    `json:"period_index"`
				StartDay     int    `json:"period_start_day"`
				EndDay       int    `json:"period_end_day"`
				DueWorldTime string `json:"due_world_time"`
				GraceUntil   string `json:"grace_until_world_time"`
				AmountMinor  int64  `json:"amount_minor"`
			}
			if err := json.Unmarshal([]byte(raw), &source); err != nil {
				rows.Close()
				return rpRentRuntimeExpected{}, err
			}
			agreementID, obligationID, period = source.AgreementID, source.ObligationID, source.PeriodIndex
			model, ok := models[agreementID]
			if !ok {
				rows.Close()
				return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "rent accrual has no sourced agreement")
			}
			startDay, endDay := model.StartDay+(period-1)*model.PeriodDays, model.StartDay+period*model.PeriodDays
			due := model.StartTime.AddDate(0, 0, period*model.PeriodDays).UTC().Format(time.RFC3339)
			grace := model.StartTime.AddDate(0, 0, period*model.PeriodDays+model.GraceDays).UTC().Format(time.RFC3339)
			if period < 1 || source.ContractID != model.ContractID || source.StartDay != startDay || source.EndDay != endDay || source.AmountMinor != model.RentMinor || source.DueWorldTime != due || source.GraceUntil != grace || obligationID != fmt.Sprintf("rent_%s_%d_%d", model.ContractID, startDay, endDay) {
				rows.Close()
				return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "invalid sourced household rent accrual")
			}
			if _, exists := want.Obligations[obligationID]; exists {
				rows.Close()
				return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "rent obligation accrued twice")
			}
			want.Obligations[obligationID] = rpRentObligationProjection{obligationID, model.ContractID, startDay, endDay, due, grace, model.RentMinor, 0, "due", sequence}
			if err := rpRentExpectedPeriodQueues(&want, model, period+1); err != nil {
				rows.Close()
				return rpRentRuntimeExpected{}, err
			}
		} else {
			var source struct {
				AgreementID string `json:"agreement_id"`
				PeriodIndex int    `json:"period_index"`
				Settlement  struct {
					ObligationID   string `json:"obligation_id"`
					PaidMinor      int64  `json:"paid_minor"`
					RemainingMinor int64  `json:"remaining_minor"`
					Status         string `json:"status"`
					Reason         string `json:"reason"`
				} `json:"settlement"`
			}
			if err := json.Unmarshal([]byte(raw), &source); err != nil {
				rows.Close()
				return rpRentRuntimeExpected{}, err
			}
			agreementID, obligationID, period = source.AgreementID, source.Settlement.ObligationID, source.PeriodIndex
			model, ok := models[agreementID]
			if !ok {
				rows.Close()
				return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "rent settlement has no sourced agreement")
			}
			obligation, ok := want.Obligations[obligationID]
			if !ok || period < 1 || obligation.Contract != model.ContractID || obligation.EndDay != model.StartDay+period*model.PeriodDays {
				rows.Close()
				return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "rent settlement has no matching obligation")
			}
			if kind == "RPHouseholdRentPaid" || kind == "RPHouseholdRentPaymentFailed" {
				remaining := obligation.DueMinor - obligation.PaidMinor
				paid := source.Settlement.PaidMinor
				if paid < 0 || paid > remaining || source.Settlement.RemainingMinor != remaining-paid || (kind == "RPHouseholdRentPaid") != (paid == remaining) {
					rows.Close()
					return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "invalid household rent payment source")
				}
				obligation.PaidMinor += paid
				journalPaid = paid
				obligation.LastSeq = sequence
				settlementStatus, reason := "paid", "settled"
				if paid < remaining {
					obligation.Status = "partially_paid"
					settlementStatus, reason = "partial", "insufficient_funds"
					if paid == 0 {
						settlementStatus = "failed"
					}
				} else {
					obligation.Status = "paid"
				}
				if source.Settlement.Status != obligation.Status {
					rows.Close()
					return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "rent payment status differs from source")
				}
				id := fmt.Sprintf("settlement_rent_%s_day_%03d", obligationID, obligation.EndDay)
				if _, duplicate := want.Settlements[id]; duplicate {
					rows.Close()
					return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "duplicate sourced rent payment")
				}
				want.Settlements[id] = rpRentSettlementProjection{id, "rent", obligationID, remaining, paid, remaining - paid, settlementStatus, reason, eventID, sequence}
			} else if kind == "RPHouseholdRentPastDue" {
				if obligation.PaidMinor >= obligation.DueMinor || source.Settlement.RemainingMinor != obligation.DueMinor-obligation.PaidMinor {
					rows.Close()
					return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "invalid rent past-due source")
				}
				obligation.Status = "past_due"
				obligation.LastSeq = sequence
			} else if obligation.PaidMinor != obligation.DueMinor || source.Settlement.Reason != "rent_already_paid" {
				rows.Close()
				return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "invalid rent past-due skip source")
			}
			want.Obligations[obligationID] = obligation
		}
		model, ok := models[agreementID]
		if !ok {
			rows.Close()
			return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "rent schedule has no agreement")
		}
		queueKind := rpHouseholdRentAccrue
		switch kind {
		case "RPHouseholdRentPaid", "RPHouseholdRentPaymentFailed":
			queueKind = rpHouseholdRentPay
		case "RPHouseholdRentPastDue", "RPHouseholdRentPastDueSkipped":
			queueKind = rpHouseholdRentPastDue
		}
		item, _, err := rpHouseholdRentItem(model, period, queueKind)
		if err != nil || item.WorldTime != at || "event_"+item.SchedulerItemID != eventID {
			rows.Close()
			return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "rent Event differs from scheduled source")
		}
		queue, exists := want.Queues[item.SchedulerItemID]
		if !exists || queue.Status != "pending" {
			rows.Close()
			return rpRentRuntimeExpected{}, core.NewError(core.CodeProjectionDiverged, "rent scheduled Event repeats or lacks queue")
		}
		queue.Status = "completed"
		want.Queues[item.SchedulerItemID] = queue
		if kind == "RPHouseholdRentAccrued" || journalPaid > 0 {
			agreement := agreements[agreementID]
			var accounts []string
			var amounts []int64
			if kind == "RPHouseholdRentAccrued" {
				accounts = []string{agreement.LedgerExpense, agreement.LedgerPayable, agreement.LedgerReceivable, agreement.LedgerIncome}
				amounts = []int64{model.RentMinor, -model.RentMinor, model.RentMinor, -model.RentMinor}
			} else {
				accounts = []string{agreement.LedgerPayable, model.RentAccount, model.LandlordCash, agreement.LedgerReceivable}
				amounts = []int64{journalPaid, -journalPaid, journalPaid, -journalPaid}
			}
			journal := rpRentJournalProjection{ID: "journal_" + item.SchedulerItemID, Event: eventID, Status: "posted", Purpose: kind}
			for index, account := range accounts {
				journal.Postings = append(journal.Postings, rpRentPostingProjection{
					ID: fmt.Sprintf("posting_%s_%02d", item.SchedulerItemID, index), Account: account,
					Currency: model.CurrencyID, Amount: amounts[index],
				})
			}
			want.Journals[journal.ID] = journal
		}
	}
	err = rows.Err()
	rows.Close()
	return want, err
}

func rpRentCompareRow(kind, key string, expected any, actual any, present bool) (ProjectionDifference, bool, error) {
	want, err := core.CanonicalJSON(expected)
	if err != nil {
		return ProjectionDifference{}, false, err
	}
	got := "missing"
	if present {
		encoded, err := core.CanonicalJSON(actual)
		if err != nil {
			return ProjectionDifference{}, false, err
		}
		got = string(encoded)
	}
	if string(want) == got {
		return ProjectionDifference{}, false, nil
	}
	return ProjectionDifference{Projection: kind, Key: key, ExpectedText: string(want), ActualText: got}, true, nil
}

func rpHouseholdRentRuntimeDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpHouseholdRentRuntimeExpectedRows(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	queueRows, err := q.QueryContext(ctx, `SELECT scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload FROM scheduler_items WHERE instance_id=? AND branch_id=? AND phase_id=? ORDER BY scheduler_item_id`, instance, branch, rpHouseholdRentPhase)
	if err != nil {
		return nil, err
	}
	actualQueues := map[string]rpRentQueueProjection{}
	for queueRows.Next() {
		var row rpRentQueueProjection
		if err := queueRows.Scan(&row.ID, &row.Instance, &row.Branch, &row.At, &row.Phase, &row.Priority, &row.Status, &row.Payload); err != nil {
			queueRows.Close()
			return nil, err
		}
		actualQueues[row.ID] = row
	}
	err = queueRows.Err()
	queueRows.Close()
	if err != nil {
		return nil, err
	}
	queueKeys := map[string]bool{}
	for key := range want.Queues {
		queueKeys[key] = true
	}
	for key := range actualQueues {
		queueKeys[key] = true
	}
	for _, key := range rpRentSortedKeys(queueKeys) {
		row, sourced := want.Queues[key]
		if !sourced {
			differences = append(differences, ProjectionDifference{Projection: "rp_household_rent_queue", Key: key, ExpectedText: "missing", ActualText: "unsourced"})
			continue
		}
		got, present := actualQueues[key]
		difference, unequal, err := rpRentCompareRow("rp_household_rent_queue", key, row, got, present)
		if err != nil {
			return nil, err
		}
		if unequal {
			differences = append(differences, difference)
		}
	}
	obligationRows, err := q.QueryContext(ctx, `SELECT o.obligation_id,o.contract_id,o.period_start_day,o.period_end_day,o.due_world_time,o.grace_until_world_time,o.amount_due_minor,o.amount_paid_minor,o.status,o.last_event_sequence FROM rent_obligations o JOIN rp_household_rent_agreements a ON a.contract_id=o.contract_id WHERE a.instance_id=? AND a.branch_id=? ORDER BY o.obligation_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	actualObligations := map[string]rpRentObligationProjection{}
	for obligationRows.Next() {
		var row rpRentObligationProjection
		if err := obligationRows.Scan(&row.ID, &row.Contract, &row.StartDay, &row.EndDay, &row.DueAt, &row.GraceAt, &row.DueMinor, &row.PaidMinor, &row.Status, &row.LastSeq); err != nil {
			obligationRows.Close()
			return nil, err
		}
		actualObligations[row.ID] = row
	}
	err = obligationRows.Err()
	obligationRows.Close()
	if err != nil {
		return nil, err
	}
	obligationKeys := map[string]bool{}
	for key := range want.Obligations {
		obligationKeys[key] = true
	}
	for key := range actualObligations {
		obligationKeys[key] = true
	}
	for _, key := range rpRentSortedKeys(obligationKeys) {
		row, sourced := want.Obligations[key]
		if !sourced {
			differences = append(differences, ProjectionDifference{Projection: "rp_household_rent_obligation", Key: key, ExpectedText: "missing", ActualText: "unsourced"})
			continue
		}
		got, present := actualObligations[key]
		difference, unequal, err := rpRentCompareRow("rp_household_rent_obligation", key, row, got, present)
		if err != nil {
			return nil, err
		}
		if unequal {
			differences = append(differences, difference)
		}
	}
	settlementRows, err := q.QueryContext(ctx, `SELECT s.settlement_id,s.obligation_kind,s.obligation_id,s.attempted_minor,s.paid_minor,s.remaining_minor,s.status,s.reason_code,s.event_id,s.event_sequence FROM obligation_settlements s JOIN rent_obligations o ON o.obligation_id=s.obligation_id JOIN rp_household_rent_agreements a ON a.contract_id=o.contract_id WHERE a.instance_id=? AND a.branch_id=? AND s.obligation_kind='rent' ORDER BY s.settlement_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	actualSettlements := map[string]rpRentSettlementProjection{}
	for settlementRows.Next() {
		var row rpRentSettlementProjection
		if err := settlementRows.Scan(&row.ID, &row.Kind, &row.Obligation, &row.Attempted, &row.Paid, &row.Remaining, &row.Status, &row.Reason, &row.Event, &row.Sequence); err != nil {
			settlementRows.Close()
			return nil, err
		}
		actualSettlements[row.ID] = row
	}
	err = settlementRows.Err()
	settlementRows.Close()
	if err != nil {
		return nil, err
	}
	settlementKeys := map[string]bool{}
	for key := range want.Settlements {
		settlementKeys[key] = true
	}
	for key := range actualSettlements {
		settlementKeys[key] = true
	}
	for _, key := range rpRentSortedKeys(settlementKeys) {
		row, sourced := want.Settlements[key]
		if !sourced {
			differences = append(differences, ProjectionDifference{Projection: "rp_household_rent_settlement", Key: key, ExpectedText: "missing", ActualText: "unsourced"})
			continue
		}
		got, present := actualSettlements[key]
		difference, unequal, err := rpRentCompareRow("rp_household_rent_settlement", key, row, got, present)
		if err != nil {
			return nil, err
		}
		if unequal {
			differences = append(differences, difference)
		}
	}
	journalRows, err := q.QueryContext(ctx, `SELECT j.entry_id,j.event_id,j.status,j.purpose FROM journal_entries j JOIN events e ON e.event_id=j.event_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND (j.entry_id GLOB 'journal_sched_rp_household_rent_*' OR e.event_type IN ('RPHouseholdRentAccrued','RPHouseholdRentPaid','RPHouseholdRentPaymentFailed','RPHouseholdRentPastDue','RPHouseholdRentPastDueSkipped')) ORDER BY j.entry_id`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	actualJournals := map[string]rpRentJournalProjection{}
	for journalRows.Next() {
		var row rpRentJournalProjection
		if err := journalRows.Scan(&row.ID, &row.Event, &row.Status, &row.Purpose); err != nil {
			journalRows.Close()
			return nil, err
		}
		actualJournals[row.ID] = row
	}
	err = journalRows.Err()
	journalRows.Close()
	if err != nil {
		return nil, err
	}
	journalKeys := map[string]bool{}
	for key := range want.Journals {
		journalKeys[key] = true
	}
	for key := range actualJournals {
		journalKeys[key] = true
	}
	for _, key := range rpRentSortedKeys(journalKeys) {
		got, present := actualJournals[key]
		if present {
			postings, err := q.QueryContext(ctx, `SELECT posting_id,account_id,currency_id,amount_minor FROM postings WHERE entry_id=? ORDER BY posting_id`, key)
			if err != nil {
				return nil, err
			}
			for postings.Next() {
				var posting rpRentPostingProjection
				if err := postings.Scan(&posting.ID, &posting.Account, &posting.Currency, &posting.Amount); err != nil {
					postings.Close()
					return nil, err
				}
				got.Postings = append(got.Postings, posting)
			}
			err = postings.Err()
			postings.Close()
			if err != nil {
				return nil, err
			}
		}
		row, sourced := want.Journals[key]
		if !sourced {
			differences = append(differences, ProjectionDifference{Projection: "rp_household_rent_journal", Key: key, ExpectedText: "missing", ActualText: "unsourced"})
			continue
		}
		difference, unequal, err := rpRentCompareRow("rp_household_rent_journal", key, row, got, present)
		if err != nil {
			return nil, err
		}
		if unequal {
			differences = append(differences, difference)
		}
	}
	return differences, nil
}

func rpRentSortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func repairRPHouseholdRentRuntime(ctx context.Context, conn *sql.Conn, instance, branch string, through int64, differences []ProjectionDifference) error {
	if len(differences) == 0 {
		return nil
	}
	want, err := rpHouseholdRentRuntimeExpectedRows(ctx, conn, instance, branch, through)
	if err != nil {
		return err
	}
	for _, diff := range differences {
		var sourced bool
		switch diff.Projection {
		case "rp_household_rent_queue":
			_, sourced = want.Queues[diff.Key]
		case "rp_household_rent_obligation":
			_, sourced = want.Obligations[diff.Key]
		case "rp_household_rent_settlement":
			_, sourced = want.Settlements[diff.Key]
		}
		if !sourced {
			return core.NewError(core.CodeProjectionDiverged, "unsourced household rent runtime row requires manual audit")
		}
	}
	for _, key := range rpRentSortedKeys(mapKeysRentQueue(want.Queues)) {
		row := want.Queues[key]
		if _, err := conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(scheduler_item_id) DO UPDATE SET instance_id=excluded.instance_id,branch_id=excluded.branch_id,world_time=excluded.world_time,phase_id=excluded.phase_id,declared_priority=excluded.declared_priority,status=excluded.status,payload=excluded.payload`, row.ID, row.Instance, row.Branch, row.At, row.Phase, row.Priority, row.Status, row.Payload); err != nil {
			return err
		}
	}
	for _, key := range rpRentSortedKeys(mapKeysRentObligation(want.Obligations)) {
		row := want.Obligations[key]
		if _, err := conn.ExecContext(ctx, `INSERT INTO rent_obligations(obligation_id,contract_id,period_start_day,period_end_day,due_world_time,grace_until_world_time,amount_due_minor,amount_paid_minor,status,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(obligation_id) DO UPDATE SET contract_id=excluded.contract_id,period_start_day=excluded.period_start_day,period_end_day=excluded.period_end_day,due_world_time=excluded.due_world_time,grace_until_world_time=excluded.grace_until_world_time,amount_due_minor=excluded.amount_due_minor,amount_paid_minor=excluded.amount_paid_minor,status=excluded.status,last_event_sequence=excluded.last_event_sequence`, row.ID, row.Contract, row.StartDay, row.EndDay, row.DueAt, row.GraceAt, row.DueMinor, row.PaidMinor, row.Status, row.LastSeq); err != nil {
			return err
		}
	}
	for _, key := range rpRentSortedKeys(mapKeysRentSettlement(want.Settlements)) {
		row := want.Settlements[key]
		if _, err := conn.ExecContext(ctx, `INSERT INTO obligation_settlements(settlement_id,obligation_kind,obligation_id,attempted_minor,paid_minor,remaining_minor,status,reason_code,event_id,event_sequence) VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(settlement_id) DO UPDATE SET obligation_kind=excluded.obligation_kind,obligation_id=excluded.obligation_id,attempted_minor=excluded.attempted_minor,paid_minor=excluded.paid_minor,remaining_minor=excluded.remaining_minor,status=excluded.status,reason_code=excluded.reason_code,event_id=excluded.event_id,event_sequence=excluded.event_sequence`, row.ID, row.Kind, row.Obligation, row.Attempted, row.Paid, row.Remaining, row.Status, row.Reason, row.Event, row.Sequence); err != nil {
			return err
		}
	}
	return nil
}

func mapKeysRentQueue(rows map[string]rpRentQueueProjection) map[string]bool {
	keys := map[string]bool{}
	for key := range rows {
		keys[key] = true
	}
	return keys
}
func mapKeysRentObligation(rows map[string]rpRentObligationProjection) map[string]bool {
	keys := map[string]bool{}
	for key := range rows {
		keys[key] = true
	}
	return keys
}
func mapKeysRentSettlement(rows map[string]rpRentSettlementProjection) map[string]bool {
	keys := map[string]bool{}
	for key := range rows {
		keys[key] = true
	}
	return keys
}
