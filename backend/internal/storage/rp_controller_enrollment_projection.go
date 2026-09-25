package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"

	"corerp.local/backend/internal/core"
)

type rpExternalEnrollmentProjection struct {
	Instance    string `json:"instance"`
	Branch      string `json:"branch"`
	Entity      string `json:"entity"`
	Principal   string `json:"principal"`
	Controller  string `json:"controller"`
	SourceEvent string `json:"source_event"`
	WorldTime   string `json:"world_time"`
}

func rpExternalEnrollmentExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpExternalEnrollmentProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_type,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPExternalControllerEnrolled','RPExternalControllerReplaced') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	want := map[string]rpExternalEnrollmentProjection{}
	for rows.Next() {
		var eventID, eventType, worldTime, raw string
		if err := rows.Scan(&eventID, &eventType, &worldTime, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := applyRPExternalEnrollmentSource(want, instance, branch, eventID, eventType, worldTime, raw); err != nil {
			rows.Close()
			return nil, err
		}
	}
	err = rows.Err()
	rows.Close()
	return want, err
}

func applyRPExternalEnrollmentSource(want map[string]rpExternalEnrollmentProjection, instance, branch, eventID, eventType, worldTime, raw string) error {
	var entity, principal, controller string
	switch eventType {
	case "RPExternalControllerEnrolled":
		var fact RPExternalControllerEnrollmentFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return err
		}
		if fact.Version != "corerp.controller-enrollment.v1" || len(want) >= 2 {
			return core.NewError(core.CodeProjectionDiverged, "invalid external controller enrollment source")
		}
		entity, principal, controller = fact.EntityID, fact.ControllerPrincipalID, fact.ControllerInstanceID
		if _, exists := want[entity]; exists {
			return core.NewError(core.CodeProjectionDiverged, "external controller Entity enrolled twice")
		}
	case "RPExternalControllerReplaced":
		var fact RPExternalControllerReplacementFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return err
		}
		previous, exists := want[fact.EntityID]
		if fact.Version != "corerp.controller-replacement.v1" || fact.ReleasedGeneration < 2 || !exists ||
			previous.Principal != fact.OldControllerPrincipalID || previous.Controller != fact.OldControllerInstanceID ||
			previous.Principal == fact.ControllerPrincipalID || previous.Controller == fact.ControllerInstanceID {
			return core.NewError(core.CodeProjectionDiverged, "invalid external controller replacement source")
		}
		entity, principal, controller = fact.EntityID, fact.ControllerPrincipalID, fact.ControllerInstanceID
	default:
		return core.NewError(core.CodeProjectionDiverged, "unknown external enrollment source")
	}
	if !studioID(entity) || !studioID(principal) || !rpLocationSlotKey.MatchString(controller) ||
		strings.TrimSpace(entity) != entity || strings.TrimSpace(principal) != principal ||
		entity == principal || entity == controller || principal == controller {
		return core.NewError(core.CodeProjectionDiverged, "invalid external controller identity source")
	}
	for other, row := range want {
		if other != entity && (row.Principal == principal || row.Controller == controller) {
			return core.NewError(core.CodeProjectionDiverged, "duplicate external controller principal or instance")
		}
	}
	want[entity] = rpExternalEnrollmentProjection{instance, branch, entity, principal, controller, eventID, worldTime}
	return nil
}

func rpExternalEnrollmentDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpExternalEnrollmentExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT instance_id,branch_id,entity_id,principal_id,controller_instance_id,source_event_id,enrolled_world_time FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=? ORDER BY entity_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	got := map[string]rpExternalEnrollmentProjection{}
	for rows.Next() {
		var row rpExternalEnrollmentProjection
		if err := rows.Scan(&row.Instance, &row.Branch, &row.Entity, &row.Principal, &row.Controller, &row.SourceEvent, &row.WorldTime); err != nil {
			rows.Close()
			return nil, err
		}
		got[row.Entity] = row
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for key := range want {
		keys[key] = true
	}
	for key := range got {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var differences []ProjectionDifference
	for _, key := range ordered {
		expected, actual := "missing", "missing"
		if row, ok := want[key]; ok {
			encoded, err := core.CanonicalJSON(row)
			if err != nil {
				return nil, err
			}
			expected = string(encoded)
		}
		if row, ok := got[key]; ok {
			encoded, err := core.CanonicalJSON(row)
			if err != nil {
				return nil, err
			}
			actual = string(encoded)
		}
		if expected != actual {
			differences = append(differences, ProjectionDifference{Projection: "rp_external_controller_enrollment", Key: key, ExpectedText: expected, ActualText: actual})
		}
	}
	return differences, nil
}

func repairRPExternalEnrollments(ctx context.Context, conn *sql.Conn, instance, branch string, through int64, differences []ProjectionDifference) error {
	if len(differences) == 0 {
		return nil
	}
	want, err := rpExternalEnrollmentExpected(ctx, conn, instance, branch, through)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=?`, instance, branch); err != nil {
		return err
	}
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		row := want[key]
		if _, err := conn.ExecContext(ctx, `INSERT INTO rp_external_controller_enrollments(instance_id,branch_id,entity_id,principal_id,controller_instance_id,source_event_id,enrolled_world_time) VALUES (?,?,?,?,?,?,?)`, row.Instance, row.Branch, row.Entity, row.Principal, row.Controller, row.SourceEvent, row.WorldTime); err != nil {
			return err
		}
	}
	return nil
}
