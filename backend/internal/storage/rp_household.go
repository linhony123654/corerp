package storage

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

// The local operator only establishes a household. Membership does not
// transfer individual controller or account authority.
type RPHouseholdFoundRequest struct {
	Binding          core.CareerBinding `json:"binding"`
	HouseholdKey     string             `json:"household_key"`
	DisplayName      string             `json:"display_name"`
	ResidencePlaceID string             `json:"residence_place_id"`
	AdultEntityIDs   [2]string          `json:"adult_entity_ids"`
}

type RPHouseholdFoundFact struct {
	Version          string    `json:"version"`
	HouseholdID      string    `json:"household_id"`
	RentAccountID    string    `json:"rent_account_id"`
	DisplayName      string    `json:"display_name"`
	ResidencePlaceID string    `json:"residence_place_id"`
	CurrencyID       string    `json:"currency_id"`
	AdultEntityIDs   [2]string `json:"adult_entity_ids"`
}

type RPHouseholdFoundRecord = privateFactRecord[RPHouseholdFoundFact]

func validateRPHouseholdFoundRequest(r RPHouseholdFoundRequest) error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !rpLocationSlotKey.MatchString(r.HouseholdKey) || !studioID(r.ResidencePlaceID) ||
		strings.TrimSpace(r.ResidencePlaceID) != r.ResidencePlaceID ||
		!utf8.ValidString(r.DisplayName) || strings.TrimSpace(r.DisplayName) != r.DisplayName ||
		r.DisplayName == "" || len(r.DisplayName) > 128 ||
		!studioID(r.AdultEntityIDs[0]) || !studioID(r.AdultEntityIDs[1]) ||
		r.AdultEntityIDs[0] == r.AdultEntityIDs[1] {
		return core.NewError(core.CodeInvalidArgument, "bounded household key, name, residence and two distinct adults required")
	}
	for _, ch := range r.DisplayName {
		if unicode.IsControl(ch) {
			return core.NewError(core.CodeInvalidArgument, "household display name contains control characters")
		}
	}
	return nil
}

