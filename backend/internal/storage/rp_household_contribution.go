package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPHouseholdRentContributionRequest struct {
	Binding     core.CareerBinding `json:"binding"`
	SessionID   string             `json:"session_id"`
	AgreementID string             `json:"agreement_id"`
	PeriodIndex int                `json:"period_index"`
	AmountMinor int64              `json:"amount_minor"`
}

type RPHouseholdRentContributionFact struct {
	Version        string `json:"version"`
	ContributionID string `json:"contribution_id"`
	AgreementID    string `json:"agreement_id"`
	HouseholdID    string `json:"household_id"`
	MembershipID   string `json:"membership_id"`
	MemberEntityID string `json:"member_entity_id"`
	PeriodIndex    int    `json:"period_index"`
	AmountMinor    int64  `json:"amount_minor"`
	SourceAccount  string `json:"source_account_id"`
	TargetAccount  string `json:"target_account_id"`
	CurrencyID     string `json:"currency_id"`
}

type RPHouseholdRentContributionRecord = privateFactRecord[RPHouseholdRentContributionFact]

func (s *Store) ContributeRPHouseholdRent(ctx context.Context, r RPHouseholdRentContributionRequest) (RPHouseholdRentContributionRecord, error) {
	var empty RPHouseholdRentContributionRecord
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.SessionID) || !studioID(r.AgreementID) || strings.TrimSpace(r.SessionID) != r.SessionID || strings.TrimSpace(r.AgreementID) != r.AgreementID ||
		r.PeriodIndex < 0 || r.PeriodIndex > 100000 || r.AmountMinor < 1 || r.AmountMinor > core.MaxJSONSafeInteger {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded household contribution request required")
	}
	identity, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.Binding.PrincipalID, r.SessionID, r.Binding.IdempotencyKey})
	if err != nil {
		return empty, err
	}
	contributionID := "contribution_household_rent_" + identity[7:31]
	// Exact-key receipt remains available to the original session principal
	// after loss of control, but fresh writes require live control and membership.
	replayAuth := func(conn *sql.Conn) error {
		session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
		if err != nil {
			return err
		}
		if session.InstanceID != r.Binding.InstanceID || session.BranchID != r.Binding.BranchID {
			return core.NewError(core.CodeNotFound, "session is outside requested world branch")
		}
		return nil
	}
	return executePrivateFactCommandWithOptions(s, ctx, r.Binding, "ContributeRPHouseholdRent", r,
		privateFactDomain{"rp_household_contribution", "RPHouseholdRentContributed", `{"authorization":"current-member-own-asset-account-v1"}`},
		privateFactOptions{replayAuthorize: replayAuth},
		func(conn *sql.Conn) error {
			if err := replayAuth(conn); err != nil {
				return err
			}
			session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return err
			}
			if session.Status != "active" {
				return core.NewError(core.CodeBranchConflict, "rent contribution requires active session")
			}
			if err := requireCurrentRPSession(ctx, conn, session); err != nil {
				return err
			}
			if err := authorizeRPControl(ctx, conn, r.Binding.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
				return err
			}
			if session.ObservationCursor != r.Binding.ExpectedHead {
				return core.NewError(core.CodeBranchConflict, "observe current world before funding household rent")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPHouseholdRentContributionFact, func() error, error) {
			var fact RPHouseholdRentContributionFact
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return fact, nil, err
			}
			var householdID, memberID, memberAccount, rentAccount, currency, starts string
			var periodDays int
			var share int64
			err = conn.QueryRowContext(ctx, `SELECT h.household_id,m.membership_id,n.asset_account_id,h.rent_account_id,h.currency_id,a.starts_world_time,c.period_days,rs.amount_minor
				FROM rp_household_rent_agreements a JOIN rp_households h ON h.household_id=a.household_id
				JOIN rp_household_memberships m ON m.household_id=h.household_id
				JOIN rp_household_rent_shares rs ON rs.agreement_id=a.agreement_id AND rs.membership_id=m.membership_id
				JOIN materialized_entities n ON n.entity_id=m.member_entity_id
				JOIN rent_contracts c ON c.contract_id=a.contract_id
				WHERE a.agreement_id=? AND a.instance_id=? AND a.branch_id=? AND a.status='active'
				AND h.status='active' AND c.status='active' AND m.member_entity_id=? AND m.ended_event_id IS NULL`,
				r.AgreementID, r.Binding.InstanceID, r.Binding.BranchID, session.ControlledEntityID).Scan(&householdID, &memberID, &memberAccount, &rentAccount, &currency, &starts, &periodDays, &share)
			if err != nil {
				return fact, nil, classifyMissing(err, "active member rent agreement")
			}
			startTime, err := time.Parse(time.RFC3339, starts)
			if err != nil {
				return fact, nil, err
			}
			worldTime, err := time.Parse(time.RFC3339, c.WorldTime)
			if err != nil {
				return fact, nil, err
			}
			if worldTime.Before(startTime) || periodDays < 1 || int(worldTime.Sub(startTime)/(time.Duration(periodDays)*24*time.Hour)) != r.PeriodIndex {
				return fact, nil, core.NewError(core.CodeBranchConflict, "rent contribution targets a noncurrent period")
			}
			var already int64
			if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_minor),0) FROM rp_household_rent_contributions WHERE agreement_id=? AND membership_id=? AND period_index=?`, r.AgreementID, memberID, r.PeriodIndex).Scan(&already); err != nil {
				return fact, nil, err
			}
			if already < 0 || already > share || r.AmountMinor > share-already {
				return fact, nil, core.NewError(core.CodeInvalidArgument, "rent contribution exceeds own agreed share")
			}
			var memberBalance, rentBalance, memberVersion, rentVersion int64
			err = conn.QueryRowContext(ctx, `SELECT b.balance_minor,b.projection_version FROM accounts a JOIN account_balances b ON b.account_id=a.account_id WHERE a.account_id=? AND a.owner_id=? AND a.currency_id=? AND a.closed_by_event_id IS NULL`, memberAccount, session.ControlledEntityID, currency).Scan(&memberBalance, &memberVersion)
			if err != nil {
				return fact, nil, classifyMissing(err, "own active asset account")
			}
			err = conn.QueryRowContext(ctx, `SELECT b.balance_minor,b.projection_version FROM accounts a JOIN account_balances b ON b.account_id=a.account_id WHERE a.account_id=? AND a.owner_id=? AND a.currency_id=? AND a.closed_by_event_id IS NULL AND a.account_type='household_rent_cash'`, rentAccount, householdID, currency).Scan(&rentBalance, &rentVersion)
			if err != nil {
				return fact, nil, classifyMissing(err, "active household rent account")
			}
			if memberAccount == rentAccount || memberBalance < r.AmountMinor || rentBalance > core.MaxJSONSafeInteger-r.AmountMinor {
				return fact, nil, core.NewError(core.CodeInvalidArgument, "rent contribution exceeds available personal cash or household capacity")
			}
			fact = RPHouseholdRentContributionFact{"corerp.household.contribution.v1", contributionID, r.AgreementID, householdID, memberID, session.ControlledEntityID, r.PeriodIndex, r.AmountMinor, memberAccount, rentAccount, currency}
			return fact, func() error {
				journalID := "journal_" + c.EventID
				if err := execAgentOne(ctx, conn, "rent contribution journal", `INSERT INTO journal_entries(entry_id,event_id,status,purpose) VALUES (?,?,'draft','RP household rent contribution')`, journalID, c.EventID); err != nil {
					return err
				}
				for i, posting := range []struct {
					account string
					amount  int64
				}{{memberAccount, -r.AmountMinor}, {rentAccount, r.AmountMinor}} {
					if err := execAgentOne(ctx, conn, "rent contribution posting", `INSERT INTO postings(posting_id,entry_id,account_id,currency_id,amount_minor) VALUES (?,?,?,?,?)`, fmt.Sprintf("posting_%s_%d", c.EventID, i), journalID, posting.account, currency, posting.amount); err != nil {
						return err
					}
				}
				if err := execAgentOne(ctx, conn, "debit own rent contribution", `UPDATE account_balances SET balance_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE account_id=? AND balance_minor=? AND projection_version=?`, memberBalance-r.AmountMinor, c.Sequence, memberAccount, memberBalance, memberVersion); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "credit household rent contribution", `UPDATE account_balances SET balance_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE account_id=? AND balance_minor=? AND projection_version=?`, rentBalance+r.AmountMinor, c.Sequence, rentAccount, rentBalance, rentVersion); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "post rent contribution journal", `UPDATE journal_entries SET status='posted' WHERE entry_id=? AND status='draft'`, journalID); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "project rent contribution", `INSERT INTO rp_household_rent_contributions(contribution_id,agreement_id,membership_id,period_index,amount_minor,source_account_id,target_account_id,event_id,journal_entry_id) VALUES (?,?,?,?,?,?,?,?,?)`, contributionID, r.AgreementID, memberID, r.PeriodIndex, r.AmountMinor, memberAccount, rentAccount, c.EventID, journalID)
			}, nil
		})
}
