package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"strings"
)

// A chosen-family declaration is not evidence of biological descent, marriage,
// inheritance, co-residence, employment or authority over the other person.
type CultureFamilyFact struct {
	FamilyID         string `json:"family_id"`
	ProposerID       string `json:"proposer_id"`
	InviteeID        string `json:"invitee_id"`
	ProposalEventID  string `json:"proposal_event_id"`
	PlaceID          string `json:"place_id"`
	Status           string `json:"status"`
	RelationshipKind string `json:"relationship_kind"`
}
type CultureFamilyProposalRequest struct {
	Binding    core.CareerBinding `json:"binding"`
	FamilyID   string             `json:"family_id"`
	ProposerID string             `json:"proposer_id"`
	InviteeID  string             `json:"invitee_id"`
}
type CultureFamilyAcceptRequest struct {
	Binding         core.CareerBinding `json:"binding"`
	EntityID        string             `json:"entity_id"`
	ProposalEventID string             `json:"proposal_event_id"`
}

func (s *Store) ProposeRPCulturalFamily(ctx context.Context, r CultureFamilyProposalRequest) (CultureRecord, error) {
	if strings.TrimSpace(r.FamilyID) == "" || len(r.FamilyID) > 256 || r.ProposerID == r.InviteeID {
		return CultureRecord{}, core.NewError(core.CodeInvalidArgument, "family requires bounded identity and distinct participants")
	}
	return s.executeCultureCommand(ctx, r.Binding, "ProposeRPCulturalFamily", r, r.ProposerID, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.InviteeID); err != nil {
			return CultureFact{}, nil, err
		}
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCultureFactRecorded' AND json_extract(payload,'$.family.family_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.FamilyID).Scan(&count); err != nil {
			return CultureFact{}, nil, err
		}
		if count != 0 {
			return CultureFact{}, nil, core.NewError(core.CodeBranchConflict, "family identity already proposed")
		}
		var place string
		if err := conn.QueryRowContext(ctx, `SELECT p.place_id FROM agent_positions p JOIN agent_positions i ON i.agent_id=? AND i.place_id=p.place_id WHERE p.agent_id=?`, r.InviteeID, r.ProposerID).Scan(&place); err != nil {
			return CultureFact{}, nil, classifyMissing(err, "present family invitee")
		}
		family := &CultureFamilyFact{FamilyID: r.FamilyID, ProposerID: r.ProposerID, InviteeID: r.InviteeID, ProposalEventID: c.EventID, PlaceID: place, Status: "proposed", RelationshipKind: "chosen_family"}
		return CultureFact{Kind: "family_proposal", ActorID: r.ProposerID, Family: family}, nil, nil
	})
}

func (s *Store) AcceptRPCulturalFamily(ctx context.Context, r CultureFamilyAcceptRequest) (CultureRecord, error) {
	return s.executeCultureCommand(ctx, r.Binding, "AcceptRPCulturalFamily", r, r.EntityID, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		proposal, err := readCultureFact(ctx, conn, r.Binding, r.ProposalEventID)
		if err != nil {
			return CultureFact{}, nil, err
		}
		if proposal.Kind != "family_proposal" || proposal.Family == nil || proposal.Family.InviteeID != r.EntityID {
			return CultureFact{}, nil, core.NewError(core.CodeUnauthorized, "family acceptance belongs to actual invited person")
		}
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='family_accepted' AND json_extract(payload,'$.family.proposal_event_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.ProposalEventID).Scan(&count); err != nil {
			return CultureFact{}, nil, err
		}
		if count != 0 {
			return CultureFact{}, nil, core.NewError(core.CodeBranchConflict, "family proposal already accepted")
		}
		family := *proposal.Family
		family.Status = "accepted"
		return CultureFact{Kind: "family_accepted", ActorID: r.EntityID, Family: &family}, nil, nil
	})
}

func requireCulturalFamilyMember(ctx context.Context, conn *sql.Conn, b core.CareerBinding, familyID, entityID string) (string, error) {
	var eventID, raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='family_accepted' AND json_extract(payload,'$.family.family_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, familyID).Scan(&eventID, &raw)
	if err == sql.ErrNoRows {
		return "", core.NewError(core.CodeUnauthorized, "family has no mutually confirmed source")
	}
	if err != nil {
		return "", err
	}
	var fact CultureFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return "", err
	}
	if fact.Family == nil {
		return "", core.NewError(core.CodeProjectionDiverged, "family source lacks declaration")
	}
	if entityID != fact.Family.ProposerID && entityID != fact.Family.InviteeID {
		return "", core.NewError(core.CodeUnauthorized, "actor is not a declared family member")
	}
	return eventID, nil
}
