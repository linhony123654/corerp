package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"fmt"
)

type LawViolationRequest struct {
	Binding          core.CareerBinding `json:"binding"`
	InstitutionID    string             `json:"institution_id"`
	EnforcerID       string             `json:"enforcer_id"`
	EnactmentEventID string             `json:"enactment_event_id"`
	ActionEventID    string             `json:"action_event_id"`
}
type LawViolationFact struct {
	EnforcerID               string          `json:"enforcer_id"`
	ActorID                  string          `json:"actor_id"`
	ActionEventID            string          `json:"action_event_id"`
	ActionWorldTime          string          `json:"action_world_time"`
	PlaceID                  string          `json:"place_id"`
	Law                      core.RPKnownLaw `json:"law"`
	EnforcerKnowledgeEventID string          `json:"enforcer_knowledge_event_id"`
}
type LawEnforcementRequest struct {
	Binding          core.CareerBinding `json:"binding"`
	InstitutionID    string             `json:"institution_id"`
	EnforcerID       string             `json:"enforcer_id"`
	ViolationEventID string             `json:"violation_event_id"`
}
type LawEnforcementFact struct {
	EnforcerID        string `json:"enforcer_id"`
	ActorID           string `json:"actor_id"`
	ViolationEventID  string `json:"violation_event_id"`
	EnactmentEventID  string `json:"enactment_event_id"`
	FineMinor         int64  `json:"fine_minor"`
	ActorAccountID    string `json:"actor_account_id"`
	TreasuryAccountID string `json:"treasury_account_id"`
	CurrencyID        string `json:"currency_id"`
}

