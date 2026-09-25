package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpHouseholdMemberProjection struct {
	ID        string `json:"id"`
	Entity    string `json:"entity"`
	Role      string `json:"role"`
	Supporter string `json:"supporter"`
	Source    string `json:"source"`
	StartedAt string `json:"started_at"`
	EndedBy   string `json:"ended_by"`
	EndedAt   string `json:"ended_at"`
}

type rpHouseholdProjection struct {
	ID                  string                        `json:"id"`
	Instance            string                        `json:"instance"`
	Branch              string                        `json:"branch"`
	Name                string                        `json:"name"`
	Residence           string                        `json:"residence"`
	RentAccount         string                        `json:"rent_account"`
	Currency            string                        `json:"currency"`
	Status              string                        `json:"status"`
	Source              string                        `json:"source"`
	StartedAt           string                        `json:"started_at"`
	EconomicKind        string                        `json:"economic_kind"`
	EconomicName        string                        `json:"economic_name"`
	EconomicAccount     string                        `json:"economic_account"`
	PrivateFinances     int                           `json:"private_finances"`
	AccountOwner        string                        `json:"account_owner"`
	AccountType         string                        `json:"account_type"`
	AccountCurrency     string                        `json:"account_currency"`
	AccountOpeningEvent string                        `json:"account_opening_event"`
	AccountOverdraft    int64                         `json:"account_overdraft"`
	AccountPolicy       string                        `json:"account_policy"`
	AccountClosedBy     string                        `json:"account_closed_by"`
	OpeningBalanceValid bool                          `json:"opening_balance_valid"`
	Members             []rpHouseholdMemberProjection `json:"members"`
}

func rpHouseholdExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpHouseholdProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPHouseholdFounded' ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	want := map[string]rpHouseholdProjection{}
	for rows.Next() {
		var eventID, worldTime, raw string
		if err := rows.Scan(&eventID, &worldTime, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var fact RPHouseholdFoundFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return nil, err
		}
		if fact.Version != "corerp.household.foundation.v1" || fact.HouseholdID == "" || fact.RentAccountID == "" || fact.AdultEntityIDs[0] == fact.AdultEntityIDs[1] {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid household foundation source")
		}
		if _, exists := want[fact.HouseholdID]; exists {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "household founded twice")
		}
		row := rpHouseholdProjection{
			ID: fact.HouseholdID, Instance: instance, Branch: branch, Name: fact.DisplayName,
			Residence: fact.ResidencePlaceID, RentAccount: fact.RentAccountID, Currency: fact.CurrencyID,
			Status: "active", Source: eventID, StartedAt: worldTime,
			EconomicKind: "household", EconomicName: fact.DisplayName, EconomicAccount: fact.RentAccountID,
			PrivateFinances: 1,
			AccountOwner:    fact.HouseholdID, AccountType: "household_rent_cash",
			AccountCurrency: fact.CurrencyID, AccountOpeningEvent: eventID, OpeningBalanceValid: true,
			Members: []rpHouseholdMemberProjection{},
		}
		for i, member := range fact.AdultEntityIDs {
			row.Members = append(row.Members, rpHouseholdMemberProjection{
				ID: fact.HouseholdID + "_adult_" + string(rune('0'+i)), Entity: member,
				Role: "adult", Source: eventID, StartedAt: worldTime,
			})
		}
		want[row.ID] = row
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	changes, err := q.QueryContext(ctx, `SELECT event_id,event_type,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPHouseholdDependentEnrolled','RPHouseholdMemberLeft') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	for changes.Next() {
		var eventID, kind, worldTime, raw string
		if err := changes.Scan(&eventID, &kind, &worldTime, &raw); err != nil {
			changes.Close()
			return nil, err
		}
		if kind == "RPHouseholdDependentEnrolled" {
			var fact RPHouseholdDependentFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				changes.Close()
				return nil, err
			}
			row, ok := want[fact.HouseholdID]
			if !ok || fact.Version != "corerp.household.dependent.v1" || fact.Reason == "" || fact.MembershipID == "" || fact.DependentEntityID == "" || worldTime < row.StartedAt {
				changes.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced household dependent")
			}
			supported := false
			for _, member := range row.Members {
				if member.ID == fact.SupporterMembershipID && member.Role == "adult" && member.EndedBy == "" {
					supported = true
				}
			}
			if !supported {
				changes.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "dependent has no sourced active adult supporter")
			}
			for _, home := range want {
				for _, member := range home.Members {
					if member.ID == fact.MembershipID || (member.Entity == fact.DependentEntityID && member.EndedBy == "") {
						changes.Close()
						return nil, core.NewError(core.CodeProjectionDiverged, "duplicate sourced dependent membership")
					}
				}
			}
			row.Members = append(row.Members, rpHouseholdMemberProjection{ID: fact.MembershipID, Entity: fact.DependentEntityID,
				Role: "dependent", Supporter: fact.SupporterMembershipID, Source: eventID, StartedAt: worldTime})
			want[row.ID] = row
			continue
		}
		var fact RPHouseholdMemberExitFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			changes.Close()
			return nil, err
		}
		row, ok := want[fact.HouseholdID]
		if !ok || fact.Version != "corerp.household.member_exit.v1" || fact.Reason == "" {
			changes.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced household member exit")
		}
		found := false
		for index := range row.Members {
			member := &row.Members[index]
			if member.ID != fact.MembershipID {
				continue
			}
			if member.Entity != fact.MemberEntityID || member.Role != "adult" || member.EndedBy != "" || worldTime < member.StartedAt {
				changes.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "household exit source differs from active member interval")
			}
			for _, dependent := range row.Members {
				if dependent.Role == "dependent" && dependent.Supporter == member.ID && dependent.EndedBy == "" {
					changes.Close()
					return nil, core.NewError(core.CodeProjectionDiverged, "adult exit strands sourced dependent")
				}
			}
			member.EndedBy, member.EndedAt = eventID, worldTime
			found = true
			break
		}
		if !found {
			changes.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "household exit has no sourced member")
		}
		want[row.ID] = row
	}
	err = changes.Err()
	changes.Close()
	for key, row := range want {
		sort.Slice(row.Members, func(i, j int) bool { return row.Members[i].ID < row.Members[j].ID })
		want[key] = row
	}
	return want, err
}

func rpHouseholdActual(ctx context.Context, q replayQuerier, instance, branch string) (map[string]rpHouseholdProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT h.household_id,h.instance_id,h.branch_id,h.display_name,h.residence_place_id,h.rent_account_id,h.currency_id,h.status,h.source_event_id,h.started_world_time,
		COALESCE(e.entity_kind,''),COALESCE(e.display_name,''),COALESCE(e.account_id,''),COALESCE(e.private_finances,-1),
		COALESCE(a.owner_id,''),COALESCE(a.account_type,''),COALESCE(a.currency_id,''),COALESCE(a.opened_by_event_id,''),
		COALESCE(a.overdraft_limit_minor,-1),COALESCE(a.overdraft_policy_id,''),COALESCE(a.closed_by_event_id,''),
		CASE WHEN b.account_id IS NULL THEN 0 WHEN EXISTS(SELECT 1 FROM postings p WHERE p.account_id=h.rent_account_id) THEN 1 WHEN b.balance_minor=0 THEN 1 ELSE 0 END
		FROM rp_households h LEFT JOIN economic_entities e ON e.entity_id=h.household_id
		LEFT JOIN accounts a ON a.account_id=h.rent_account_id LEFT JOIN account_balances b ON b.account_id=h.rent_account_id
		WHERE h.instance_id=? AND h.branch_id=? ORDER BY h.household_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	got := map[string]rpHouseholdProjection{}
	for rows.Next() {
		var row rpHouseholdProjection
		var openingValid int
		if err := rows.Scan(&row.ID, &row.Instance, &row.Branch, &row.Name, &row.Residence, &row.RentAccount, &row.Currency, &row.Status, &row.Source, &row.StartedAt, &row.EconomicKind, &row.EconomicName, &row.EconomicAccount, &row.PrivateFinances, &row.AccountOwner, &row.AccountType, &row.AccountCurrency, &row.AccountOpeningEvent, &row.AccountOverdraft, &row.AccountPolicy, &row.AccountClosedBy, &openingValid); err != nil {
			rows.Close()
			return nil, err
		}
		row.OpeningBalanceValid = openingValid == 1
		row.Members = []rpHouseholdMemberProjection{}
		got[row.ID] = row
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	memberRows, err := q.QueryContext(ctx, `SELECT m.membership_id,m.household_id,m.member_entity_id,m.member_role,COALESCE(m.supporter_membership_id,''),m.source_event_id,m.started_world_time,COALESCE(m.ended_event_id,''),COALESCE(m.ended_world_time,'') FROM rp_household_memberships m JOIN rp_households h ON h.household_id=m.household_id WHERE h.instance_id=? AND h.branch_id=? ORDER BY m.household_id,m.membership_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	for memberRows.Next() {
		var householdID string
		var member rpHouseholdMemberProjection
		if err := memberRows.Scan(&member.ID, &householdID, &member.Entity, &member.Role, &member.Supporter, &member.Source, &member.StartedAt, &member.EndedBy, &member.EndedAt); err != nil {
			memberRows.Close()
			return nil, err
		}
		row := got[householdID]
		row.Members = append(row.Members, member)
		got[householdID] = row
	}
	err = memberRows.Err()
	memberRows.Close()
	return got, err
}

func rpHouseholdProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpHouseholdExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	got, err := rpHouseholdActual(ctx, q, instance, branch)
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
			differences = append(differences, ProjectionDifference{Projection: "rp_household", Key: key, ExpectedText: expected, ActualText: actual})
		}
	}
	return differences, nil
}

func repairRPHouseholdProjections(ctx context.Context, conn *sql.Conn, instance, branch string, through int64, differences []ProjectionDifference) error {
	if len(differences) == 0 {
		return nil
	}
	want, err := rpHouseholdExpected(ctx, conn, instance, branch, through)
	if err != nil {
		return err
	}
	for _, difference := range differences {
		if _, ok := want[difference.Key]; !ok {
			return core.NewError(core.CodeProjectionDiverged, "unsourced household row requires manual audit")
		}
	}
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		row := want[key]
		if _, err := conn.ExecContext(ctx, `INSERT INTO accounts(account_id,owner_id,currency_id,account_type,overdraft_limit_minor,opened_by_event_id) VALUES (?,?,?,'household_rent_cash',0,?) ON CONFLICT(account_id) DO UPDATE SET owner_id=excluded.owner_id,currency_id=excluded.currency_id,account_type=excluded.account_type,overdraft_limit_minor=0,overdraft_policy_id=NULL,opened_by_event_id=excluded.opened_by_event_id,closed_by_event_id=NULL`, row.RentAccount, row.ID, row.Currency, row.Source); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO account_balances(account_id,balance_minor,projection_version,last_event_sequence) VALUES (?,0,0,?) ON CONFLICT(account_id) DO NOTHING`, row.RentAccount, through); err != nil {
			return err
		}
		var postings int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM postings WHERE account_id=?`, row.RentAccount).Scan(&postings); err != nil {
			return err
		}
		if postings == 0 {
			if _, err := conn.ExecContext(ctx, `UPDATE account_balances SET balance_minor=0 WHERE account_id=?`, row.RentAccount); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO economic_entities(entity_id,entity_kind,display_name,account_id,private_finances) VALUES (?,'household',?,?,1) ON CONFLICT(entity_id) DO UPDATE SET entity_kind='household',display_name=excluded.display_name,account_id=excluded.account_id,private_finances=1`, row.ID, row.Name, row.RentAccount); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO rp_households(household_id,instance_id,branch_id,display_name,residence_place_id,rent_account_id,currency_id,status,source_event_id,started_world_time) VALUES (?,?,?,?,?,?,?,'active',?,?) ON CONFLICT(household_id) DO UPDATE SET display_name=excluded.display_name,residence_place_id=excluded.residence_place_id,rent_account_id=excluded.rent_account_id,currency_id=excluded.currency_id,status='active',source_event_id=excluded.source_event_id,started_world_time=excluded.started_world_time`, row.ID, instance, branch, row.Name, row.Residence, row.RentAccount, row.Currency, row.Source, row.StartedAt); err != nil {
			return err
		}
		known := map[string]bool{}
		for _, member := range row.Members {
			known[member.ID] = true
		}
		membershipRows, err := conn.QueryContext(ctx, `SELECT membership_id FROM rp_household_memberships WHERE household_id=?`, row.ID)
		if err != nil {
			return err
		}
		for membershipRows.Next() {
			var id string
			if err := membershipRows.Scan(&id); err != nil {
				membershipRows.Close()
				return err
			}
			if !known[id] {
				membershipRows.Close()
				return core.NewError(core.CodeProjectionDiverged, "unsourced membership row requires manual audit")
			}
		}
		err = membershipRows.Err()
		membershipRows.Close()
		if err != nil {
			return err
		}
		for _, member := range row.Members {
			var endedBy, endedAt any
			var supporter any
			if member.Supporter != "" {
				supporter = member.Supporter
			}
			if member.EndedBy != "" {
				endedBy, endedAt = member.EndedBy, member.EndedAt
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO rp_household_memberships(membership_id,household_id,instance_id,branch_id,member_entity_id,member_role,supporter_membership_id,source_event_id,started_world_time,ended_event_id,ended_world_time) VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(membership_id) DO UPDATE SET member_entity_id=excluded.member_entity_id,member_role=excluded.member_role,supporter_membership_id=excluded.supporter_membership_id,source_event_id=excluded.source_event_id,started_world_time=excluded.started_world_time,ended_event_id=excluded.ended_event_id,ended_world_time=excluded.ended_world_time`, member.ID, row.ID, instance, branch, member.Entity, member.Role, supporter, member.Source, member.StartedAt, endedBy, endedAt); err != nil {
				return err
			}
		}
	}
	return nil
}
