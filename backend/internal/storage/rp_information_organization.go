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

func rpOrganizationNoticeSource(ctx context.Context, q replayQuerier, instanceID, branchID, eventID string) (CareerFact, string, int64, string, string, error) {
	var raw, actor, worldTime string
	var sequence int64
	err := q.QueryRowContext(ctx, `SELECT actor_id,event_sequence,world_time,payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded'`,
		eventID, instanceID, branchID).Scan(&actor, &sequence, &worldTime, &raw)
	if err == sql.ErrNoRows {
		return CareerFact{}, "", 0, "", "", core.NewError(core.CodeNotFound, "scoped notice source not found")
	}
	if err != nil {
		return CareerFact{}, "", 0, "", "", err
	}
	var fact CareerFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return CareerFact{}, "", 0, "", "", core.NewError(core.CodeProjectionDiverged, "invalid Career source payload")
	}
	if fact.Version != "corerp.career.v1" {
		return CareerFact{}, "", 0, "", "", core.NewError(core.CodeProjectionDiverged, "invalid Career source version")
	}
	if fact.Kind == "organization_review" && fact.OrganizationReview != nil {
		review := fact.OrganizationReview
		if fact.OrganizationID == "" || review.ReviewID == "" || fact.RecordID != review.ReviewID ||
			review.OrganizationID != fact.OrganizationID || review.PolicyID == "" ||
			review.WorldTime != worldTime || review.EventID != eventID || review.EventSequence != sequence ||
			review.Decision.DecisionKind == "no_change" || review.Decision.TargetPositionID == "" ||
			fact.Posting == nil || fact.Posting.PositionID != review.Decision.TargetPositionID ||
			fact.Posting.OrganizationID != fact.OrganizationID || fact.Posting.Status != review.Decision.PostingStatus {
			return CareerFact{}, "", 0, "", "", core.NewError(core.CodeProjectionDiverged, "invalid organization review source")
		}
		policy, _, err := organizationScheduleSource(ctx, q, instanceID, branchID, eventID)
		if err != nil || review.Decision != core.EvaluateOrganizationPolicy(policy, review.Evidence) {
			return CareerFact{}, "", 0, "", "", core.NewError(core.CodeProjectionDiverged, "organization decision lacks policy source")
		}
		if actor == "system" {
			scheduledPolicy, at, err := organizationScheduleSource(ctx, q, instanceID, branchID, review.ScheduleSourceEventID)
			if err != nil {
				return CareerFact{}, "", 0, "", "", err
			}
			item, _, err := organizationReviewItem(review.ScheduleSourceEventID, at, scheduledPolicy)
			if err != nil || !policy.AutomaticReview || scheduledPolicy != policy || review.DecisionPrincipalID != policy.ManagerPrincipalID || eventID != "event_"+item.SchedulerItemID || worldTime != item.WorldTime {
				return CareerFact{}, "", 0, "", "", core.NewError(core.CodeProjectionDiverged, "scheduled notice lacks manager authority source")
			}
			actor = review.DecisionPrincipalID
		} else if review.DecisionPrincipalID != actor || review.ScheduleSourceEventID != "" {
			return CareerFact{}, "", 0, "", "", core.NewError(core.CodeProjectionDiverged, "manual review principal differs")
		}
		text := fmt.Sprintf("管理层公告：组织因运营需要实施决定：%s。", review.Decision.Reason)
		return fact, actor, sequence, worldTime, text, nil
	}
	if fact.Kind == "employment" && fact.Exit != nil && fact.Exit.Kind == "layoff" {
		if fact.Employment == nil || fact.EmploymentChange == nil || fact.EmploymentChange.Kind != "layoff" ||
			fact.OrganizationID == "" || fact.OrganizationID != fact.Employment.OrganizationID ||
			fact.Exit.EffectiveFromDay != fact.Employment.EffectiveFromDay ||
			fact.Exit.RequestedByPrincipalID != actor || fact.EmploymentChange.ManagerPrincipalID != actor {
			return CareerFact{}, "", 0, "", "", core.NewError(core.CodeProjectionDiverged, "invalid Career layoff source")
		}
		return fact, actor, sequence, worldTime, rpOrganizationLayoffText(fact.Exit.EffectiveFromDay), nil
	}
	return CareerFact{}, "", 0, "", "", core.NewError(core.CodeProjectionDiverged, "unsupported organization notice source")
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
			fact, actor, _, _, _, err := rpOrganizationNoticeSource(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.CareerEventID)
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
			career, _, sourceSequence, sourceWorldTime, noticeText, err := rpOrganizationNoticeSource(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.CareerEventID)
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
				SenderID: r.SpeakerID, Text: noticeText,
				Channel: "organization_announcement", Visibility: "organization", ClaimedReliability: "official_statement",
				OrganizationID: career.OrganizationID, SourceCareerEventID: r.CareerEventID}
			return fact, nil, nil
		})
}
