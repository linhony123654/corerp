package storage

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

type RPHouseholdRentShare struct {
	MemberEntityID string `json:"member_entity_id"`
	AmountMinor    int64  `json:"amount_minor"`
}

// The local operator defines a household rent agreement. This does not give
// either resident authority over the other's wallet; funding is a later,
// separately authorized command.
type RPHouseholdRentAgreementRequest struct {
	Binding      core.CareerBinding      `json:"binding"`
	HouseholdID  string                  `json:"household_id"`
	AgreementKey string                  `json:"agreement_key"`
	LandlordName string                  `json:"landlord_name"`
	RentMinor    int64                   `json:"rent_minor"`
	PeriodDays   int                     `json:"period_days"`
	GraceDays    int                     `json:"grace_days"`
	Shares       [2]RPHouseholdRentShare `json:"shares"`
}

type RPHouseholdRentAgreementFact struct {
	Version           string                  `json:"version"`
	AgreementID       string                  `json:"agreement_id"`
	HouseholdID       string                  `json:"household_id"`
	ContractID        string                  `json:"contract_id"`
	LandlordEntityID  string                  `json:"landlord_entity_id"`
	LandlordAccountID string                  `json:"landlord_account_id"`
	LandlordName      string                  `json:"landlord_name"`
	RentMinor         int64                   `json:"rent_minor"`
	PeriodDays        int                     `json:"period_days"`
	GraceDays         int                     `json:"grace_days"`
	StartsWorldDay    int                     `json:"starts_world_day"`
	CurrencyID        string                  `json:"currency_id"`
	Shares            [2]RPHouseholdRentShare `json:"shares"`
}

type RPHouseholdRentAgreementRecord = privateFactRecord[RPHouseholdRentAgreementFact]

