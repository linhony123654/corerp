package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type LawDisputeRequest struct {
	Binding            core.CareerBinding `json:"binding"`
	InstitutionID      string             `json:"institution_id"`
	ActorID            string             `json:"actor_id"`
	EnforcementEventID string             `json:"enforcement_event_id"`
	Statement          string             `json:"statement"`
}
type LawDisputeFact struct {
	ActorID            string   `json:"actor_id"`
	EnforcementEventID string   `json:"enforcement_event_id"`
	Statement          string   `json:"statement"`
	ReviewerIDs        []string `json:"reviewer_ids"`
}
type LawReviewRequest struct {
	Binding        core.CareerBinding `json:"binding"`
	InstitutionID  string             `json:"institution_id"`
	ReviewerID     string             `json:"reviewer_id"`
	DisputeEventID string             `json:"dispute_event_id"`
	Decision       string             `json:"decision"`
	Reason         string             `json:"reason"`
}
type LawReviewFact struct {
	ReceiptEventID     string `json:"receipt_event_id,omitempty"`
	ActorID            string `json:"actor_id"`
	ReviewerID         string `json:"reviewer_id"`
	EnforcementEventID string `json:"enforcement_event_id"`
	DisputeEventID     string `json:"dispute_event_id"`
	Decision           string `json:"decision"`
	Reason             string `json:"reason"`
	RefundedMinor      int64  `json:"refunded_minor"`
}

func readInstitutionFact(ctx context.Context, conn *sql.Conn, b core.CareerBinding, institution, eventID, kind string) (InstitutionFact, error) {
	var raw string
	var fact InstitutionFact
	err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.institution_id')=? AND json_extract(payload,'$.kind')=?`, eventID, b.InstanceID, b.BranchID, institution, kind).Scan(&raw)
	if err != nil {
		return fact, classifyMissing(err, "institution procedure source")
	}
	err = json.Unmarshal([]byte(raw), &fact)
	return fact, err
}

