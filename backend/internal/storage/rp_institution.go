package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"strings"
)

const institutionLegislate = "world.institution.legislate"
const institutionEnforce = "world.institution.enforce"
const institutionReview = "world.institution.review"

type InstitutionDefinitionRequest struct {
	Binding                core.CareerBinding `json:"binding"`
	InstitutionID          string             `json:"institution_id"`
	ScopeKind              string             `json:"scope_kind"`
	ScopeID                string             `json:"scope_id"`
	TreasuryOrganizationID string             `json:"treasury_organization_id"`
	LegislatorID           string             `json:"legislator_id"`
	EnforcerID             string             `json:"enforcer_id"`
	ReviewerID             string             `json:"reviewer_id"`
}
type InstitutionDefinitionFact struct {
	InstitutionID          string               `json:"institution_id"`
	Territory              CultureTerritoryFact `json:"territory"`
	TerritoryEventID       string               `json:"territory_event_id"`
	TreasuryOrganizationID string               `json:"treasury_organization_id"`
	TreasuryEventID        string               `json:"treasury_event_id"`
	TreasuryAccountID      string               `json:"treasury_account_id"`
	CurrencyID             string               `json:"currency_id"`
	BuilderSourceEventID   string               `json:"builder_source_event_id"`
	Roles                  []InstitutionRole    `json:"roles"`
}
type InstitutionRole struct {
	GrantID      string `json:"grant_id"`
	CapabilityID string `json:"capability_id"`
	EntityID     string `json:"entity_id"`
	PrincipalID  string `json:"principal_id"`
}
type LawDefinition struct {
	LawID            string `json:"law_id"`
	ProhibitedAction string `json:"prohibited_action"`
	FineMinor        int64  `json:"fine_minor"`
	Text             string `json:"text"`
}
type LawProposalRequest struct {
	Repealed                 bool               `json:"repealed,omitempty"`
	Binding                  core.CareerBinding `json:"binding"`
	InstitutionID            string             `json:"institution_id"`
	ProposerID               string             `json:"proposer_id"`
	PreviousEnactmentEventID string             `json:"previous_enactment_event_id,omitempty"`
	Law                      LawDefinition      `json:"law"`
}
type LawProposalFact struct {
	Repealed                 bool          `json:"repealed,omitempty"`
	ProposerID               string        `json:"proposer_id"`
	PreviousEnactmentEventID string        `json:"previous_enactment_event_id,omitempty"`
	InstitutionEventID       string        `json:"institution_event_id"`
	Law                      LawDefinition `json:"law"`
}
type InstitutionFact struct {
	DisputeForward *LawDisputeForwardFact     `json:"dispute_forward,omitempty"`
	Authority      *InstitutionAuthorityFact  `json:"authority,omitempty"`
	Dispute        *LawDisputeFact            `json:"dispute,omitempty"`
	Review         *LawReviewFact             `json:"review,omitempty"`
	Violation      *LawViolationFact          `json:"violation,omitempty"`
	Enforcement    *LawEnforcementFact        `json:"enforcement,omitempty"`
	Announcement   *LawAnnouncementFact       `json:"announcement,omitempty"`
	Enactment      *LawEnactmentFact          `json:"enactment,omitempty"`
	Version        string                     `json:"version"`
	Kind           string                     `json:"kind"`
	InstitutionID  string                     `json:"institution_id"`
	Definition     *InstitutionDefinitionFact `json:"definition,omitempty"`
	Proposal       *LawProposalFact           `json:"proposal,omitempty"`
}
type InstitutionRecord = privateFactRecord[InstitutionFact]

func validInstitutionID(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 256 && !strings.ContainsAny(value, "\x00\r\n")
}

