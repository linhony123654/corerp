package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type VisibleEvent struct {
	Sequence  int64           `json:"-"`
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	WorldTime string          `json:"world_time"`
	Payload   json.RawMessage `json:"payload"`
	Redacted  bool            `json:"redacted"`
}

type VisibleEventPage struct {
	Events         []VisibleEvent `json:"events"`
	ScannedThrough int64          `json:"-"`
}

func (s *Store) ListVisibleEvents(ctx context.Context, request core.VisibleEventRequest) (VisibleEventPage, error) {
	if err := request.Validate(); err != nil {
		return VisibleEventPage{}, err
	}
	var principalType string
	err := s.db.QueryRowContext(ctx, `
		SELECT p.principal_type
		FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		WHERE g.principal_id = ? AND p.status = 'active' AND g.capability_id = ?
		  AND g.instance_id = ? AND g.branch_id = ? AND g.subject_id IN (?, '*') AND g.status = 'active'
		ORDER BY CASE WHEN g.subject_id = ? THEN 0 ELSE 1 END, g.grant_id LIMIT 1`,
		request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID,
		request.SubjectID, request.SubjectID,
	).Scan(&principalType)
	if errors.Is(err, sql.ErrNoRows) {
		return VisibleEventPage{}, core.NewError(core.CodeUnauthorized, "principal lacks visible-event scope")
	}
	if err != nil {
		return VisibleEventPage{}, core.WrapError(core.CodeStorageFailure, "read visible-event grant", err)
	}
	switch principalType {
	case "creator":
		if request.CapabilityID != "world.events.read" {
			return VisibleEventPage{}, core.NewError(core.CodeUnauthorized, "creator event capability is invalid")
		}
	case "operator":
		if request.CapabilityID != "diagnostics.events.read" {
			return VisibleEventPage{}, core.NewError(core.CodeUnauthorized, "operator event capability is invalid")
		}
	case "player":
		if request.CapabilityID != "world.events.read" {
			return VisibleEventPage{}, core.NewError(core.CodeUnauthorized, "player event capability is invalid")
		}
	default:
		return VisibleEventPage{}, core.NewError(core.CodeUnauthorized, "principal type cannot subscribe to events")
	}

	visibility := eventVisibility{}
	if principalType == "player" {
		visibility, err = s.loadPlayerEventVisibility(ctx, request.SubjectID)
		if err != nil {
			return VisibleEventPage{}, err
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_sequence, event_id, event_type, world_time, payload
		FROM events
		WHERE instance_id = ? AND branch_id = ? AND event_sequence > ?
		ORDER BY event_sequence LIMIT 10000`, request.InstanceID, request.BranchID, request.AfterSequence)
	if err != nil {
		return VisibleEventPage{}, core.WrapError(core.CodeStorageFailure, "read visible events", err)
	}
	defer rows.Close()
	page := VisibleEventPage{Events: make([]VisibleEvent, 0, request.Limit), ScannedThrough: request.AfterSequence}
	for rows.Next() {
		var event VisibleEvent
		var payload string
		if err := rows.Scan(&event.Sequence, &event.EventID, &event.EventType, &event.WorldTime, &payload); err != nil {
			return VisibleEventPage{}, core.WrapError(core.CodeStorageFailure, "scan visible event", err)
		}
		event.Payload = json.RawMessage(payload)
		page.ScannedThrough = event.Sequence
		if principalType == "player" && !visibility.allows(event) {
			continue
		}
		if principalType == "operator" {
			event.Payload = json.RawMessage(`{"redacted":true}`)
			event.Redacted = true
		}
		page.Events = append(page.Events, event)
		if len(page.Events) == request.Limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return VisibleEventPage{}, core.WrapError(core.CodeStorageFailure, "iterate visible events", err)
	}
	return page, nil
}

type eventVisibility struct {
	accountID     string
	obligationIDs []string
}

func (s *Store) loadPlayerEventVisibility(ctx context.Context, subjectID string) (eventVisibility, error) {
	var visibility eventVisibility
	if err := s.db.QueryRowContext(ctx, `SELECT account_id FROM economic_entities WHERE entity_id = ? AND entity_kind IN ('employee', 'household')`, subjectID).Scan(&visibility.accountID); err != nil {
		return eventVisibility{}, classifyMissing(err, "player event subject")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.obligation_id FROM wage_obligations o
		JOIN employment_contracts c ON c.contract_id = o.contract_id WHERE c.employee_entity_id = ?
		UNION ALL
		SELECT o.obligation_id FROM rent_obligations o
		JOIN rent_contracts c ON c.contract_id = o.contract_id WHERE c.tenant_entity_id = ?`, subjectID, subjectID)
	if err != nil {
		return eventVisibility{}, core.WrapError(core.CodeStorageFailure, "read player event obligations", err)
	}
	defer rows.Close()
	for rows.Next() {
		var obligationID string
		if err := rows.Scan(&obligationID); err != nil {
			return eventVisibility{}, core.WrapError(core.CodeStorageFailure, "scan player event obligation", err)
		}
		visibility.obligationIDs = append(visibility.obligationIDs, obligationID)
	}
	if err := rows.Err(); err != nil {
		return eventVisibility{}, core.WrapError(core.CodeStorageFailure, "iterate player event obligations", err)
	}
	sort.Strings(visibility.obligationIDs)
	return visibility, nil
}

func (v eventVisibility) allows(event VisibleEvent) bool {
	if event.EventType == "WorldTimeAdvanced" {
		return true
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return false
	}
	for _, key := range []string{"buyer_account_id", "target_account_id"} {
		var accountID string
		if raw, ok := payload[key]; ok && json.Unmarshal(raw, &accountID) == nil && accountID == v.accountID {
			return true
		}
	}
	var obligationID string
	if raw, ok := payload["obligation_id"]; ok && json.Unmarshal(raw, &obligationID) == nil {
		index := sort.SearchStrings(v.obligationIDs, obligationID)
		return index < len(v.obligationIDs) && v.obligationIDs[index] == obligationID
	}
	return false
}
