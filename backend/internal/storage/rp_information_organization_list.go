package storage

import (
	"context"
	"time"

	"corerp.local/backend/internal/core"
)

type RPOrganizationNoticeSummary struct {
	MessageID          string `json:"message_id"`
	PublishedWorldTime string `json:"published_world_time"`
	Accessed           bool   `json:"accessed"`
}

type RPOrganizationNoticeList struct {
	ProtocolVersion string                        `json:"protocol_version"`
	WorldTime       string                        `json:"world_time"`
	Notices         []RPOrganizationNoticeSummary `json:"notices"`
	More            bool                          `json:"more"`
}

// Discovery exposes notice handles and publication times, never claim text,
// affected employee identity or private Career source payload. It is a pure
// read: listing a notice does not teach its claim to the actor.
func (s *Store) ReadRPOrganizationNotices(ctx context.Context, r core.RPSessionReadRequest) (RPOrganizationNoticeList, error) {
	var result RPOrganizationNoticeList
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
	if err := tx.conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`,
		session.InstanceID, session.BranchID).Scan(&result.WorldTime); err != nil {
		return result, err
	}
	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`,
		session.InstanceID, session.BranchID).Scan(&head); err != nil {
		return result, err
	}
	if _, _, err := rpInformationExpected(ctx, tx.conn, session.InstanceID, session.BranchID, head); err != nil {
		return result, err
	}
	result.ProtocolVersion = RPClientProtocolVersion
	result.Notices = []RPOrganizationNoticeSummary{}
	now, err := time.Parse(time.RFC3339Nano, result.WorldTime)
	if err != nil {
		return result, core.NewError(core.CodeProjectionDiverged, "invalid notice listing world time")
	}
	base := time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)
	day := int(now.Sub(base) / (24 * time.Hour))
	rows, err := tx.conn.QueryContext(ctx, `SELECT DISTINCT p.event_id,p.world_time,
	 json_extract(p.payload,'$.message_id'),json_extract(p.payload,'$.organization_id'),
	 EXISTS(SELECT 1 FROM events d WHERE d.instance_id=p.instance_id AND d.branch_id=p.branch_id
	 AND d.event_type='RPInformationDelivered' AND json_extract(d.payload,'$.source_event_id')=p.event_id
	 AND json_extract(d.payload,'$.recipient_id')=?)
	 FROM events p JOIN employment_contracts c ON c.employer_entity_id=json_extract(p.payload,'$.organization_id')
	 JOIN events origin ON origin.event_id=c.definition_event_id AND origin.instance_id=p.instance_id AND origin.branch_id=p.branch_id
	 WHERE p.instance_id=? AND p.branch_id=? AND p.event_type='RPInformationSent'
	 AND json_extract(p.payload,'$.channel')='organization_announcement'
	 AND json_extract(p.payload,'$.sender_id')<>? AND c.employee_entity_id=? AND c.status='active' AND c.starts_on_day<=?
	 AND p.world_time<=? ORDER BY p.event_sequence DESC LIMIT 21`, session.ControlledEntityID,
		session.InstanceID, session.BranchID, session.ControlledEntityID, session.ControlledEntityID, day, result.WorldTime)
	if err != nil {
		return result, err
	}
	type candidate struct {
		ID, Published, MessageID, OrganizationID string
		Accessed                                 bool
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.ID, &item.Published, &item.MessageID, &item.OrganizationID, &item.Accessed); err != nil {
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
	result.More = len(candidates) > 20
	if result.More {
		candidates = candidates[:20]
	}
	for _, item := range candidates {
		if err := rpOrganizationEmployeeEligible(ctx, tx.conn, session.InstanceID, session.BranchID,
			session.ControlledEntityID, item.OrganizationID, result.WorldTime); err != nil {
			return result, err
		}
		result.Notices = append(result.Notices, RPOrganizationNoticeSummary{MessageID: item.MessageID,
			PublishedWorldTime: item.Published, Accessed: item.Accessed})
	}
	return result, nil
}
