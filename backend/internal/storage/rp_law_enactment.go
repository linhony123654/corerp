package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"time"
)

type LawEnactmentRequest struct {
	Binding            core.CareerBinding `json:"binding"`
	InstitutionID      string             `json:"institution_id"`
	LegislatorID       string             `json:"legislator_id"`
	ProposalEventID    string             `json:"proposal_event_id"`
	EffectiveWorldTime string             `json:"effective_world_time"`
}
type LawEnactmentFact struct {
	PreviousEnactmentEventID string        `json:"previous_enactment_event_id,omitempty"`
	Repealed                 bool          `json:"repealed,omitempty"`
	LegislatorID             string        `json:"legislator_id"`
	ProposalEventID          string        `json:"proposal_event_id"`
	InstitutionEventID       string        `json:"institution_event_id"`
	EffectiveWorldTime       string        `json:"effective_world_time"`
	Law                      LawDefinition `json:"law"`
}

func authorizeInstitutionRole(ctx context.Context, conn *sql.Conn, b core.CareerBinding, institution, actor, capability string) error {
	if err := authorizeCareerCandidate(ctx, conn, b, actor); err != nil {
		return err
	}
	role, status, source, err := readInstitutionRole(ctx, conn, b, institution, capability)
	if err != nil {
		return err
	}
	if status != "active" || role.EntityID != actor || role.PrincipalID != b.PrincipalID {
		return core.NewError(core.CodeUnauthorized, "institution role is revoked or assigned to another actor")
	}
	var count int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants WHERE grant_id=? AND principal_id=? AND instance_id=? AND branch_id=? AND subject_id=? AND capability_id=? AND status='active' AND field_scope='[]' AND amount_limit_minor IS NULL AND definition_event_id=?`, role.GrantID, b.PrincipalID, b.InstanceID, b.BranchID, institution, capability, source).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return core.NewError(core.CodeUnauthorized, "institution requires dedicated sourced role")
	}
	return nil
}

func (s *Store) EnactRPLaw(ctx context.Context, r LawEnactmentRequest) (InstitutionRecord, error) {
	at, err := time.Parse(time.RFC3339, r.EffectiveWorldTime)
	if err != nil {
		return InstitutionRecord{}, core.NewError(core.CodeInvalidArgument, "law requires effective world time")
	}
	r.EffectiveWorldTime = at.UTC().Format(time.RFC3339)
	return executePrivateFactCommand(s, ctx, r.Binding, "EnactRPLaw", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"institution-legislate-v1"}`}, func(conn *sql.Conn) error {
		return authorizeInstitutionRole(ctx, conn, r.Binding, r.InstitutionID, r.LegislatorID, institutionLegislate)
	}, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		now, err := time.Parse(time.RFC3339, c.WorldTime)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		effective, _ := time.Parse(time.RFC3339, r.EffectiveWorldTime)
		if !effective.After(now) {
			return InstitutionFact{}, nil, core.NewError(core.CodeInvalidArgument, "enactment must take effect strictly in the future")
		}
		var raw string
		var proposal InstitutionFact
		if err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_proposal' AND json_extract(payload,'$.institution_id')=?`, r.ProposalEventID, r.Binding.InstanceID, r.Binding.BranchID, r.InstitutionID).Scan(&raw); err != nil {
			return InstitutionFact{}, nil, classifyMissing(err, "law proposal")
		}
		if err := json.Unmarshal([]byte(raw), &proposal); err != nil {
			return InstitutionFact{}, nil, err
		}
		if proposal.Proposal == nil {
			return InstitutionFact{}, nil, core.NewError(core.CodeProjectionDiverged, "missing law proposal source")
		}
		var latestID, latestTime string
		err = conn.QueryRowContext(ctx, `SELECT event_id,json_extract(payload,'$.enactment.effective_world_time') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_enactment' AND json_extract(payload,'$.institution_id')=? AND json_extract(payload,'$.enactment.law.law_id')=? ORDER BY event_sequence DESC LIMIT 1`, r.Binding.InstanceID, r.Binding.BranchID, r.InstitutionID, proposal.Proposal.Law.LawID).Scan(&latestID, &latestTime)
		if err != nil && err != sql.ErrNoRows {
			return InstitutionFact{}, nil, err
		}
		if latestID != proposal.Proposal.PreviousEnactmentEventID {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "law predecessor is not the latest enacted version")
		}
		if latestID != "" {
			previousTime, err := time.Parse(time.RFC3339, latestTime)
			if err != nil {
				return InstitutionFact{}, nil, err
			}
			if !effective.After(previousTime) {
				return InstitutionFact{}, nil, core.NewError(core.CodeInvalidArgument, "law version must take effect after predecessor")
			}
		}
		enacted := &LawEnactmentFact{LegislatorID: r.LegislatorID, ProposalEventID: r.ProposalEventID, InstitutionEventID: proposal.Proposal.InstitutionEventID, EffectiveWorldTime: r.EffectiveWorldTime, Law: proposal.Proposal.Law, PreviousEnactmentEventID: proposal.Proposal.PreviousEnactmentEventID, Repealed: proposal.Proposal.Repealed}
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "law_enactment", InstitutionID: r.InstitutionID, Enactment: enacted}, nil, nil
	})
}
