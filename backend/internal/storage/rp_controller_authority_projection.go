package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpControllerAuthorityProjection struct {
	Instance    string `json:"instance"`
	Branch      string `json:"branch"`
	Entity      string `json:"entity"`
	Principal   string `json:"principal"`
	Controller  string `json:"controller"`
	Generation  int64  `json:"generation"`
	Status      string `json:"status"`
	SourceEvent string `json:"source_event"`
	WorldTime   string `json:"world_time"`
}

func rpControllerAuthorityExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpControllerAuthorityProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_type,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPExternalControllerEnrolled','RPExternalControllerReplaced','RPExternalControllerAssigned','RPExternalControllerReleased') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	want := map[string]rpControllerAuthorityProjection{}
	enrollments := map[string]rpExternalEnrollmentProjection{}
	for rows.Next() {
		var eventID, eventType, at, raw string
		if err := rows.Scan(&eventID, &eventType, &at, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		switch eventType {
		case "RPExternalControllerEnrolled", "RPExternalControllerReplaced":
			if eventType == "RPExternalControllerReplaced" {
				var fact RPExternalControllerReplacementFact
				if err := json.Unmarshal([]byte(raw), &fact); err != nil {
					rows.Close()
					return nil, err
				}
				previous, exists := want[fact.EntityID]
				if !exists || previous.Status != "released" || previous.Generation != fact.ReleasedGeneration {
					rows.Close()
					return nil, core.NewError(core.CodeProjectionDiverged, "controller replacement was not made at released generation")
				}
			}
			if err := applyRPExternalEnrollmentSource(enrollments, instance, branch, eventID, eventType, at, raw); err != nil {
				rows.Close()
				return nil, err
			}
		case "RPExternalControllerAssigned":
			var fact RPExternalControllerAssignmentFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, err
			}
			previous, exists := want[fact.EntityID]
			first := !exists && fact.Version == "corerp.controller-assignment.v1" && fact.Generation == 1
			reassigned := exists && previous.Status == "released" && fact.Version == "corerp.controller-assignment.v2" && fact.Generation == previous.Generation+1
			if !studioID(fact.EntityID) || !studioID(fact.ControllerPrincipalID) || !rpLocationSlotKey.MatchString(fact.ControllerInstanceID) || (!first && !reassigned) {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid external controller assignment source")
			}
			enrolled, ok := enrollments[fact.EntityID]
			if !ok || enrolled.Principal != fact.ControllerPrincipalID || enrolled.Controller != fact.ControllerInstanceID {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "controller assignment differs from enrollment source")
			}
			want[fact.EntityID] = rpControllerAuthorityProjection{instance, branch, fact.EntityID, fact.ControllerPrincipalID, fact.ControllerInstanceID, fact.Generation, "active", eventID, at}
		case "RPExternalControllerReleased":
			var fact RPExternalControllerReleaseFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, err
			}
			previous, exists := want[fact.EntityID]
			if !exists || previous.Status != "active" || fact.Version != "corerp.controller-release.v1" || previous.Principal != fact.ControllerPrincipalID || previous.Controller != fact.ControllerInstanceID || fact.Generation != previous.Generation+1 {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid external controller release source")
			}
			previous.Generation, previous.Status, previous.SourceEvent = fact.Generation, "released", eventID
			want[fact.EntityID] = previous
		}
	}
	err = rows.Err()
	rows.Close()
	return want, err
}

func rpControllerAuthorityDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpControllerAuthorityExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT instance_id,branch_id,entity_id,principal_id,controller_instance_id,generation,status,source_event_id,assigned_world_time FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? ORDER BY entity_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	got := map[string]rpControllerAuthorityProjection{}
	for rows.Next() {
		var row rpControllerAuthorityProjection
		if err := rows.Scan(&row.Instance, &row.Branch, &row.Entity, &row.Principal, &row.Controller, &row.Generation, &row.Status, &row.SourceEvent, &row.WorldTime); err != nil {
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
			differences = append(differences, ProjectionDifference{Projection: "rp_controller_authority", Key: key, ExpectedText: expected, ActualText: actual})
		}
	}
	return differences, nil
}

func repairRPControllerAuthorities(ctx context.Context, conn *sql.Conn, instance, branch string, through int64, differences []ProjectionDifference) error {
	if len(differences) == 0 {
		return nil
	}
	want, err := rpControllerAuthorityExpected(ctx, conn, instance, branch, through)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM rp_controller_authorities WHERE instance_id=? AND branch_id=?`, instance, branch); err != nil {
		return err
	}
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		row := want[key]
		if _, err := conn.ExecContext(ctx, `INSERT INTO rp_controller_authorities(instance_id,branch_id,entity_id,principal_id,controller_instance_id,generation,status,source_event_id,assigned_world_time) VALUES (?,?,?,?,?,?,?,?,?)`, row.Instance, row.Branch, row.Entity, row.Principal, row.Controller, row.Generation, row.Status, row.SourceEvent, row.WorldTime); err != nil {
			return err
		}
	}
	return nil
}
