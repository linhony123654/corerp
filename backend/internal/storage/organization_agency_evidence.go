package storage

import (
	"context"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// Derive account membership and workforce from Events, not a scan of mutable
// contracts/obligations (which cannot detect deleted rows). Liability balances
// come from posted journals, including partial, late and estate settlements.
// through also permits auditing the evidence as it stood before a review.
func organizationBusinessSnapshot(ctx context.Context, q replayQuerier, instance, branch, org, currency, at string, through int64) (int64, int, error) {
	now, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return 0, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_type,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND (event_type IN ('M2BackgroundEconomyDefined','CareerEmploymentTermsActivated','CareerAggregateExitActivated') OR (event_type='RPCareerFactRecorded' AND json_extract(payload,'$.organization_id')=? AND json_extract(payload,'$.kind')='offer' AND json_extract(payload,'$.offer.status')='accepted')) ORDER BY event_sequence`, instance, branch, through, org)
	if err != nil {
		return 0, 0, err
	}
	accounts := map[string]bool{}
	jobs := map[string]CareerEmploymentFact{}
	ended := map[string]bool{}
	departures := map[string]bool{}
	cohortWorkers := 0
	invalid := func() error {
		return core.NewError(core.CodeProjectionDiverged, "organization business source differs")
	}
	for rows.Next() {
		var id, kind, raw string
		if err := rows.Scan(&id, &kind, &raw); err != nil {
			rows.Close()
			return 0, 0, err
		}
		switch kind {
		case "M2BackgroundEconomyDefined":
			var source struct {
				Actor    string `json:"actor_id"`
				Currency string `json:"currency_id"`
				Workers  int    `json:"background_workers"`
				From     string `json:"effective_from"`
				Until    string `json:"wage_effective_until"`
			}
			if json.Unmarshal([]byte(raw), &source) != nil {
				rows.Close()
				return 0, 0, invalid()
			}
			if source.Actor != org {
				continue
			}
			from, e1 := time.Parse(time.RFC3339Nano, source.From)
			until, e2 := time.Parse(time.RFC3339Nano, source.Until)
			if id != m2EconomyEventID || instance != M2DemoInstanceID || branch != M2DemoBranchID || source.Currency != currency || source.Workers < 1 || source.Workers > 10000 || e1 != nil || e2 != nil || !until.After(from) {
				rows.Close()
				return 0, 0, invalid()
			}
			accounts[m2EconomyEmployerPayable] = true
			if !now.Before(from) && now.Before(until) {
				cohortWorkers = source.Workers
			}
		case "RPCareerFactRecorded":
			var f CareerFact
			if json.Unmarshal([]byte(raw), &f) != nil {
				rows.Close()
				return 0, 0, invalid()
			}
			if f.OrganizationID != org || f.Kind != "offer" || f.Offer == nil || f.Offer.Status != "accepted" {
				continue
			}
			job := f.Employment
			if job == nil || job.ContractID == "" || job.ContractID != f.Offer.ContractID || job.OrganizationID != org || job.CurrencyID != currency {
				rows.Close()
				return 0, 0, invalid()
			}
			if _, exists := jobs[job.ContractID]; exists {
				rows.Close()
				return 0, 0, invalid()
			}
			jobs[job.ContractID] = *job
			accounts["account_wage_"+job.ContractID+"_payable"] = true
		case "CareerEmploymentTermsActivated":
			var source struct {
				ContractID string `json:"contract_id"`
				Status     string `json:"contract_status"`
			}
			if json.Unmarshal([]byte(raw), &source) != nil {
				rows.Close()
				return 0, 0, invalid()
			}
			if source.Status == "ended" {
				ended[source.ContractID] = true
			}
		case "CareerAggregateExitActivated":
			var source careerAggregateActivation
			if json.Unmarshal([]byte(raw), &source) != nil {
				rows.Close()
				return 0, 0, invalid()
			}
			if source.ContractID == m2EconomyContractID {
				departures[source.MaterializationID] = true
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, 0, err
	}
	staff := 0
	if cohortWorkers > 0 {
		staff = cohortWorkers - len(departures)
		if staff < 0 {
			return 0, 0, invalid()
		}
	}
	for id, job := range jobs {
		start, err := time.Parse(time.RFC3339, careerTime(job.StartsOnDay, 0, 0))
		if err != nil {
			return 0, 0, err
		}
		if !ended[id] && !now.Before(start) {
			staff++
		}
	}
	var payable int64
	for account := range accounts {
		var balance int64
		if err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(p.amount_minor),0) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id AND j.status='posted' JOIN events e ON e.event_id=j.event_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND p.account_id=?`, instance, branch, through, account).Scan(&balance); err != nil {
			return 0, 0, err
		}
		if balance > 0 || balance < -core.MaxJSONSafeInteger {
			return 0, 0, invalid()
		}
		var ok bool
		payable, ok = checkedAdd(payable, -balance)
		if !ok || payable > core.MaxJSONSafeInteger {
			return 0, 0, core.NewError(core.CodeIntegerOverflow, "organization wage payables overflow")
		}
	}
	return payable, staff, nil
}

func verifyOrganizationReviewBusinessSource(ctx context.Context, q replayQuerier, instance, branch string, through int64, review core.OrganizationReviewResult, outcome *core.CareerPostingDefinition, position string) error {
	var raw string
	if err := q.QueryRowContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='organization' AND json_extract(payload,'$.record_id')=? ORDER BY event_sequence DESC LIMIT 1`, instance, branch, through, review.OrganizationID).Scan(&raw); err != nil {
		return err
	}
	var source CareerFact
	if json.Unmarshal([]byte(raw), &source) != nil || source.Organization == nil {
		return core.NewError(core.CodeProjectionDiverged, "review organization source missing")
	}
	org := source.Organization
	payable, staff, err := organizationBusinessSnapshot(ctx, q, instance, branch, review.OrganizationID, org.CurrencyID, review.WorldTime, through)
	if err != nil {
		return err
	}
	var cash int64
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(p.amount_minor),0) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id AND j.status='posted' JOIN events e ON e.event_id=j.event_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND p.account_id=?`, instance, branch, through, org.CashAccountID).Scan(&cash); err != nil {
		return err
	}
	if payable != review.Evidence.WagePayablesMinor || staff != review.Evidence.ActiveEmployees || cash != review.Evidence.CashBalanceMinor {
		return core.NewError(core.CodeProjectionDiverged, "review business evidence differs from historical source")
	}
	if err := q.QueryRowContext(ctx, `SELECT json_extract(payload,'$.posting') FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind') IN ('posting','organization_review') AND json_extract(payload,'$.posting.position_id')=? ORDER BY event_sequence DESC LIMIT 1`, instance, branch, through, position).Scan(&raw); err != nil {
		return err
	}
	var posting core.CareerPostingDefinition
	if json.Unmarshal([]byte(raw), &posting) != nil || posting.OrganizationID != review.OrganizationID {
		return core.NewError(core.CodeProjectionDiverged, "review posting source missing")
	}
	status := posting.Status
	if status == "" {
		status = "active"
	}
	if posting.Capacity != review.Evidence.CurrentCapacity || status != review.Evidence.PostingStatus || (outcome != nil && !careerPostingTermsEqual(posting, *outcome)) {
		return core.NewError(core.CodeProjectionDiverged, "review posting terms differ from source")
	}
	return nil
}
