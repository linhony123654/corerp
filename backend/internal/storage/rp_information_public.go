package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPPublicNoticePublishRequest struct {
	Binding    core.CareerBinding `json:"binding"`
	MessageID  string             `json:"message_id"`
	LawEventID string             `json:"law_event_id"`
	SpeakerID  string             `json:"speaker_id"`
	// A selected F3 child retains the legislator's source principal as Event
	// actor while proving who currently controls the speaking Person.
	ControllerSessionID   string `json:"controller_session_id,omitempty"`
	ControllerPrincipalID string `json:"controller_principal_id,omitempty"`
}

func (r RPPublicNoticePublishRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !studioID(r.MessageID) || !studioID(r.LawEventID) || !studioID(r.SpeakerID) {
		return core.NewError(core.CodeInvalidArgument, "bounded public notice source required")
	}
	if (r.ControllerSessionID == "") != (r.ControllerPrincipalID == "") ||
		(r.ControllerSessionID != "" && (!studioID(r.ControllerSessionID) || !studioID(r.ControllerPrincipalID))) {
		return core.NewError(core.CodeInvalidArgument, "bounded notice controller required")
	}
	return nil
}

func rpPublicLawText(fact InstitutionFact) string {
	law := fact.Enactment
	if law.Repealed {
		return fmt.Sprintf("法令公告：%s 将于 %s 废止。", law.Law.LawID, law.EffectiveWorldTime)
	}
	return fmt.Sprintf("法令公告：%s 将于 %s 生效。%s", law.Law.LawID, law.EffectiveWorldTime, law.Law.Text)
}

