package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type RPNoticePublicationSourceReadRequest struct {
	PrincipalID string `json:"principal_id"`
	SessionID   string `json:"session_id"`
	Channel     string `json:"channel"`
}

type RPNoticePublicationSource struct {
	SourceHandle string `json:"source_handle"`
	Channel      string `json:"channel"`
	Summary      string `json:"summary"`
	WorldTime    string `json:"world_time"`
	// Event ID, Agent principal, employee, contract and private notice are
	// intentionally absent from this model-facing business-source view.
}

type RPNoticePublicationSourceList struct {
	ProtocolVersion string                      `json:"protocol_version"`
	Sources         []RPNoticePublicationSource `json:"sources"`
	More            bool                        `json:"more"`
}

func (r RPNoticePublicationSourceReadRequest) Validate() error {
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) ||
		(r.Channel != "public_notice" && r.Channel != "organization_announcement") {
		return core.NewError(core.CodeInvalidArgument, "scoped publication-source read required")
	}
	return nil
}

func rpPublicationSourceHandle(channel, sessionID, eventID string) (string, error) {
	hash, err := core.HashJSON([]string{"f7-publication-source-v1", channel, sessionID, eventID})
	if err != nil {
		return "", err
	}
	return "pns_" + hash[7:], nil
}

// This is the only model-facing discovery of business source evidence for
// F7 publication. It requires current RP control and the source Actor's
// still-active, purpose-specific Career/Institution grant.
func (s *Store) ReadRPNoticePublicationSources(ctx context.Context, r RPNoticePublicationSourceReadRequest) (RPNoticePublicationSourceList, error) {
	var empty RPNoticePublicationSourceList
	if err := r.Validate(); err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return empty, err
	}
	if session.Status != "active" {
		return empty, core.NewError(core.CodeBranchConflict, "publication source requires active RP session")
	}
	if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
		return empty, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	return rpNoticePublicationSourcesForSession(ctx, tx.conn, session, r.Channel)
}

func rpNoticePublicationSourcesForSession(ctx context.Context, conn *sql.Conn, session RPSession, channel string) (RPNoticePublicationSourceList, error) {
	var result RPNoticePublicationSourceList
	result.ProtocolVersion = RPClientProtocolVersion
	result.Sources = []RPNoticePublicationSource{}
	if channel != "public_notice" && channel != "organization_announcement" {
		return result, core.NewError(core.CodeInvalidArgument, "unknown publication source channel")
	}
	if err := validateRPBinding(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	var owner string
	if err := conn.QueryRowContext(ctx, `SELECT a.principal_id FROM agent_profiles a JOIN principals p ON p.principal_id=a.principal_id AND p.status='active'
	 WHERE a.agent_id=? AND a.instance_id=? AND a.branch_id=? AND a.status='active'`,
		session.ControlledEntityID, session.InstanceID, session.BranchID).Scan(&owner); err != nil {
		return result, classifyMissing(err, "controlled publication source Actor")
	}
	condition := `event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_enactment'`
	if channel == "organization_announcement" {
		condition = `event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='employment' AND json_extract(payload,'$.exit.kind')='layoff'`
	}
	rows, err := conn.QueryContext(ctx, `SELECT event_id,world_time FROM events WHERE instance_id=? AND branch_id=? AND actor_id=? AND `+condition+` ORDER BY event_sequence DESC LIMIT 101`,
		session.InstanceID, session.BranchID, owner)
	if err != nil {
		return result, err
	}
	type candidate struct{ id, at string }
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.at); err != nil {
			rows.Close()
			return result, err
		}
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	// Deliberately bounded. A later pagination contract can serve older
	// business sources without turning this into a general Event browser.
	if len(candidates) > 100 {
		result.More = true
		candidates = candidates[:100]
	}
	b := core.CareerBinding{PrincipalID: owner, InstanceID: session.InstanceID, BranchID: session.BranchID}
	for _, item := range candidates {
		var summary string
		if channel == "public_notice" {
			fact, actor, _, _, err := rpPublicLawSource(ctx, conn, session.InstanceID, session.BranchID, item.id)
			if err != nil {
				return result, err
			}
			if actor != owner || fact.Enactment.LegislatorID != session.ControlledEntityID {
				return result, core.NewError(core.CodeProjectionDiverged, "law publication source Actor differs")
			}
			err = authorizeInstitutionControlledPublisher(ctx, conn, b, fact.InstitutionID, session.ControlledEntityID)
			if core.HasCode(err, core.CodeUnauthorized) {
				continue
			}
			if err != nil {
				return result, err
			}
			summary = rpPublicLawText(fact)
		} else {
			fact, actor, _, _, err := rpOrganizationLayoffSource(ctx, conn, session.InstanceID, session.BranchID, item.id)
			if err != nil {
				return result, err
			}
			if actor != owner {
				return result, core.NewError(core.CodeProjectionDiverged, "Career publication source Actor differs")
			}
			err = authorizeCareerManager(ctx, conn, b, fact.OrganizationID)
			if core.HasCode(err, core.CodeUnauthorized) {
				continue
			}
			if err != nil {
				return result, err
			}
			summary = rpOrganizationLayoffText(fact.Exit.EffectiveFromDay)
		}
		handle, err := rpPublicationSourceHandle(channel, session.SessionID, item.id)
		if err != nil {
			return result, err
		}
		result.Sources = append(result.Sources, RPNoticePublicationSource{SourceHandle: handle, Channel: channel,
			Summary: summary, WorldTime: item.at})
		if len(result.Sources) > 20 {
			result.More = true
			result.Sources = result.Sources[:20]
			break
		}
	}
	return result, nil
}

func resolveRPNoticePublicationSource(ctx context.Context, conn *sql.Conn, session RPSession, channel, handle string) (string, error) {
	if !studioID(handle) {
		return "", core.NewError(core.CodeInvalidArgument, "bounded publication source handle required")
	}
	var owner string
	if err := conn.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=? AND instance_id=? AND branch_id=? AND status='active'`,
		session.ControlledEntityID, session.InstanceID, session.BranchID).Scan(&owner); err != nil {
		return "", classifyMissing(err, "controlled publication source Actor")
	}
	condition := `event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_enactment'`
	if channel == "organization_announcement" {
		condition = `event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='employment' AND json_extract(payload,'$.exit.kind')='layoff'`
	}
	rows, err := conn.QueryContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND actor_id=? AND `+condition+` ORDER BY event_sequence DESC LIMIT 100`,
		session.InstanceID, session.BranchID, owner)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return "", err
		}
		candidate, err := rpPublicationSourceHandle(channel, session.SessionID, eventID)
		if err != nil {
			return "", err
		}
		if candidate == handle {
			return eventID, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", core.NewError(core.CodeNotFound, "own publication source handle not found")
}