func (s *Store) RecordRPLawViolation(ctx context.Context, r LawViolationRequest) (InstitutionRecord, error) {
	return executePrivateFactCommand(s, ctx, r.Binding, "RecordRPLawViolation", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"institution-enforce-v1"}`}, func(conn *sql.Conn) error {
		return authorizeInstitutionRole(ctx, conn, r.Binding, r.InstitutionID, r.EnforcerID, institutionEnforce)
	}, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		var raw, at, eventType string
		err := conn.QueryRowContext(ctx, `SELECT payload,world_time,event_type FROM events WHERE event_id=? AND instance_id=? AND branch_id=?`, r.ActionEventID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&raw, &at, &eventType)
		if err != nil {
			return InstitutionFact{}, nil, classifyMissing(err, "actual accepted speech")
		}
		speech, err := lawSpeechEvidence(eventType, raw)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		witnessed := speech.SpeakerEntityID == r.EnforcerID
		for _, id := range speech.ListenerIDs {
			if id == r.EnforcerID {
				witnessed = true
			}
		}
		if !witnessed {
			return InstitutionFact{}, nil, core.NewError(core.CodeUnauthorized, "enforcer has no actual observation of this act")
		}
		laws, err := readEffectiveInstitutionLaws(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, at, speech.PlaceID)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		var applicable *core.RPKnownLaw
		for i := range laws {
			if laws[i].InstitutionID == r.InstitutionID && laws[i].EnactmentEventID == r.EnactmentEventID && laws[i].ProhibitedAction == "speak" {
				copy := laws[i]
				applicable = &copy
			}
		}
		if applicable == nil {
			return InstitutionFact{}, nil, core.NewError(core.CodeInvalidArgument, "act was not prohibited by this rule at its actual time/place")
		}
		known, err := readKnownRPLaws(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.EnforcerID)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		source := ""
		for _, law := range known {
			if law.EnactmentEventID == r.EnactmentEventID {
				source = law.KnowledgeEventID
			}
		}
		if source == "" {
			return InstitutionFact{}, nil, core.NewError(core.CodeUnauthorized, "enforcer has not learned cited law")
		}
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_violation' AND json_extract(payload,'$.violation.action_event_id')=? AND json_extract(payload,'$.violation.law.enactment_event_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.ActionEventID, r.EnactmentEventID).Scan(&count); err != nil {
			return InstitutionFact{}, nil, err
		}
		if count != 0 {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "violation already recorded for this act/rule")
		}
		violation := &LawViolationFact{EnforcerID: r.EnforcerID, ActorID: speech.SpeakerEntityID, ActionEventID: r.ActionEventID, ActionWorldTime: at, PlaceID: speech.PlaceID, Law: *applicable, EnforcerKnowledgeEventID: source}
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "law_violation", InstitutionID: r.InstitutionID, Violation: violation}, nil, nil
	})
}

func (s *Store) EnforceRPLaw(ctx context.Context, r LawEnforcementRequest) (InstitutionRecord, error) {
	return executePrivateFactCommand(s, ctx, r.Binding, "EnforceRPLaw", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"institution-enforce-v1"}`}, func(conn *sql.Conn) error {
		return authorizeInstitutionRole(ctx, conn, r.Binding, r.InstitutionID, r.EnforcerID, institutionEnforce)
	}, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		var raw string
		var source InstitutionFact
		if err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_violation' AND json_extract(payload,'$.institution_id')=?`, r.ViolationEventID, r.Binding.InstanceID, r.Binding.BranchID, r.InstitutionID).Scan(&raw); err != nil {
			return InstitutionFact{}, nil, classifyMissing(err, "recorded violation")
		}
		if err := json.Unmarshal([]byte(raw), &source); err != nil {
			return InstitutionFact{}, nil, err
		}
		if source.Violation == nil {
			return InstitutionFact{}, nil, core.NewError(core.CodeProjectionDiverged, "violation source missing")
		}
		v := source.Violation
		// The procedure may be delegated to another enforcer, but a grant alone is
		// not knowledge of the private case: this first slice requires its recorder.
		if v.EnforcerID != r.EnforcerID {
			return InstitutionFact{}, nil, core.NewError(core.CodeUnauthorized, "enforcer has not received this case")
		}
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_enforcement' AND json_extract(payload,'$.enforcement.violation_event_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.ViolationEventID).Scan(&count); err != nil {
			return InstitutionFact{}, nil, err
		}
		if count != 0 {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "violation already enforced")
		}
		institution, _, err := readInstitutionDefinition(ctx, conn, r.Binding, r.InstitutionID)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		effect := &LawEnforcementFact{EnforcerID: r.EnforcerID, ActorID: v.ActorID, ViolationEventID: r.ViolationEventID, EnactmentEventID: v.Law.EnactmentEventID, FineMinor: v.Law.FineMinor, TreasuryAccountID: institution.TreasuryAccountID, CurrencyID: institution.CurrencyID}
		var actorBalance, treasuryBalance int64
		if err := conn.QueryRowContext(ctx, `SELECT n.asset_account_id,b.balance_minor FROM materialized_entities n JOIN cohorts cohort ON cohort.cohort_id=n.source_cohort_id JOIN accounts a ON a.account_id=n.asset_account_id AND a.closed_by_event_id IS NULL JOIN account_balances b ON b.account_id=a.account_id WHERE n.entity_id=? AND n.status='active' AND n.population_count=1 AND n.currency_id=? AND cohort.instance_id=? AND cohort.branch_id=?`, v.ActorID, institution.CurrencyID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&effect.ActorAccountID, &actorBalance); err != nil {
			return InstitutionFact{}, nil, classifyMissing(err, "actual defendant account")
		}
		if err := conn.QueryRowContext(ctx, `SELECT b.balance_minor FROM account_balances b JOIN accounts a ON a.account_id=b.account_id WHERE a.account_id=? AND a.currency_id=? AND a.closed_by_event_id IS NULL`, effect.TreasuryAccountID, effect.CurrencyID).Scan(&treasuryBalance); err != nil {
			return InstitutionFact{}, nil, err
		}
		if effect.ActorAccountID == effect.TreasuryAccountID || effect.FineMinor < 0 || actorBalance < effect.FineMinor || treasuryBalance > core.MaxJSONSafeInteger-effect.FineMinor {
			return InstitutionFact{}, nil, core.NewError(core.CodeInvalidArgument, "fine exceeds actual funds or treasury capacity")
		}
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "law_enforcement", InstitutionID: r.InstitutionID, Enforcement: effect}, func() error {
			if effect.FineMinor == 0 {
				return nil
			}
			entry := "journal_" + c.EventID
			if err := execAgentOne(ctx, conn, "fine journal", `INSERT INTO journal_entries(entry_id,event_id,status,purpose) VALUES (?,?,'draft','Institution law fine')`, entry, c.EventID); err != nil {
				return err
			}
			for i, p := range []struct {
				account         string
				amount, balance int64
			}{{effect.ActorAccountID, -effect.FineMinor, actorBalance - effect.FineMinor}, {effect.TreasuryAccountID, effect.FineMinor, treasuryBalance + effect.FineMinor}} {
				if err := execAgentOne(ctx, conn, "fine posting", `INSERT INTO postings(posting_id,entry_id,account_id,currency_id,amount_minor) VALUES (?,?,?,?,?)`, fmt.Sprintf("posting_%s_%d", c.EventID, i), entry, p.account, effect.CurrencyID, p.amount); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "fine balance", `UPDATE account_balances SET balance_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE account_id=?`, p.balance, c.Sequence, p.account); err != nil {
					return err
				}
			}
			return execAgentOne(ctx, conn, "post fine journal", `UPDATE journal_entries SET status='posted' WHERE entry_id=?`, entry)
		}, nil
	})
}