// Only an enacted law, never a proposal or arbitrary caller text, can source
// this public publication. The enactment and proposal are both immutable.
func rpPublicLawSource(ctx context.Context, q replayQuerier, instanceID, branchID, eventID string) (InstitutionFact, string, int64, string, error) {
	var raw, actor, worldTime string
	var sequence int64
	err := q.QueryRowContext(ctx, `SELECT actor_id,event_sequence,world_time,payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded'`,
		eventID, instanceID, branchID).Scan(&actor, &sequence, &worldTime, &raw)
	if err == sql.ErrNoRows {
		return InstitutionFact{}, "", 0, "", core.NewError(core.CodeNotFound, "scoped law enactment not found")
	}
	if err != nil {
		return InstitutionFact{}, "", 0, "", err
	}
	var fact InstitutionFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Version != "corerp.institution.v1" ||
		fact.Kind != "law_enactment" || fact.Enactment == nil || fact.InstitutionID == "" ||
		fact.Enactment.LegislatorID == "" || fact.Enactment.ProposalEventID == "" ||
		fact.Enactment.InstitutionEventID == "" || !validInstitutionID(fact.Enactment.Law.LawID) ||
		strings.TrimSpace(fact.Enactment.Law.Text) == "" || len(fact.Enactment.Law.Text) > 1000 {
		return InstitutionFact{}, "", 0, "", core.NewError(core.CodeProjectionDiverged, "invalid law enactment source")
	}
	var proposalRaw, proposalActor string
	var proposalSequence int64
	if err := q.QueryRowContext(ctx, `SELECT actor_id,event_sequence,payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded'`,
		fact.Enactment.ProposalEventID, instanceID, branchID).Scan(&proposalActor, &proposalSequence, &proposalRaw); err != nil {
		return InstitutionFact{}, "", 0, "", core.NewError(core.CodeProjectionDiverged, "law notice lacks proposal source")
	}
	var proposal InstitutionFact
	if err := json.Unmarshal([]byte(proposalRaw), &proposal); err != nil || proposal.Version != "corerp.institution.v1" ||
		proposal.Kind != "law_proposal" || proposal.Proposal == nil || proposal.InstitutionID != fact.InstitutionID ||
		proposal.Proposal.Law != fact.Enactment.Law || proposal.Proposal.InstitutionEventID != fact.Enactment.InstitutionEventID ||
		proposal.Proposal.PreviousEnactmentEventID != fact.Enactment.PreviousEnactmentEventID ||
		proposal.Proposal.Repealed != fact.Enactment.Repealed || proposalSequence >= sequence || proposalActor == "" {
		return InstitutionFact{}, "", 0, "", core.NewError(core.CodeProjectionDiverged, "law notice proposal differs")
	}
	var speakerPrincipal string
	if err := q.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=? AND instance_id=? AND branch_id=?`,
		fact.Enactment.LegislatorID, instanceID, branchID).Scan(&speakerPrincipal); err != nil || speakerPrincipal != actor {
		return InstitutionFact{}, "", 0, "", core.NewError(core.CodeProjectionDiverged, "law notice legislator differs")
	}
	return fact, actor, sequence, worldTime, nil
}

func (s *Store) PublishRPPublicNotice(ctx context.Context, r RPPublicNoticePublishRequest) (RPInformationRecord, error) {
	if err := r.Validate(); err != nil {
		return RPInformationRecord{}, err
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "PublishRPPublicNotice", r,
		privateFactDomain{"rp_information_public", "RPInformationSent", `{"authorization":"institution-legislator-own-enactment-v1"}`},
		func(conn *sql.Conn) error {
			fact, actor, _, _, err := rpPublicLawSource(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.LawEventID)
			if err != nil {
				return err
			}
			if actor != r.Binding.PrincipalID || fact.Enactment.LegislatorID != r.SpeakerID {
				return core.NewError(core.CodeNotFound, "scoped law enactment not found")
			}
			if r.ControllerSessionID == "" {
				return authorizeInstitutionRole(ctx, conn, r.Binding, fact.InstitutionID, r.SpeakerID, institutionLegislate)
			}
			if err := requireRPSelectedNoticePublishWindow(ctx, conn, "information_public_publish", r.Binding,
				r.ControllerPrincipalID, r.ControllerSessionID, r.SpeakerID, r); err != nil {
				return err
			}
			return authorizeInstitutionControlledPublisher(ctx, conn, r.Binding, fact.InstitutionID, r.SpeakerID)
		},
		func(conn *sql.Conn, c privateFactContext) (RPInformationFact, func() error, error) {
			if err := requireRPSelectedNoticePublishWindow(ctx, conn, "information_public_publish", r.Binding,
				r.ControllerPrincipalID, r.ControllerSessionID, r.SpeakerID, r); err != nil {
				return RPInformationFact{}, nil, err
			}
			law, _, sourceSequence, sourceTime, err := rpPublicLawSource(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.LawEventID)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			sourceAt, sourceErr := time.Parse(time.RFC3339Nano, sourceTime)
			publishAt, publishErr := time.Parse(time.RFC3339Nano, c.WorldTime)
			if sourceSequence >= c.Sequence || sourceErr != nil || publishErr != nil || publishAt.Before(sourceAt) {
				return RPInformationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "notice predates law source")
			}
			var duplicate int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.MessageID).Scan(&duplicate); err != nil {
				return RPInformationFact{}, nil, err
			}
			if duplicate != 0 {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "message ID already used")
			}
			text := rpPublicLawText(law)
			if len(text) > 2000 {
				return RPInformationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "law notice exceeds bounded claim")
			}
			return RPInformationFact{Version: "corerp.information.v1", MessageID: r.MessageID,
				SenderID: r.SpeakerID, Text: text, Channel: "public_notice", Visibility: "public",
				ClaimedReliability: "official_statement", InstitutionID: law.InstitutionID,
				SourceLawEventID: r.LawEventID}, nil, nil
		})
}

// F3 control does not transfer an institutional role grant. The immutable
// legislator still owns the grant and Event; the selected controller only
// supplies the currently authorized RP decision to publish it.
func authorizeInstitutionControlledPublisher(ctx context.Context, conn *sql.Conn, b core.CareerBinding, institution, speaker string) error {
	if err := validateRPBinding(ctx, conn, b.InstanceID, b.BranchID, speaker); err != nil {
		return err
	}
	var owner string
	if err := conn.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=? AND instance_id=? AND branch_id=? AND status='active'`,
		speaker, b.InstanceID, b.BranchID).Scan(&owner); err != nil || owner != b.PrincipalID {
		return core.NewError(core.CodeUnauthorized, "public notice source principal differs from legislator")
	}
	role, status, source, err := readInstitutionRole(ctx, conn, b, institution, institutionLegislate)
	if err != nil {
		return err
	}
	if status != "active" || role.EntityID != speaker || role.PrincipalID != b.PrincipalID {
		return core.NewError(core.CodeUnauthorized, "institution role is revoked or assigned to another actor")
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g JOIN principals p ON p.principal_id=g.principal_id AND p.status='active'
	 WHERE g.grant_id=? AND g.principal_id=? AND g.instance_id=? AND g.branch_id=? AND g.subject_id=?
	 AND g.capability_id=? AND g.status='active' AND g.field_scope='[]' AND g.amount_limit_minor IS NULL AND g.definition_event_id=?`,
		role.GrantID, b.PrincipalID, b.InstanceID, b.BranchID, institution, institutionLegislate, source).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return core.NewError(core.CodeUnauthorized, "institution requires dedicated sourced role")
	}
	return nil
}

type RPPublicNoticeAccessRequest struct {
	Binding   core.CareerBinding `json:"binding"`
	SessionID string             `json:"session_id"`
	MessageID string             `json:"message_id"`
}

func (r RPPublicNoticeAccessRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !studioID(r.SessionID) || !studioID(r.MessageID) {
		return core.NewError(core.CodeInvalidArgument, "bounded public notice access required")
	}
	return nil
}

func rpPublicPublishedSource(ctx context.Context, conn *sql.Conn, instanceID, branchID, messageID string) (RPInformationFact, string, error) {
	var head int64
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, instanceID, branchID).Scan(&head); err != nil {
		return RPInformationFact{}, "", err
	}
	if _, _, err := rpInformationExpected(ctx, conn, instanceID, branchID, head); err != nil {
		return RPInformationFact{}, "", err
	}
	var eventID, raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent'
	 AND json_extract(payload,'$.message_id')=? AND json_extract(payload,'$.channel')='public_notice'`, instanceID, branchID, messageID).Scan(&eventID, &raw)
	if err == sql.ErrNoRows {
		return RPInformationFact{}, "", core.NewError(core.CodeNotFound, "public notice not available")
	}
	if err != nil {
		return RPInformationFact{}, "", err
	}
	var fact RPInformationFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.SourceLawEventID == "" {
		return RPInformationFact{}, "", core.NewError(core.CodeProjectionDiverged, "invalid public notice source")
	}
	return fact, eventID, nil
}