func (s *Store) FoundRPHouseholdLocal(ctx context.Context, r RPHouseholdFoundRequest) (RPHouseholdFoundRecord, error) {
	var empty RPHouseholdFoundRecord
	if err := validateRPHouseholdFoundRequest(r); err != nil {
		return empty, err
	}
	identity, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.HouseholdKey})
	if err != nil {
		return empty, err
	}
	suffix := identity[7:31]
	householdID := "household_" + suffix
	rentAccountID := "account_household_rent_" + suffix
	return executePrivateFactCommand(s, ctx, r.Binding, "FoundRPHouseholdLocal", r,
		privateFactDomain{"rp_household", "RPHouseholdFounded", `{"authorization":"local-operator-household-foundation-v1"}`},
		func(conn *sql.Conn) error {
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.Binding.PrincipalID).Scan(&role); err != nil || role != "operator" {
				if err != nil && err != sql.ErrNoRows {
					return err
				}
				return core.NewError(core.CodeUnauthorized, "household foundation requires the local operator")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPHouseholdFoundFact, func() error, error) {
			var fact RPHouseholdFoundFact
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			var placeCount int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=? AND status='active'`, r.ResidencePlaceID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&placeCount); err != nil {
				return fact, nil, err
			}
			if placeCount != 1 {
				return fact, nil, core.NewError(core.CodeNotFound, "active household residence not found in branch")
			}
			members := r.AdultEntityIDs
			sort.Strings(members[:])
			var currency string
			for _, member := range members {
				if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, member); err != nil {
					return fact, nil, err
				}
				var memberCurrency string
				if err := conn.QueryRowContext(ctx, `SELECT currency_id FROM materialized_entities WHERE entity_id=?`, member).Scan(&memberCurrency); err != nil {
					return fact, nil, err
				}
				if currency != "" && currency != memberCurrency {
					return fact, nil, core.NewError(core.CodeInvalidArgument, "household adults must share one currency")
				}
				currency = memberCurrency
			}
			var existing int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_households WHERE household_id=?`, householdID).Scan(&existing); err != nil {
				return fact, nil, err
			}
			if existing != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "household key already founded")
			}
			for _, member := range members {
				if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_household_memberships WHERE instance_id=? AND branch_id=? AND member_entity_id=? AND ended_event_id IS NULL`, r.Binding.InstanceID, r.Binding.BranchID, member).Scan(&existing); err != nil {
					return fact, nil, err
				}
				if existing != 0 {
					return fact, nil, core.NewError(core.CodeBranchConflict, "adult already belongs to an active household")
				}
			}
			fact = RPHouseholdFoundFact{"corerp.household.foundation.v1", householdID, rentAccountID, r.DisplayName, r.ResidencePlaceID, currency, members}
			return fact, func() error {
				if err := execAgentOne(ctx, conn, "open household rent account", `INSERT INTO accounts(account_id,owner_id,currency_id,account_type,overdraft_limit_minor,opened_by_event_id) VALUES (?,?,?,'household_rent_cash',0,?)`, rentAccountID, householdID, currency, c.EventID); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "project household rent account", `INSERT INTO account_balances(account_id,balance_minor,projection_version,last_event_sequence) VALUES (?,0,0,?)`, rentAccountID, c.Sequence); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "create household economic identity", `INSERT INTO economic_entities(entity_id,entity_kind,display_name,account_id,private_finances) VALUES (?,'household',?,?,1)`, householdID, r.DisplayName, rentAccountID); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "create household", `INSERT INTO rp_households(household_id,instance_id,branch_id,display_name,residence_place_id,rent_account_id,currency_id,status,source_event_id,started_world_time) VALUES (?,?,?,?,?,?,?,'active',?,?)`, householdID, r.Binding.InstanceID, r.Binding.BranchID, r.DisplayName, r.ResidencePlaceID, rentAccountID, currency, c.EventID, c.WorldTime); err != nil {
					return err
				}
				for index, member := range members {
					membershipID := householdID + "_adult_" + string(rune('0'+index))
					if err := execAgentOne(ctx, conn, "enroll household adult", `INSERT INTO rp_household_memberships(membership_id,household_id,instance_id,branch_id,member_entity_id,member_role,source_event_id,started_world_time) VALUES (?,?,?,?,?,'adult',?,?)`, membershipID, householdID, r.Binding.InstanceID, r.Binding.BranchID, member, c.EventID, c.WorldTime); err != nil {
						return err
					}
				}
				return nil
			}, nil
		})
}

// RPHouseholdView deliberately omits the other members' canonical IDs and
// personal finances. It is only available to a current, active member's RP
// session and is safe as a bounded input to that member's controller.
type RPHouseholdView struct {
	HouseholdID      string               `json:"household_id"`
	DisplayName      string               `json:"display_name"`
	ResidencePlaceID string               `json:"residence_place_id"`
	OwnRole          string               `json:"own_role"`
	MemberCount      int                  `json:"member_count"`
	RentFundMinor    int64                `json:"rent_fund_minor,string"`
	CurrencyID       string               `json:"currency_id"`
	WorldTime        string               `json:"world_time"`
	Rent             *RPHouseholdRentView `json:"rent,omitempty"`
}

// Only this member's share and aggregate household obligation/fund are shown.
// No other member Entity, personal account or source Event ID is returned.
type RPHouseholdRentView struct {
	AgreementID         string `json:"agreement_id"`
	PeriodIndex         int    `json:"period_index"`
	NextDueWorldTime    string `json:"next_due_world_time"`
	RentMinor           int64  `json:"rent_minor,string"`
	OwnShareMinor       int64  `json:"own_share_minor,string"`
	OwnContributedMinor int64  `json:"own_contributed_minor,string"`
	OwnRemainingMinor   int64  `json:"own_remaining_minor,string"`
}

func (s *Store) ReadRPHousehold(ctx context.Context, request core.RPSessionReadRequest, householdID string) (RPHouseholdView, error) {
	var out RPHouseholdView
	if err := request.Validate(); err != nil {
		return out, err
	}
	if !studioID(householdID) || strings.TrimSpace(householdID) != householdID {
		return out, core.NewError(core.CodeInvalidArgument, "bounded household identity required")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return out, err
	}
	if session.Status != "active" {
		return out, core.NewError(core.CodeBranchConflict, "household view requires an active session")
	}
	if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
		return out, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return out, err
	}
	err = tx.conn.QueryRowContext(ctx, `SELECT h.household_id,h.display_name,h.residence_place_id,m.member_role,
		(SELECT COUNT(*) FROM rp_household_memberships all_members WHERE all_members.household_id=h.household_id AND all_members.ended_event_id IS NULL),
		b.balance_minor,h.currency_id,c.current_world_time
		FROM rp_households h JOIN rp_household_memberships m ON m.household_id=h.household_id
		JOIN account_balances b ON b.account_id=h.rent_account_id
		JOIN world_clocks c ON c.instance_id=h.instance_id AND c.branch_id=h.branch_id
		WHERE h.household_id=? AND h.instance_id=? AND h.branch_id=? AND h.status='active'
		AND m.member_entity_id=? AND m.ended_event_id IS NULL`, householdID, session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(
		&out.HouseholdID, &out.DisplayName, &out.ResidencePlaceID, &out.OwnRole,
		&out.MemberCount, &out.RentFundMinor, &out.CurrencyID, &out.WorldTime)
	if err == sql.ErrNoRows {
		return RPHouseholdView{}, core.NewError(core.CodeNotFound, "authorized household not found")
	}
	if err != nil {
		return RPHouseholdView{}, err
	}
	var rent RPHouseholdRentView
	var starts string
	var periodDays int
	err = tx.conn.QueryRowContext(ctx, `SELECT a.agreement_id,a.starts_world_time,c.period_days,c.rent_minor,rs.amount_minor
		FROM rp_household_rent_agreements a JOIN rent_contracts c ON c.contract_id=a.contract_id
		JOIN rp_household_memberships m ON m.household_id=a.household_id
		JOIN rp_household_rent_shares rs ON rs.agreement_id=a.agreement_id AND rs.membership_id=m.membership_id
		WHERE a.household_id=? AND a.instance_id=? AND a.branch_id=? AND a.status='active' AND c.status='active'
		AND m.member_entity_id=? AND m.ended_event_id IS NULL`, householdID, session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(
		&rent.AgreementID, &starts, &periodDays, &rent.RentMinor, &rent.OwnShareMinor)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return RPHouseholdView{}, err
	}
	startTime, err := time.Parse(time.RFC3339, starts)
	if err != nil {
		return RPHouseholdView{}, err
	}
	current, err := time.Parse(time.RFC3339, out.WorldTime)
	if err != nil || current.Before(startTime) || periodDays < 1 {
		return RPHouseholdView{}, core.NewError(core.CodeProjectionDiverged, "invalid household rent clock")
	}
	period := time.Duration(periodDays) * 24 * time.Hour
	rent.PeriodIndex = int(current.Sub(startTime) / period)
	rent.NextDueWorldTime = startTime.Add(time.Duration(rent.PeriodIndex+1) * period).UTC().Format(time.RFC3339)
	if err := tx.conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(c.amount_minor),0) FROM rp_household_rent_contributions c
		JOIN rp_household_rent_shares rs ON rs.agreement_id=c.agreement_id AND rs.membership_id=c.membership_id
		JOIN rp_household_memberships m ON m.membership_id=rs.membership_id
		WHERE c.agreement_id=? AND m.member_entity_id=? AND c.period_index=?`, rent.AgreementID, session.ControlledEntityID, rent.PeriodIndex).Scan(&rent.OwnContributedMinor); err != nil {
		return RPHouseholdView{}, err
	}
	if rent.OwnContributedMinor < 0 || rent.OwnContributedMinor > rent.OwnShareMinor {
		return RPHouseholdView{}, core.NewError(core.CodeProjectionDiverged, "household rent contribution exceeds agreed share")
	}
	rent.OwnRemainingMinor = rent.OwnShareMinor - rent.OwnContributedMinor
	out.Rent = &rent
	return out, nil
}