func (s *Store) DisputeRPLaw(ctx context.Context, r LawDisputeRequest) (InstitutionRecord, error) {
	if strings.TrimSpace(r.Statement) == "" || len(r.Statement) > 2000 {
		return InstitutionRecord{}, core.NewError(core.CodeInvalidArgument, "dispute requires bounded grounds")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "DisputeRPLaw", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"own-law-dispute-v1"}`}, func(conn *sql.Conn) error {
		if err := authorizeCareerCandidate(ctx, conn, r.Binding, r.ActorID); err != nil {
			return err
		}
		fact, err := readInstitutionFact(ctx, conn, r.Binding, r.InstitutionID, r.EnforcementEventID, "law_enforcement")
		if err != nil {
			return err
		}
		if fact.Enforcement == nil || fact.Enforcement.ActorID != r.ActorID {
			return core.NewError(core.CodeUnauthorized, "only the affected actor may dispute this enforcement")
		}
		return nil
	}, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_dispute' AND json_extract(payload,'$.dispute.enforcement_event_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.EnforcementEventID).Scan(&count); err != nil {
			return InstitutionFact{}, nil, err
		}
		if count != 0 {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "enforcement already disputed")
		}
		reviewer, status, _, err := readInstitutionRole(ctx, conn, r.Binding, r.InstitutionID, institutionReview)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		dispute := &LawDisputeFact{ActorID: r.ActorID, EnforcementEventID: r.EnforcementEventID, Statement: r.Statement, ReviewerIDs: []string{}}
		if status != "active" {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "institution currently has no active reviewer")
		}
		dispute.ReviewerIDs = append(dispute.ReviewerIDs, reviewer.EntityID)
		// Filing is an explicit written submission to these institutional recipients,
		// not omniscient access to every private case or fabricated overheard speech.
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "law_dispute", InstitutionID: r.InstitutionID, Dispute: dispute}, nil, nil
	})
}

func (s *Store) ReviewRPLawDispute(ctx context.Context, r LawReviewRequest) (InstitutionRecord, error) {
	if (r.Decision != "uphold" && r.Decision != "reverse") || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 2000 {
		return InstitutionRecord{}, core.NewError(core.CodeInvalidArgument, "review requires bounded reason and supported decision")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "ReviewRPLawDispute", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"institution-review-v1"}`}, func(conn *sql.Conn) error {
		return authorizeInstitutionRole(ctx, conn, r.Binding, r.InstitutionID, r.ReviewerID, institutionReview)
	}, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		source, err := readInstitutionFact(ctx, conn, r.Binding, r.InstitutionID, r.DisputeEventID, "law_dispute")
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		if source.Dispute == nil {
			return InstitutionFact{}, nil, core.NewError(core.CodeProjectionDiverged, "missing dispute source")
		}
		d := source.Dispute
		receipt, err := lawDisputeReceipt(ctx, conn, r.Binding, r.InstitutionID, r.DisputeEventID, r.ReviewerID, *d)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		if receipt == "" {
			return InstitutionFact{}, nil, core.NewError(core.CodeUnauthorized, "reviewer did not receive this filing")
		}
		enforced, err := readInstitutionFact(ctx, conn, r.Binding, r.InstitutionID, d.EnforcementEventID, "law_enforcement")
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		if enforced.Enforcement == nil {
			return InstitutionFact{}, nil, core.NewError(core.CodeProjectionDiverged, "missing fine source")
		}
		fine := enforced.Enforcement
		if fine.ActorID != d.ActorID || r.ReviewerID == d.ActorID || r.ReviewerID == fine.EnforcerID {
			return InstitutionFact{}, nil, core.NewError(core.CodeUnauthorized, "review requires a disinterested institutional reviewer")
		}
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_review' AND json_extract(payload,'$.review.enforcement_event_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, d.EnforcementEventID).Scan(&count); err != nil {
			return InstitutionFact{}, nil, err
		}
		if count != 0 {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "enforcement already reviewed")
		}
		review := &LawReviewFact{ActorID: d.ActorID, ReviewerID: r.ReviewerID, EnforcementEventID: d.EnforcementEventID, DisputeEventID: r.DisputeEventID, Decision: r.Decision, Reason: r.Reason}
		if receipt != r.DisputeEventID {
			review.ReceiptEventID = receipt
		}
		var treasuryBalance, actorBalance int64
		if r.Decision == "reverse" && fine.FineMinor > 0 {
			var paid int64
			if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(p.amount_minor),0) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id WHERE j.event_id=? AND j.status='posted' AND p.account_id=? AND p.currency_id=?`, d.EnforcementEventID, fine.TreasuryAccountID, fine.CurrencyID).Scan(&paid); err != nil {
				return InstitutionFact{}, nil, err
			}
			if paid != fine.FineMinor {
				return InstitutionFact{}, nil, core.NewError(core.CodeProjectionDiverged, "refund lacks actual collected fine")
			}
			for _, row := range []struct {
				account string
				balance *int64
			}{{fine.TreasuryAccountID, &treasuryBalance}, {fine.ActorAccountID, &actorBalance}} {
				if err := conn.QueryRowContext(ctx, `SELECT b.balance_minor FROM account_balances b JOIN accounts a ON a.account_id=b.account_id WHERE a.account_id=? AND a.currency_id=? AND a.closed_by_event_id IS NULL`, row.account, fine.CurrencyID).Scan(row.balance); err != nil {
					return InstitutionFact{}, nil, err
				}
			}
			if treasuryBalance < paid || actorBalance > core.MaxJSONSafeInteger-paid {
				return InstitutionFact{}, nil, core.NewError(core.CodeInvalidArgument, "refund exceeds available treasury or recipient capacity")
			}
			review.RefundedMinor = paid
		}
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "law_review", InstitutionID: r.InstitutionID, Review: review}, func() error {
			amount := review.RefundedMinor
			if amount == 0 {
				return nil
			}
			entry := "journal_" + c.EventID
			if err := execAgentOne(ctx, conn, "refund journal", `INSERT INTO journal_entries(entry_id,event_id,status,purpose) VALUES (?,?,'draft','Institution fine reversal')`, entry, c.EventID); err != nil {
				return err
			}
			for i, p := range []struct {
				account         string
				amount, balance int64
			}{{fine.TreasuryAccountID, -amount, treasuryBalance - amount}, {fine.ActorAccountID, amount, actorBalance + amount}} {
				if err := execAgentOne(ctx, conn, "refund posting", `INSERT INTO postings(posting_id,entry_id,account_id,currency_id,amount_minor) VALUES (?,?,?,?,?)`, fmt.Sprintf("posting_%s_%d", c.EventID, i), entry, p.account, fine.CurrencyID, p.amount); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "refund balance", `UPDATE account_balances SET balance_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE account_id=?`, p.balance, c.Sequence, p.account); err != nil {
					return err
				}
			}
			return execAgentOne(ctx, conn, "post refund journal", `UPDATE journal_entries SET status='posted' WHERE entry_id=?`, entry)
		}, nil
	})
}