// A public board is discoverable by anyone in the world, but the publication
// itself is not their Knowledge. Access commits recipient-specific evidence.
func (s *Store) AccessRPPublicNotice(ctx context.Context, r RPPublicNoticeAccessRequest) (RPInformationRecord, error) {
	if err := r.Validate(); err != nil {
		return RPInformationRecord{}, err
	}
	var session RPSession
	return executePrivateFactCommand(s, ctx, r.Binding, "AccessRPPublicNotice", r,
		privateFactDomain{"rp_information_public_access", "RPInformationDelivered", `{"authorization":"rp-current-public-notice-reader-v1"}`},
		func(conn *sql.Conn) error {
			var err error
			session, err = loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return err
			}
			if session.InstanceID != r.Binding.InstanceID || session.BranchID != r.Binding.BranchID || session.Status != "active" {
				return core.NewError(core.CodeNotFound, "current RP notice session required")
			}
			if err := requireCurrentRPSession(ctx, conn, session); err != nil {
				return err
			}
			if err := authorizeRPControl(ctx, conn, r.Binding.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
				return err
			}
			_, _, err = rpPublicPublishedSource(ctx, conn, session.InstanceID, session.BranchID, r.MessageID)
			return err
		},
		func(conn *sql.Conn, c privateFactContext) (RPInformationFact, func() error, error) {
			if session.ObservationCursor != r.Binding.ExpectedHead {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "observe current world before reading public notice")
			}
			if err := requireRPPublicNoticeAccessWindow(ctx, conn, r); err != nil {
				return RPInformationFact{}, nil, err
			}
			published, sendID, err := rpPublicPublishedSource(ctx, conn, session.InstanceID, session.BranchID, r.MessageID)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			if published.SenderID == session.ControlledEntityID {
				return RPInformationFact{}, nil, core.NewError(core.CodeNotFound, "notice author is not a recipient")
			}
			var duplicate int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationDelivered'
			 AND json_extract(payload,'$.source_event_id')=? AND json_extract(payload,'$.recipient_id')=?`,
				session.InstanceID, session.BranchID, sendID, session.ControlledEntityID).Scan(&duplicate); err != nil {
				return RPInformationFact{}, nil, err
			}
			if duplicate != 0 {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "public notice already received")
			}
			var place string
			if err := conn.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, session.ControlledEntityID).Scan(&place); err != nil {
				return RPInformationFact{}, nil, classifyMissing(err, "notice recipient location")
			}
			delivered := published
			delivered.SourceEventID = sendID
			delivered.RecipientID = session.ControlledEntityID
			delivered.RecipientPlaceID = place
			delivered.DeliverWorldTime = c.WorldTime
			claimJSON, err := rpInformationClaimJSON(published)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			return delivered, func() error {
				observationID := "observation_" + c.EventID + "_" + session.ControlledEntityID
				claimKey := "information:" + sendID
				if err := execAgentOne(ctx, conn, "public notice observation", `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,?,?,?,?)`,
					observationID, c.EventID, session.ControlledEntityID, published.SenderID, place,
					published.Channel, c.WorldTime, claimKey, claimJSON); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "public notice knowledge", `INSERT INTO agent_knowledge(observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,0,?)`,
					session.ControlledEntityID, claimKey, published.SenderID, place, c.EventID,
					observationID, c.WorldTime, claimJSON, c.Sequence)
			}, nil
		})
}

type RPPublicNoticeSummary = RPOrganizationNoticeSummary
type RPPublicNoticeList = RPOrganizationNoticeList

func (s *Store) ReadRPPublicNotices(ctx context.Context, r core.RPSessionReadRequest) (RPPublicNoticeList, error) {
	var result RPPublicNoticeList
	if err := r.Validate(); err != nil {
		return result, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return result, err
	}
	if session.Status != "active" {
		return result, core.NewError(core.CodeBranchConflict, "notice listing requires active session")
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, session.InstanceID, session.BranchID).Scan(&result.WorldTime); err != nil {
		return result, err
	}
	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, session.InstanceID, session.BranchID).Scan(&head); err != nil {
		return result, err
	}
	if _, _, err := rpInformationExpected(ctx, tx.conn, session.InstanceID, session.BranchID, head); err != nil {
		return result, err
	}
	result.ProtocolVersion = RPClientProtocolVersion
	result.Notices = []RPPublicNoticeSummary{}
	rows, err := tx.conn.QueryContext(ctx, `SELECT p.world_time,json_extract(p.payload,'$.message_id'),
	 EXISTS(SELECT 1 FROM events d WHERE d.instance_id=p.instance_id AND d.branch_id=p.branch_id
	 AND d.event_type='RPInformationDelivered' AND json_extract(d.payload,'$.source_event_id')=p.event_id
	 AND json_extract(d.payload,'$.recipient_id')=?)
	 FROM events p WHERE p.instance_id=? AND p.branch_id=? AND p.event_type='RPInformationSent'
	 AND json_extract(p.payload,'$.channel')='public_notice' AND json_extract(p.payload,'$.sender_id')<>?
	 AND p.world_time<=? ORDER BY p.event_sequence DESC LIMIT 21`, session.ControlledEntityID,
		session.InstanceID, session.BranchID, session.ControlledEntityID, result.WorldTime)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item RPPublicNoticeSummary
		if err := rows.Scan(&item.PublishedWorldTime, &item.MessageID, &item.Accessed); err != nil {
			rows.Close()
			return result, err
		}
		result.Notices = append(result.Notices, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	result.More = len(result.Notices) > 20
	if result.More {
		result.Notices = result.Notices[:20]
	}
	return result, nil
}
