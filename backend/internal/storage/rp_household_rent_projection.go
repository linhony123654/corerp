package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpRentAccountProjection struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	Kind         string `json:"kind"`
	Currency     string `json:"currency"`
	OpenedBy     string `json:"opened_by"`
	Overdraft    int64  `json:"overdraft"`
	Policy       string `json:"policy"`
	ClosedBy     string `json:"closed_by"`
	BalanceValid bool   `json:"balance_valid"`
}

type rpRentContributionProjection struct {
	ID           string `json:"id"`
	Membership   string `json:"membership"`
	Period       int    `json:"period"`
	Amount       int64  `json:"amount"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	Event        string `json:"event"`
	Journal      string `json:"journal"`
	JournalValid bool   `json:"journal_valid"`
}

type rpRentShareProjection struct {
	Membership string `json:"membership"`
	Amount     int64  `json:"amount"`
	Source     string `json:"source"`
}

type rpHouseholdRentProjection struct {
	ID                      string                         `json:"id"`
	Household               string                         `json:"household"`
	Instance                string                         `json:"instance"`
	Branch                  string                         `json:"branch"`
	Contract                string                         `json:"contract"`
	Landlord                string                         `json:"landlord"`
	LandlordAccount         string                         `json:"landlord_account"`
	LandlordName            string                         `json:"landlord_name"`
	RentAccount             string                         `json:"rent_account"`
	ContractTenant          string                         `json:"contract_tenant"`
	ContractLandlord        string                         `json:"contract_landlord"`
	ContractLandlordAccount string                         `json:"contract_landlord_account"`
	ContractStartsDay       int                            `json:"contract_starts_day"`
	Rent                    int64                          `json:"rent"`
	PeriodDays              int                            `json:"period_days"`
	GraceDays               int                            `json:"grace_days"`
	Currency                string                         `json:"currency"`
	Starts                  string                         `json:"starts"`
	StartsDay               int                            `json:"starts_day"`
	Status                  string                         `json:"status"`
	Source                  string                         `json:"source"`
	ContractSource          string                         `json:"contract_source"`
	ContractStatus          string                         `json:"contract_status"`
	LandlordKind            string                         `json:"landlord_kind"`
	LandlordPrivate         int                            `json:"landlord_private"`
	LandlordCash            string                         `json:"landlord_cash"`
	LedgerSource            string                         `json:"ledger_source"`
	LedgerExpense           string                         `json:"ledger_expense"`
	LedgerPayable           string                         `json:"ledger_payable"`
	LedgerReceivable        string                         `json:"ledger_receivable"`
	LedgerIncome            string                         `json:"ledger_income"`
	LedgerCurrency          string                         `json:"ledger_currency"`
	Accounts                []rpRentAccountProjection      `json:"accounts"`
	Shares                  []rpRentShareProjection        `json:"shares"`
	Contributions           []rpRentContributionProjection `json:"contributions"`
}

func rpRentAccountIDs(contractID, landlordAccount string) [5]string {
	prefix := "account_rent_" + contractID
	return [5]string{landlordAccount, prefix + "_expense", prefix + "_payable", prefix + "_receivable", prefix + "_income"}
}

func rpRentExpectedAccount(id, owner, kind, currency, source string) rpRentAccountProjection {
	return rpRentAccountProjection{ID: id, Owner: owner, Kind: kind, Currency: currency, OpenedBy: source, BalanceValid: true}
}

func rpHouseholdRentExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpHouseholdRentProjection, error) {
	households, err := rpHouseholdExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT event_id,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPHouseholdRentAgreed' ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	want := map[string]rpHouseholdRentProjection{}
	for rows.Next() {
		var eventID, worldTime, raw string
		if err := rows.Scan(&eventID, &worldTime, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var fact RPHouseholdRentAgreementFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return nil, err
		}
		home, ok := households[fact.HouseholdID]
		if !ok || fact.Version != "corerp.household.rent.v1" || fact.AgreementID == "" || fact.ContractID == "" || fact.CurrencyID != home.Currency || fact.Shares[0].AmountMinor+fact.Shares[1].AmountMinor != fact.RentMinor {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid household rent agreement source")
		}
		if _, duplicate := want[fact.AgreementID]; duplicate {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "duplicate household rent agreement source")
		}
		ids := rpRentAccountIDs(fact.ContractID, fact.LandlordAccountID)
		row := rpHouseholdRentProjection{
			ID: fact.AgreementID, Household: fact.HouseholdID, Instance: instance, Branch: branch,
			Contract: fact.ContractID, Landlord: fact.LandlordEntityID, LandlordAccount: fact.LandlordAccountID,
			LandlordName: fact.LandlordName, RentAccount: home.RentAccount, Rent: fact.RentMinor,
			ContractTenant: fact.HouseholdID, ContractLandlord: fact.LandlordEntityID,
			ContractLandlordAccount: fact.LandlordAccountID, ContractStartsDay: fact.StartsWorldDay,
			PeriodDays: fact.PeriodDays, GraceDays: fact.GraceDays, Currency: fact.CurrencyID,
			Starts: worldTime, StartsDay: fact.StartsWorldDay, Status: "active", Source: eventID, ContractSource: eventID,
			ContractStatus: "active", LandlordKind: "landlord", LandlordPrivate: 1,
			LandlordCash: fact.LandlordAccountID, LedgerSource: eventID,
			LedgerExpense: ids[1], LedgerPayable: ids[2], LedgerReceivable: ids[3], LedgerIncome: ids[4], LedgerCurrency: fact.CurrencyID,
			Accounts: []rpRentAccountProjection{
				rpRentExpectedAccount(ids[0], fact.LandlordEntityID, "landlord_cash", fact.CurrencyID, eventID),
				rpRentExpectedAccount(ids[1], fact.HouseholdID, "expense", fact.CurrencyID, eventID),
				rpRentExpectedAccount(ids[2], fact.HouseholdID, "liability", fact.CurrencyID, eventID),
				rpRentExpectedAccount(ids[3], fact.LandlordEntityID, "receivable", fact.CurrencyID, eventID),
				rpRentExpectedAccount(ids[4], fact.LandlordEntityID, "income", fact.CurrencyID, eventID),
			},
			Shares: []rpRentShareProjection{}, Contributions: []rpRentContributionProjection{},
		}
		for _, share := range fact.Shares {
			memberID := ""
			for _, member := range home.Members {
				if member.Entity == share.MemberEntityID {
					memberID = member.ID
				}
			}
			if memberID == "" {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "rent share references nonmember")
			}
			row.Shares = append(row.Shares, rpRentShareProjection{memberID, share.AmountMinor, eventID})
		}
		sort.Slice(row.Shares, func(i, j int) bool { return row.Shares[i].Membership < row.Shares[j].Membership })
		want[row.ID] = row
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	contributions, err := q.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPHouseholdRentContributed' ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	for contributions.Next() {
		var eventID, raw string
		if err := contributions.Scan(&eventID, &raw); err != nil {
			contributions.Close()
			return nil, err
		}
		var fact RPHouseholdRentContributionFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			contributions.Close()
			return nil, err
		}
		row, ok := want[fact.AgreementID]
		if !ok || fact.Version != "corerp.household.contribution.v1" || fact.HouseholdID != row.Household || fact.TargetAccount != row.RentAccount || fact.CurrencyID != row.Currency || fact.AmountMinor < 1 {
			contributions.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid household rent contribution source")
		}
		row.Contributions = append(row.Contributions, rpRentContributionProjection{fact.ContributionID, fact.MembershipID, fact.PeriodIndex, fact.AmountMinor, fact.SourceAccount, fact.TargetAccount, eventID, "journal_" + eventID, true})
		want[row.ID] = row
	}
	err = contributions.Err()
	contributions.Close()
	if err != nil {
		return nil, err
	}
	for key, row := range want {
		sort.Slice(row.Contributions, func(i, j int) bool { return row.Contributions[i].ID < row.Contributions[j].ID })
		want[key] = row
	}
	return want, nil
}

func rpRentAccountActual(ctx context.Context, q replayQuerier, id string) (rpRentAccountProjection, error) {
	var row rpRentAccountProjection
	var valid int
	err := q.QueryRowContext(ctx, `SELECT COALESCE(a.account_id,''),COALESCE(a.owner_id,''),COALESCE(a.account_type,''),COALESCE(a.currency_id,''),COALESCE(a.opened_by_event_id,''),COALESCE(a.overdraft_limit_minor,-1),COALESCE(a.overdraft_policy_id,''),COALESCE(a.closed_by_event_id,''),
		CASE WHEN b.account_id IS NULL THEN 0 WHEN EXISTS(SELECT 1 FROM postings p WHERE p.account_id=?) THEN 1 WHEN b.balance_minor=0 THEN 1 ELSE 0 END
		FROM (SELECT ? AS id) x LEFT JOIN accounts a ON a.account_id=x.id LEFT JOIN account_balances b ON b.account_id=x.id`, id, id).Scan(&row.ID, &row.Owner, &row.Kind, &row.Currency, &row.OpenedBy, &row.Overdraft, &row.Policy, &row.ClosedBy, &valid)
	row.BalanceValid = valid == 1
	return row, err
}

func rpHouseholdRentActual(ctx context.Context, q replayQuerier, instance, branch string) (map[string]rpHouseholdRentProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT a.agreement_id,a.household_id,a.instance_id,a.branch_id,a.contract_id,a.landlord_entity_id,a.landlord_account_id,a.starts_world_time,a.starts_world_day,a.status,a.source_event_id,
		COALESCE(c.tenant_account_id,''),COALESCE(c.tenant_entity_id,''),COALESCE(c.landlord_entity_id,''),COALESCE(c.landlord_account_id,''),COALESCE(c.starts_on_day,-1),
		COALESCE(c.rent_minor,0),COALESCE(c.period_days,0),COALESCE(c.grace_days,0),COALESCE(c.currency_id,''),COALESCE(c.definition_event_id,''),COALESCE(c.status,''),
		COALESCE(e.entity_kind,''),COALESCE(e.display_name,''),COALESCE(e.private_finances,-1),COALESCE(e.account_id,''),COALESCE(l.definition_event_id,''),
		COALESCE(l.expense_account_id,''),COALESCE(l.payable_account_id,''),COALESCE(l.receivable_account_id,''),COALESCE(l.income_account_id,''),COALESCE(l.currency_id,'')
		FROM rp_household_rent_agreements a LEFT JOIN rent_contracts c ON c.contract_id=a.contract_id
		LEFT JOIN economic_entities e ON e.entity_id=a.landlord_entity_id
		LEFT JOIN obligation_ledger_accounts l ON l.obligation_kind='rent' AND l.contract_id=a.contract_id
		WHERE a.instance_id=? AND a.branch_id=? ORDER BY a.agreement_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	got := map[string]rpHouseholdRentProjection{}
	for rows.Next() {
		var row rpHouseholdRentProjection
		if err := rows.Scan(&row.ID, &row.Household, &row.Instance, &row.Branch, &row.Contract, &row.Landlord, &row.LandlordAccount, &row.Starts, &row.StartsDay, &row.Status, &row.Source,
			&row.RentAccount, &row.ContractTenant, &row.ContractLandlord, &row.ContractLandlordAccount, &row.ContractStartsDay,
			&row.Rent, &row.PeriodDays, &row.GraceDays, &row.Currency, &row.ContractSource, &row.ContractStatus,
			&row.LandlordKind, &row.LandlordName, &row.LandlordPrivate, &row.LandlordCash, &row.LedgerSource,
			&row.LedgerExpense, &row.LedgerPayable, &row.LedgerReceivable, &row.LedgerIncome, &row.LedgerCurrency); err != nil {
			rows.Close()
			return nil, err
		}
		row.Accounts = []rpRentAccountProjection{}
		row.Shares = []rpRentShareProjection{}
		row.Contributions = []rpRentContributionProjection{}
		got[row.ID] = row
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for key, row := range got {
		ids := rpRentAccountIDs(row.Contract, row.LandlordAccount)
		for _, id := range ids {
			account, err := rpRentAccountActual(ctx, q, id)
			if err != nil {
				return nil, err
			}
			row.Accounts = append(row.Accounts, account)
		}
		shares, err := q.QueryContext(ctx, `SELECT membership_id,amount_minor,source_event_id FROM rp_household_rent_shares WHERE agreement_id=? ORDER BY membership_id`, key)
		if err != nil {
			return nil, err
		}
		for shares.Next() {
			var share rpRentShareProjection
			if err := shares.Scan(&share.Membership, &share.Amount, &share.Source); err != nil {
				shares.Close()
				return nil, err
			}
			row.Shares = append(row.Shares, share)
		}
		err = shares.Err()
		shares.Close()
		if err != nil {
			return nil, err
		}
		contributions, err := q.QueryContext(ctx, `SELECT contribution_id,membership_id,period_index,amount_minor,source_account_id,target_account_id,event_id,journal_entry_id FROM rp_household_rent_contributions WHERE agreement_id=? ORDER BY contribution_id`, key)
		if err != nil {
			return nil, err
		}
		for contributions.Next() {
			var contribution rpRentContributionProjection
			if err := contributions.Scan(&contribution.ID, &contribution.Membership, &contribution.Period, &contribution.Amount, &contribution.Source, &contribution.Target, &contribution.Event, &contribution.Journal); err != nil {
				contributions.Close()
				return nil, err
			}
			row.Contributions = append(row.Contributions, contribution)
		}
		err = contributions.Err()
		contributions.Close()
		if err != nil {
			return nil, err
		}
		for i := range row.Contributions {
			p := &row.Contributions[i]
			var valid int
			err := q.QueryRowContext(ctx, `SELECT CASE WHEN j.event_id=? AND j.status='posted' AND j.purpose='RP household rent contribution' AND
				(SELECT COUNT(*) FROM postings WHERE entry_id=j.entry_id)=2 AND
				(SELECT COUNT(*) FROM postings WHERE entry_id=j.entry_id AND account_id=? AND currency_id=? AND amount_minor=?)=1 AND
				(SELECT COUNT(*) FROM postings WHERE entry_id=j.entry_id AND account_id=? AND currency_id=? AND amount_minor=?)=1 THEN 1 ELSE 0 END
				FROM journal_entries j WHERE j.entry_id=?`, p.Event, p.Source, row.Currency, -p.Amount, p.Target, row.Currency, p.Amount, p.Journal).Scan(&valid)
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
			p.JournalValid = err == nil && valid == 1
		}
		got[key] = row
	}
	return got, nil
}

func rpHouseholdRentProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpHouseholdRentExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	got, err := rpHouseholdRentActual(ctx, q, instance, branch)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for key := range want {
		keys[key] = true
	}
	for key := range got {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var differences []ProjectionDifference
	for _, key := range ordered {
		expected, actual := "missing", "missing"
		if row, ok := want[key]; ok {
			encoded, err := core.CanonicalJSON(row)
			if err != nil {
				return nil, err
			}
			expected = string(encoded)
		}
		if row, ok := got[key]; ok {
			encoded, err := core.CanonicalJSON(row)
			if err != nil {
				return nil, err
			}
			actual = string(encoded)
		}
		if expected != actual {
			differences = append(differences, ProjectionDifference{Projection: "rp_household_rent", Key: key, ExpectedText: expected, ActualText: actual})
		}
	}
	return differences, nil
}

func repairRPHouseholdRentProjections(ctx context.Context, conn *sql.Conn, instance, branch string, through int64, differences []ProjectionDifference) error {
	if len(differences) == 0 {
		return nil
	}
	want, err := rpHouseholdRentExpected(ctx, conn, instance, branch, through)
	if err != nil {
		return err
	}
	for _, difference := range differences {
		if _, ok := want[difference.Key]; !ok {
			return core.NewError(core.CodeProjectionDiverged, "unsourced household rent row requires manual audit")
		}
	}
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		row := want[key]
		for _, account := range row.Accounts {
			if _, err := conn.ExecContext(ctx, `INSERT INTO accounts(account_id,owner_id,currency_id,account_type,overdraft_limit_minor,opened_by_event_id) VALUES (?,?,?,?,0,?) ON CONFLICT(account_id) DO UPDATE SET owner_id=excluded.owner_id,currency_id=excluded.currency_id,account_type=excluded.account_type,overdraft_limit_minor=0,overdraft_policy_id=NULL,opened_by_event_id=excluded.opened_by_event_id,closed_by_event_id=NULL`, account.ID, account.Owner, account.Currency, account.Kind, account.OpenedBy); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO account_balances(account_id,balance_minor,projection_version,last_event_sequence) VALUES (?,0,0,?) ON CONFLICT(account_id) DO NOTHING`, account.ID, through); err != nil {
				return err
			}
			var postings int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM postings WHERE account_id=?`, account.ID).Scan(&postings); err != nil {
				return err
			}
			if postings == 0 {
				if _, err := conn.ExecContext(ctx, `UPDATE account_balances SET balance_minor=0 WHERE account_id=?`, account.ID); err != nil {
					return err
				}
			}
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO economic_entities(entity_id,entity_kind,display_name,account_id,private_finances) VALUES (?,'landlord',?,?,1) ON CONFLICT(entity_id) DO UPDATE SET entity_kind='landlord',display_name=excluded.display_name,account_id=excluded.account_id,private_finances=1`, row.Landlord, row.LandlordName, row.LandlordAccount); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO rent_contracts(contract_id,tenant_entity_id,landlord_entity_id,tenant_account_id,landlord_account_id,rent_minor,currency_id,period_days,grace_days,starts_on_day,status,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?,?,'active',?) ON CONFLICT(contract_id) DO UPDATE SET tenant_entity_id=excluded.tenant_entity_id,landlord_entity_id=excluded.landlord_entity_id,tenant_account_id=excluded.tenant_account_id,landlord_account_id=excluded.landlord_account_id,rent_minor=excluded.rent_minor,currency_id=excluded.currency_id,period_days=excluded.period_days,grace_days=excluded.grace_days,starts_on_day=excluded.starts_on_day,status='active',definition_event_id=excluded.definition_event_id`, row.Contract, row.Household, row.Landlord, row.RentAccount, row.LandlordAccount, row.Rent, row.Currency, row.PeriodDays, row.GraceDays, row.StartsDay, row.Source); err != nil {
			return err
		}
		ids := rpRentAccountIDs(row.Contract, row.LandlordAccount)
		if _, err := conn.ExecContext(ctx, `INSERT INTO obligation_ledger_accounts(obligation_kind,contract_id,expense_account_id,payable_account_id,receivable_account_id,income_account_id,currency_id,definition_event_id) VALUES ('rent',?,?,?,?,?,?,?) ON CONFLICT(obligation_kind,contract_id) DO UPDATE SET expense_account_id=excluded.expense_account_id,payable_account_id=excluded.payable_account_id,receivable_account_id=excluded.receivable_account_id,income_account_id=excluded.income_account_id,currency_id=excluded.currency_id,definition_event_id=excluded.definition_event_id`, row.Contract, ids[1], ids[2], ids[3], ids[4], row.Currency, row.Source); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO rp_household_rent_agreements(agreement_id,household_id,instance_id,branch_id,contract_id,landlord_entity_id,landlord_account_id,starts_world_time,starts_world_day,status,source_event_id) VALUES (?,?,?,?,?,?,?,?,?,'active',?) ON CONFLICT(agreement_id) DO UPDATE SET household_id=excluded.household_id,instance_id=excluded.instance_id,branch_id=excluded.branch_id,contract_id=excluded.contract_id,landlord_entity_id=excluded.landlord_entity_id,landlord_account_id=excluded.landlord_account_id,starts_world_time=excluded.starts_world_time,starts_world_day=excluded.starts_world_day,status='active',source_event_id=excluded.source_event_id,ended_event_id=NULL`, row.ID, row.Household, instance, branch, row.Contract, row.Landlord, row.LandlordAccount, row.Starts, row.StartsDay, row.Source); err != nil {
			return err
		}
		knownShares := map[string]bool{}
		for _, share := range row.Shares {
			knownShares[share.Membership] = true
		}
		shareRows, err := conn.QueryContext(ctx, `SELECT membership_id FROM rp_household_rent_shares WHERE agreement_id=?`, row.ID)
		if err != nil {
			return err
		}
		for shareRows.Next() {
			var id string
			if err := shareRows.Scan(&id); err != nil {
				shareRows.Close()
				return err
			}
			if !knownShares[id] {
				shareRows.Close()
				return core.NewError(core.CodeProjectionDiverged, "unsourced rent share requires manual audit")
			}
		}
		err = shareRows.Err()
		shareRows.Close()
		if err != nil {
			return err
		}
		for _, share := range row.Shares {
			if _, err := conn.ExecContext(ctx, `INSERT INTO rp_household_rent_shares(agreement_id,membership_id,amount_minor,source_event_id) VALUES (?,?,?,?) ON CONFLICT(agreement_id,membership_id) DO UPDATE SET amount_minor=excluded.amount_minor,source_event_id=excluded.source_event_id`, row.ID, share.Membership, share.Amount, share.Source); err != nil {
				return err
			}
		}
		knownContributions := map[string]bool{}
		for _, contribution := range row.Contributions {
			knownContributions[contribution.ID] = true
		}
		contributionRows, err := conn.QueryContext(ctx, `SELECT contribution_id FROM rp_household_rent_contributions WHERE agreement_id=?`, row.ID)
		if err != nil {
			return err
		}
		for contributionRows.Next() {
			var id string
			if err := contributionRows.Scan(&id); err != nil {
				contributionRows.Close()
				return err
			}
			if !knownContributions[id] {
				contributionRows.Close()
				return core.NewError(core.CodeProjectionDiverged, "unsourced rent contribution requires manual audit")
			}
		}
		err = contributionRows.Err()
		contributionRows.Close()
		if err != nil {
			return err
		}
		for _, contribution := range row.Contributions {
			if _, err := conn.ExecContext(ctx, `INSERT INTO rp_household_rent_contributions(contribution_id,agreement_id,membership_id,period_index,amount_minor,source_account_id,target_account_id,event_id,journal_entry_id) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(contribution_id) DO UPDATE SET agreement_id=excluded.agreement_id,membership_id=excluded.membership_id,period_index=excluded.period_index,amount_minor=excluded.amount_minor,source_account_id=excluded.source_account_id,target_account_id=excluded.target_account_id,event_id=excluded.event_id,journal_entry_id=excluded.journal_entry_id`, contribution.ID, row.ID, contribution.Membership, contribution.Period, contribution.Amount, contribution.Source, contribution.Target, contribution.Event, contribution.Journal); err != nil {
				return err
			}
		}
	}
	return nil
}
