package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

type RPOrganizationNoticePublishRequest struct {
	Binding               core.CareerBinding `json:"binding"`
	MessageID             string             `json:"message_id"`
	CareerEventID         string             `json:"career_event_id"`
	SpeakerID             string             `json:"speaker_id"`
	ControllerSessionID   string             `json:"controller_session_id,omitempty"`
	ControllerPrincipalID string             `json:"controller_principal_id,omitempty"`
}

func (r RPOrganizationNoticePublishRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !studioID(r.MessageID) || !studioID(r.CareerEventID) || !studioID(r.SpeakerID) {
		return core.NewError(core.CodeInvalidArgument, "bounded organization notice required")
	}
	if (r.ControllerSessionID == "") != (r.ControllerPrincipalID == "") ||
		(r.ControllerSessionID != "" && (!studioID(r.ControllerSessionID) || !studioID(r.ControllerPrincipalID))) {
		return core.NewError(core.CodeInvalidArgument, "bounded notice controller required")
	}
	return nil
}

func rpOrganizationLayoffText(day int) string {
	return fmt.Sprintf("管理层公告：本组织将于第 %d 天裁减一个岗位。", day)
}

// This reads the immutable private Career event; it never returns its notice,
// employee, contract or compensation in a publishable DTO.
func rpOrganizationLayoffSource(ctx context.Context, q replayQuerier, instanceID, branchID, eventID string) (CareerFact, string, int64, string, error) {
	var raw, actor, worldTime string
	var sequence int64
	err := q.QueryRowContext(ctx, `SELECT actor_id,event_sequence,world_time,payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded'`,
		eventID, instanceID, branchID).Scan(&actor, &sequence, &worldTime, &raw)
	if err == sql.ErrNoRows {
		return CareerFact{}, "", 0, "", core.NewError(core.CodeNotFound, "scoped layoff source not found")
	}
	if err != nil {
		return CareerFact{}, "", 0, "", err
	}
	var fact CareerFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Version != "corerp.career.v1" ||
		fact.Kind != "employment" || fact.Exit == nil || fact.Exit.Kind != "layoff" ||
		fact.Employment == nil || fact.EmploymentChange == nil || fact.EmploymentChange.Kind != "layoff" ||
		fact.OrganizationID == "" || fact.OrganizationID != fact.Employment.OrganizationID ||
		fact.Exit.EffectiveFromDay != fact.Employment.EffectiveFromDay ||
		fact.Exit.RequestedByPrincipalID != actor || fact.EmploymentChange.ManagerPrincipalID != actor {
		return CareerFact{}, "", 0, "", core.NewError(core.CodeProjectionDiverged, "invalid Career layoff source")
	}
	return fact, actor, sequence, worldTime, nil
}

// Publishing creates an intended organization audience, not observations.
// Only an actual later access can make an employee a recipient.
func (s *Store) PublishRPOrganizationNotice(ctx context.Context, r RPOrganizationNoticePublishRequest) (RPInformationRecord, error) {
	if err := r.Validate(); err != nil {
		return RPInformationRecord{}, err
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "PublishRPOrganizationNotice", r,
		privateFactDomain{"rp_information_org", "RPInformationSent", `{"authorization":"career-manager-own-layoff-v1"}`},
		func(conn *sql.Conn) error {
			fact, actor, _, _, err := rpOrganizationLayoffSource(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.CareerEventID)
			if err != nil {
				return err
			}
			if actor != r.Binding.PrincipalID {
				return core.NewError(core.CodeNotFound, "scoped layoff source not found")
			}
			if err := authorizeCareerManager(ctx, conn, r.Binding, fact.OrganizationID); err != nil {
				return core.NewError(core.CodeNotFound, "scoped layoff source not found")
			}
			if r.ControllerSessionID == "" {
				if err := authorizeCareerCandidate(ctx, conn, r.Binding, r.SpeakerID); err != nil {
					return err
				}
			} else {
				if err := requireRPSelectedNoticePublishWindow(ctx, conn, "information_organization_publish", r.Binding,
					r.ControllerPrincipalID, r.ControllerSessionID, r.SpeakerID, r); err != nil {
					return err
				}
				if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.SpeakerID); err != nil {
					return err
				}
			}
			var speakerPrincipal string
			if err := conn.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=? AND instance_id=? AND branch_id=? AND status='active'`,
				r.SpeakerID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&speakerPrincipal); err != nil || speakerPrincipal != actor {
				return core.NewError(core.CodeUnauthorized, "layoff manager must publish as their own Person")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPInformationFact, func() error, error) {
			if err := requireRPSelectedNoticePublishWindow(ctx, conn, "information_organization_publish", r.Binding,
				r.ControllerPrincipalID, r.ControllerSessionID, r.SpeakerID, r); err != nil {
				return RPInformationFact{}, nil, err
			}
			career, _, sourceSequence, sourceWorldTime, err := rpOrganizationLayoffSource(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.CareerEventID)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			if sourceSequence >= c.Sequence {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "notice source must precede publication")
			}
			sourceAt, sourceErr := time.Parse(time.RFC3339Nano, sourceWorldTime)
			publishAt, publishErr := time.Parse(time.RFC3339Nano, c.WorldTime)
			if sourceErr != nil || publishErr != nil || publishAt.Before(sourceAt) {
				return RPInformationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "notice predates Career source")
			}
			var duplicate int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.MessageID).Scan(&duplicate); err != nil {
				return RPInformationFact{}, nil, err
			}
			if duplicate != 0 {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "message ID already used")
			}
			fact := RPInformationFact{Version: "corerp.information.v1", MessageID: r.MessageID,
				SenderID: r.SpeakerID, Text: rpOrganizationLayoffText(career.Exit.EffectiveFromDay),
				Channel: "organization_announcement", Visibility: "organization", ClaimedReliability: "official_statement",
				OrganizationID: career.OrganizationID, SourceCareerEventID: r.CareerEventID}
			return fact, nil, nil
		})
}