func (s *Store) DefineRPInstitution(ctx context.Context, r InstitutionDefinitionRequest) (InstitutionRecord, error) {
	if !validInstitutionID(r.InstitutionID) || (r.ScopeKind != "world" && r.ScopeKind != "region") {
		return InstitutionRecord{}, core.NewError(core.CodeInvalidArgument, "invalid institution identity or scope")
	}
	var builder string
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPInstitution", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"institution-builder-v1"}`}, func(conn *sql.Conn) error {
		var err error
		builder, err = authorizeCultureBuilder(ctx, conn, r.Binding)
		return err
	}, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='institution' AND json_extract(payload,'$.institution_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.InstitutionID).Scan(&count); err != nil {
			return InstitutionFact{}, nil, err
		}
		if count != 0 {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "institution already defined")
		}
		territory, source, err := readCultureTerritory(ctx, conn, r.Binding, r.ScopeKind, r.ScopeID)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		org, err := readCareerRecord(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, "organization", r.TreasuryOrganizationID)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		if org.Fact.Organization == nil {
			return InstitutionFact{}, nil, core.NewError(core.CodeProjectionDiverged, "institution requires actual funded treasury organization")
		}
		definition := &InstitutionDefinitionFact{InstitutionID: r.InstitutionID, Territory: territory, TerritoryEventID: source, TreasuryOrganizationID: r.TreasuryOrganizationID, TreasuryEventID: org.EventID, TreasuryAccountID: org.Fact.Organization.CashAccountID, CurrencyID: org.Fact.Organization.CurrencyID, BuilderSourceEventID: builder}
		for _, role := range []InstitutionRole{{CapabilityID: institutionLegislate, EntityID: r.LegislatorID}, {CapabilityID: institutionEnforce, EntityID: r.EnforcerID}, {CapabilityID: institutionReview, EntityID: r.ReviewerID}} {
			if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, role.EntityID); err != nil {
				return InstitutionFact{}, nil, err
			}
			if err := conn.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=?`, role.EntityID).Scan(&role.PrincipalID); err != nil {
				return InstitutionFact{}, nil, err
			}
			role.GrantID = "grant_" + c.EventID + "_" + role.CapabilityID
			definition.Roles = append(definition.Roles, role)
		}
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "institution", InstitutionID: r.InstitutionID, Definition: definition}, func() error {
			for _, role := range definition.Roles {
				if _, err := conn.ExecContext(ctx, `INSERT INTO capability_definitions(capability_id,description,policy_version) VALUES (?,'Scoped institutional role, separate from cultural and economic authority','institution-v1') ON CONFLICT(capability_id) DO NOTHING`, role.CapabilityID); err != nil {
					return err
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,?,?,?,?,'[]','active',?)`, role.GrantID, role.PrincipalID, role.CapabilityID, r.Binding.InstanceID, r.Binding.BranchID, r.InstitutionID, c.EventID); err != nil {
					return err
				}
			}
			return nil
		}, nil
	})
}

func readInstitutionDefinition(ctx context.Context, conn *sql.Conn, b core.CareerBinding, id string) (InstitutionDefinitionFact, string, error) {
	var raw, eventID string
	var fact InstitutionFact
	err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='institution' AND json_extract(payload,'$.institution_id')=?`, b.InstanceID, b.BranchID, id).Scan(&eventID, &raw)
	if err != nil {
		return InstitutionDefinitionFact{}, "", classifyMissing(err, "institution")
	}
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return InstitutionDefinitionFact{}, "", err
	}
	if fact.Definition == nil {
		return InstitutionDefinitionFact{}, "", core.NewError(core.CodeProjectionDiverged, "institution source missing")
	}
	return *fact.Definition, eventID, nil
}

func (s *Store) ProposeRPLaw(ctx context.Context, r LawProposalRequest) (InstitutionRecord, error) {
	if !validInstitutionID(r.InstitutionID) || !validInstitutionID(r.Law.LawID) || r.Law.ProhibitedAction != "speak" || r.Law.FineMinor < 0 || r.Law.FineMinor > core.MaxJSONSafeInteger || strings.TrimSpace(r.Law.Text) == "" || len(r.Law.Text) > 1000 {
		return InstitutionRecord{}, core.NewError(core.CodeInvalidArgument, "invalid bounded law proposal")
	}
	// Proposals are not laws. Any controlled actor may submit one; legislation
	// requires a separate dedicated role in the subsequent enactment command.
	return executePrivateFactCommand(s, ctx, r.Binding, "ProposeRPLaw", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"own-law-proposal-v1"}`}, func(conn *sql.Conn) error { return authorizeCareerCandidate(ctx, conn, r.Binding, r.ProposerID) }, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		_, source, err := readInstitutionDefinition(ctx, conn, r.Binding, r.InstitutionID)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		if r.PreviousEnactmentEventID != "" {
			known, err := readKnownRPLaws(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.ProposerID)
			if err != nil {
				return InstitutionFact{}, nil, err
			}
			found := false
			for _, law := range known {
				if law.EnactmentEventID == r.PreviousEnactmentEventID && law.InstitutionID == r.InstitutionID && law.LawID == r.Law.LawID {
					found = true
				}
			}
			if !found {
				return InstitutionFact{}, nil, core.NewError(core.CodeUnauthorized, "proposal requires known predecessor for this law")
			}
		} else if r.Repealed {
			return InstitutionFact{}, nil, core.NewError(core.CodeInvalidArgument, "repeal requires a predecessor")
		}
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "law_proposal", InstitutionID: r.InstitutionID, Proposal: &LawProposalFact{ProposerID: r.ProposerID, InstitutionEventID: source, Law: r.Law, PreviousEnactmentEventID: r.PreviousEnactmentEventID, Repealed: r.Repealed}}, nil, nil
	})
}
