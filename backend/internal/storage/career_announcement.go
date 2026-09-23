package storage

import (
	"context"
	"database/sql"
	"fmt"

	"corerp.local/backend/internal/core"
)

// Immutable in-world speech provenance. This is not an RP turn: no player
// session/turn is fabricated for an independently acting organization speaker.
type CareerAnnouncementFact struct {
	TermsEventID string   `json:"terms_event_id"`
	SpeakerID    string   `json:"speaker_id"`
	EmployeeID   string   `json:"employee_id"`
	PositionID   string   `json:"position_id"`
	PlaceID      string   `json:"place_id"`
	UtteranceID  string   `json:"utterance_id"`
	Text         string   `json:"text"`
	ListenerIDs  []string `json:"listener_ids"`
}

func (s *Store) SpeakCareerAnnouncement(ctx context.Context, r core.CareerAnnouncementRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "SpeakCareerAnnouncement", r, func(conn *sql.Conn) error {
		org, err := readCareerContractOrganization(ctx, conn, b, r.ContractID)
		if err != nil {
			return err
		}
		if err := authorizeCareerManager(ctx, conn, b, org); err != nil {
			return err
		}
		return authorizeCareerCandidate(ctx, conn, b, r.SpeakerID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "announcement", r.AnnouncementID); err != nil {
			return CareerFact{}, nil, err
		}
		job, source, _, err := currentCareerEmployment(ctx, conn, b, r.ContractID, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if err := validateRPBinding(ctx, conn, b.InstanceID, b.BranchID, r.SpeakerID); err != nil {
			return CareerFact{}, nil, err
		}
		var place, name string
		if err := conn.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, r.SpeakerID).Scan(&place); err != nil {
			return CareerFact{}, nil, err
		}
		if err := conn.QueryRowContext(ctx, `SELECT display_name FROM materialized_entities WHERE entity_id=?`, job.EmployeeID).Scan(&name); err != nil {
			return CareerFact{}, nil, err
		}
		var actual int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND status='active' AND position_id=? AND gross_wage_minor=?`, job.ContractID, job.PositionKey, job.DailyWageMinor).Scan(&actual); err != nil {
			return CareerFact{}, nil, err
		}
		if actual != 1 {
			return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "announced position is not the actual effective contract")
		}
		org, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "organization", job.OrganizationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		post, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", job.PositionID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if org.Fact.Organization == nil || post.Fact.Posting == nil {
			return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "announcement lacks organization or position")
		}
		authority := "not delegated by this position"
		if len(job.Capabilities) > 0 {
			var granted int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g JOIN agent_profiles a ON a.principal_id=g.principal_id AND a.instance_id=g.instance_id AND a.branch_id=g.branch_id WHERE a.agent_id=? AND g.instance_id=? AND g.branch_id=? AND g.subject_id=? AND g.capability_id=? AND g.status='active'`, job.EmployeeID, b.InstanceID, b.BranchID, job.OrganizationID, core.CareerPositionManageCapability).Scan(&granted); err != nil {
				return CareerFact{}, nil, err
			}
			if granted != 1 {
				return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "announced position authority is not effective")
			}
			authority = "delegated within this organization"
		}
		text := fmt.Sprintf("%s announces: %s currently holds %s (occupation %s, grade %s). Career-management authority is %s.", org.Fact.Organization.Definition.DisplayName, name, post.Fact.Posting.Title, job.OccupationID, job.Grade, authority)
		if len(text) > 2000 {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "announcement exceeds speech text bound")
		}
		listeners, err := rpCoLocatedEntityIDs(ctx, conn, b.InstanceID, b.BranchID, place, r.SpeakerID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		a := &CareerAnnouncementFact{TermsEventID: source, SpeakerID: r.SpeakerID, EmployeeID: job.EmployeeID, PositionID: job.PositionID, PlaceID: place, UtteranceID: "utterance_" + c.EventID, Text: text, ListenerIDs: listeners}
		fact := CareerFact{Kind: "announcement", RecordID: r.AnnouncementID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, Announcement: a}
		return fact, func() error {
			// Exactly the existing in-person speech/hearing evidence path. Each
			// listener learns an attributed statement, not global private terms.
			return insertRPSpeechHearings(ctx, conn, c.EventID, c.Sequence, r.SpeakerID, place, c.WorldTime, a.UtteranceID, text, "statement", listeners)
		}, nil
	})
}