func (s *Store) AgreeRPHouseholdRentLocal(ctx context.Context, r RPHouseholdRentAgreementRequest) (RPHouseholdRentAgreementRecord, error) {
	var empty RPHouseholdRentAgreementRecord
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.HouseholdID) || strings.TrimSpace(r.HouseholdID) != r.HouseholdID ||
		!rpLocationSlotKey.MatchString(r.AgreementKey) || !utf8.ValidString(r.LandlordName) ||
		strings.TrimSpace(r.LandlordName) != r.LandlordName || r.LandlordName == "" || len(r.LandlordName) > 128 ||
		r.RentMinor < 2 || r.RentMinor > core.MaxJSONSafeInteger || r.PeriodDays < 1 || r.PeriodDays > 366 || r.GraceDays < 0 || r.GraceDays > 30 ||
		r.Shares[0].AmountMinor < 1 || r.Shares[1].AmountMinor < 1 || r.Shares[0].AmountMinor > r.RentMinor ||
		r.Shares[1].AmountMinor != r.RentMinor-r.Shares[0].AmountMinor ||
		!studioID(r.Shares[0].MemberEntityID) || !studioID(r.Shares[1].MemberEntityID) ||
		r.Shares[0].MemberEntityID == r.Shares[1].MemberEntityID {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded household rent agreement and exact two-member shares required")
	}
	for _, ch := range r.LandlordName {
		if unicode.IsControl(ch) {
			return empty, core.NewError(core.CodeInvalidArgument, "landlord name contains control characters")
		}
	}
	identity, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.HouseholdID, r.AgreementKey})
	if err != nil {
		return empty, err
	}
	suffix := identity[7:31]
	return executePrivateFactCommand(s, ctx, r.Binding, "AgreeRPHouseholdRentLocal", r,
		privateFactDomain{"rp_household_rent", "RPHouseholdRentAgreed", `{"authorization":"local-operator-household-rent-v1"}`},
		func(conn *sql.Conn) error {
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.Binding.PrincipalID).Scan(&role); err != nil || role != "operator" {
				if err != nil && err != sql.ErrNoRows {
					return err
				}
				return core.NewError(core.CodeUnauthorized, "household rent agreement requires local operator")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPHouseholdRentAgreementFact, func() error, error) {
			var fact RPHouseholdRentAgreementFact
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			var rentAccount, currency string
			var startDay int
			if err := conn.QueryRowContext(ctx, `SELECT current_day FROM world_clocks WHERE instance_id=? AND branch_id=?`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&startDay); err != nil {
				return fact, nil, err
			}
			err := conn.QueryRowContext(ctx, `SELECT rent_account_id,currency_id FROM rp_households WHERE household_id=? AND instance_id=? AND branch_id=? AND status='active'`, r.HouseholdID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&rentAccount, &currency)
			if err != nil {
				return fact, nil, classifyMissing(err, "active household")
			}
			var existing int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_household_rent_agreements WHERE household_id=? AND status='active'`, r.HouseholdID).Scan(&existing); err != nil {
				return fact, nil, err
			}
			if existing != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "household already has active rent agreement")
			}
			shares := r.Shares
			sort.Slice(shares[:], func(i, j int) bool { return shares[i].MemberEntityID < shares[j].MemberEntityID })
			memberIDs := [2]string{}
			for i, share := range shares {
				err := conn.QueryRowContext(ctx, `SELECT membership_id FROM rp_household_memberships WHERE household_id=? AND instance_id=? AND branch_id=? AND member_entity_id=? AND member_role='adult' AND ended_event_id IS NULL`, r.HouseholdID, r.Binding.InstanceID, r.Binding.BranchID, share.MemberEntityID).Scan(&memberIDs[i])
				if err != nil {
					return fact, nil, classifyMissing(err, "active adult household member")
				}
			}
			fact = RPHouseholdRentAgreementFact{
				Version: "corerp.household.rent.v1", AgreementID: "agreement_household_rent_" + suffix,
				HouseholdID: r.HouseholdID, ContractID: "contract_household_rent_" + suffix,
				LandlordEntityID: "landlord_household_" + suffix, LandlordAccountID: "account_landlord_household_" + suffix,
				LandlordName: r.LandlordName, RentMinor: r.RentMinor, PeriodDays: r.PeriodDays,
				GraceDays: r.GraceDays, StartsWorldDay: startDay, CurrencyID: currency, Shares: shares,
			}
			return fact, func() error {
				if err := execAgentOne(ctx, conn, "create rent landlord account", `INSERT INTO accounts(account_id,owner_id,currency_id,account_type,overdraft_limit_minor,opened_by_event_id) VALUES (?,?,?,'landlord_cash',0,?)`, fact.LandlordAccountID, fact.LandlordEntityID, currency, c.EventID); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "create rent landlord balance", `INSERT INTO account_balances(account_id,balance_minor,projection_version,last_event_sequence) VALUES (?,0,0,?)`, fact.LandlordAccountID, c.Sequence); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "create rent landlord", `INSERT INTO economic_entities(entity_id,entity_kind,display_name,account_id,private_finances) VALUES (?,'landlord',?,?,1)`, fact.LandlordEntityID, fact.LandlordName, fact.LandlordAccountID); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "create shared rent contract", `INSERT INTO rent_contracts(contract_id,tenant_entity_id,landlord_entity_id,tenant_account_id,landlord_account_id,rent_minor,currency_id,period_days,grace_days,starts_on_day,status,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?,?,'active',?)`, fact.ContractID, r.HouseholdID, fact.LandlordEntityID, rentAccount, fact.LandlordAccountID, r.RentMinor, currency, r.PeriodDays, r.GraceDays, startDay, c.EventID); err != nil {
					return err
				}
				if err := createRPHouseholdRentLedger(ctx, conn, fact, c); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "project shared rent agreement", `INSERT INTO rp_household_rent_agreements(agreement_id,household_id,instance_id,branch_id,contract_id,landlord_entity_id,landlord_account_id,starts_world_time,starts_world_day,status,source_event_id) VALUES (?,?,?,?,?,?,?,?,?,'active',?)`, fact.AgreementID, fact.HouseholdID, r.Binding.InstanceID, r.Binding.BranchID, fact.ContractID, fact.LandlordEntityID, fact.LandlordAccountID, c.WorldTime, startDay, c.EventID); err != nil {
					return err
				}
				for i, share := range fact.Shares {
					if err := execAgentOne(ctx, conn, "project shared rent share", `INSERT INTO rp_household_rent_shares(agreement_id,membership_id,amount_minor,source_event_id) VALUES (?,?,?,?)`, fact.AgreementID, memberIDs[i], share.AmountMinor, c.EventID); err != nil {
						return err
					}
				}
				rulesHash, err := core.HashJSON("rp-household-rent-v1")
				if err != nil {
					return err
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO scheduler_phases(phase_id,description,ruleset_hash) VALUES (?,'RP household private rent accrual and settlement',?) ON CONFLICT(phase_id) DO NOTHING`, rpHouseholdRentPhase, rulesHash); err != nil {
					return err
				}
				return queueRPHouseholdRentPeriod(ctx, conn, fact.AgreementID, 1)
			}, nil
		})
}

func createRPHouseholdRentLedger(ctx context.Context, conn *sql.Conn, fact RPHouseholdRentAgreementFact, c privateFactContext) error {
	prefix := "account_rent_" + fact.ContractID
	for _, account := range []struct{ suffix, owner, kind string }{
		{"expense", fact.HouseholdID, "expense"}, {"payable", fact.HouseholdID, "liability"},
		{"receivable", fact.LandlordEntityID, "receivable"}, {"income", fact.LandlordEntityID, "income"},
	} {
		id := prefix + "_" + account.suffix
		if err := execAgentOne(ctx, conn, "open rent ledger account", `INSERT INTO accounts(account_id,owner_id,currency_id,account_type,overdraft_limit_minor,opened_by_event_id) VALUES (?,?,?,?,0,?)`, id, account.owner, fact.CurrencyID, account.kind, c.EventID); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "open rent ledger balance", `INSERT INTO account_balances(account_id,balance_minor,projection_version,last_event_sequence) VALUES (?,0,0,?)`, id, c.Sequence); err != nil {
			return err
		}
	}
	return execAgentOne(ctx, conn, "link rent ledger accounts", `INSERT INTO obligation_ledger_accounts(obligation_kind,contract_id,expense_account_id,payable_account_id,receivable_account_id,income_account_id,currency_id,definition_event_id) VALUES ('rent',?,?,?,?,?,?,?)`, fact.ContractID, prefix+"_expense", prefix+"_payable", prefix+"_receivable", prefix+"_income", fact.CurrencyID, c.EventID)
}
