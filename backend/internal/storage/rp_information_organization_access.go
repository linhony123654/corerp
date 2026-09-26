package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

type RPOrganizationNoticeAccessRequest struct {
	Binding   core.CareerBinding `json:"binding"`
	SessionID string             `json:"session_id"`
	MessageID string             `json:"message_id"`
}

func (r RPOrganizationNoticeAccessRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !studioID(r.SessionID) || !studioID(r.MessageID) {
		return core.NewError(core.CodeInvalidArgument, "bounded organization notice access required")
	}
	return nil
}

func rpOrganizationEmployeeSource(ctx context.Context, conn *sql.Conn, instanceID, branchID, employeeID, organizationID, worldTime string) (string, string, error) {
	now, err := time.Parse(time.RFC3339Nano, worldTime)
	if err != nil {
		return "", "", core.NewError(core.CodeProjectionDiverged, "invalid organization access world time")
	}
	base := time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)
	day := int(now.Sub(base) / (24 * time.Hour))
	var contractID string
	err = conn.QueryRowContext(ctx, `SELECT c.contract_id FROM employment_contracts c JOIN events e ON e.event_id=c.definition_event_id
	 WHERE c.employee_entity_id=? AND c.employer_entity_id=? AND c.status='active' AND c.starts_on_day<=?
	 AND e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded' ORDER BY c.contract_id LIMIT 1`,
		employeeID, organizationID, day, instanceID, branchID).Scan(&contractID)
	if err == sql.ErrNoRows {
		return "", "", core.NewError(core.CodeNotFound, "active organization notice audience required")
	}
	if err != nil {
		return "", "", err
	}
	job, termsEventID, _, err := currentCareerEmployment(ctx, conn, core.CareerBinding{InstanceID: instanceID, BranchID: branchID}, contractID, worldTime)
	if err != nil || job.EmployeeID != employeeID || job.OrganizationID != organizationID {
		return "", "", core.NewError(core.CodeNotFound, "active organization notice audience required")
	}
	return contractID, termsEventID, nil
}

func rpOrganizationEmployeeEligible(ctx context.Context, conn *sql.Conn, instanceID, branchID, employeeID, organizationID, worldTime string) error {
	_, _, err := rpOrganizationEmployeeSource(ctx, conn, instanceID, branchID, employeeID, organizationID, worldTime)
	return err
}

func rpOrganizationPublishedSource(ctx context.Context, conn *sql.Conn, instanceID, branchID, messageID string) (RPInformationFact, string, error) {
	var head int64
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, instanceID, branchID).Scan(&head); err != nil {
		return RPInformationFact{}, "", err
	}
	if _, _, err := rpInformationExpected(ctx, conn, instanceID, branchID, head); err != nil {
		return RPInformationFact{}, "", err
	}
	var eventID, raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent'
	 AND json_extract(payload,'$.message_id')=? AND json_extract(payload,'$.channel')='organization_announcement'`, instanceID, branchID, messageID).Scan(&eventID, &raw)
	if err == sql.ErrNoRows {
		return RPInformationFact{}, "", core.NewError(core.CodeNotFound, "organization notice not available")
	}
	if err != nil {
		return RPInformationFact{}, "", err
	}
	var fact RPInformationFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.OrganizationID == "" || fact.SourceCareerEventID == "" {
		return RPInformationFact{}, "", core.NewError(core.CodeProjectionDiverged, "organization notice source invalid")
	}
	return fact, eventID, nil
}

// Access is an actor action. The publication alone is not a learning Event;
// this command commits the exact recipient's immutable delivery evidence.
func (s *Store) AccessRPOrganizationNotice(ctx context.Context, r RPOrganizationNoticeAccessRequest) (RPInformationRecord, error) {
	if err := r.Validate(); err != nil {
		return RPInformationRecord{}, err
	}
	var session RPSession
	return executePrivateFactCommand(s, ctx, r.Binding, "AccessRPOrganizationNotice", r,
		privateFactDomain{"rp_information_org_access", "RPInformationDelivered", `{"authorization":"rp-current-organization-employee-v1"}`},
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
			published, _, err := rpOrganizationPublishedSource(ctx, conn, session.InstanceID, session.BranchID, r.MessageID)
			if err != nil {
				return err
			}
			var worldTime string
			if err := conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`,
				session.InstanceID, session.BranchID).Scan(&worldTime); err != nil {
				return err
			}
			return rpOrganizationEmployeeEligible(ctx, conn, session.InstanceID, session.BranchID,
				session.ControlledEntityID, published.OrganizationID, worldTime)
		},
		func(conn *sql.Conn, c privateFactContext) (RPInformationFact, func() error, error) {
			if session.ObservationCursor != r.Binding.ExpectedHead {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "observe current world before reading notice")
			}
			if err := requireRPOrganizationNoticeAccessWindow(ctx, conn, r); err != nil {
				return RPInformationFact{}, nil, err
			}
			published, sendID, err := rpOrganizationPublishedSource(ctx, conn, session.InstanceID, session.BranchID, r.MessageID)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			if published.SenderID == session.ControlledEntityID {
				return RPInformationFact{}, nil, core.NewError(core.CodeNotFound, "notice author is not a recipient")
			}
			contractID, termsEventID, err := rpOrganizationEmployeeSource(ctx, conn, session.InstanceID, session.BranchID,
				session.ControlledEntityID, published.OrganizationID, c.WorldTime)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			var duplicate int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationDelivered'
			 AND json_extract(payload,'$.source_event_id')=? AND json_extract(payload,'$.recipient_id')=?`,
				session.InstanceID, session.BranchID, sendID, session.ControlledEntityID).Scan(&duplicate); err != nil {
				return RPInformationFact{}, nil, err
			}
			if duplicate != 0 {
				return RPInformationFact{}, nil, core.NewError(core.CodeBranchConflict, "organization notice already received")
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
			delivered.AudienceContractID = contractID
			delivered.AudienceTermsEventID = termsEventID
			claimJSON, err := rpInformationClaimJSON(published)
			if err != nil {
				return RPInformationFact{}, nil, err
			}
			return delivered, func() error {
				observationID := "observation_" + c.EventID + "_" + session.ControlledEntityID
				claimKey := "information:" + sendID
				if err := execAgentOne(ctx, conn, "organization notice observation", `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,?,?,?,?)`,
					observationID, c.EventID, session.ControlledEntityID, published.SenderID, place,
					published.Channel, c.WorldTime, claimKey, claimJSON); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "organization notice knowledge", `INSERT INTO agent_knowledge(observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,0,?)`,
					session.ControlledEntityID, claimKey, published.SenderID, place, c.EventID,
					observationID, c.WorldTime, claimJSON, c.Sequence)
			}, nil
		})
}
