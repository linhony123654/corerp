package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpLocationProjectionRow struct {
	LocationID     string `json:"location_id"`
	InstanceID     string `json:"instance_id"`
	BranchID       string `json:"branch_id"`
	DisplayName    string `json:"display_name"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	PlaceSourceID  string `json:"place_source_id"`
	NodeInstanceID string `json:"node_instance_id"`
	NodeBranchID   string `json:"node_branch_id"`
	ParentID       string `json:"parent_id"`
	SlotKey        string `json:"slot_key"`
	ReadablePath   string `json:"readable_path"`
	Generator      string `json:"generator"`
	NodeSourceID   string `json:"node_source_id"`
}

func rpLocationExpectedRows(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpLocationProjectionRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_type,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPLocationMaterialized','RPLocationRenamed') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	want := map[string]rpLocationProjectionRow{}
	for rows.Next() {
		var id, kind, raw string
		if err := rows.Scan(&id, &kind, &raw); err != nil {
			return nil, err
		}
		if kind == "RPLocationMaterialized" {
			var fact RPLocationFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				return nil, err
			}
			hash, err := core.HashJSON([]string{instance, branch, fact.ParentLocationID, fact.SlotKey})
			if err != nil {
				return nil, err
			}
			if fact.Version != "corerp.spatial.location.v1" || fact.LocationID != "location_"+hash[7:] || fact.ParentLocationID == "" || !rpLocationSlotKey.MatchString(fact.SlotKey) || fact.DisplayName == "" || fact.ReadablePath == "" || !rpLocationSlotKey.MatchString(fact.GeneratorVersion) {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid location materialization source")
			}
			if _, exists := want[fact.LocationID]; exists {
				return nil, core.NewError(core.CodeProjectionDiverged, "duplicate location materialization source")
			}
			want[fact.LocationID] = rpLocationProjectionRow{LocationID: fact.LocationID, InstanceID: instance, BranchID: branch, DisplayName: fact.DisplayName, Kind: "public", Status: "active", PlaceSourceID: id, NodeInstanceID: instance, NodeBranchID: branch, ParentID: fact.ParentLocationID, SlotKey: fact.SlotKey, ReadablePath: fact.ReadablePath, Generator: fact.GeneratorVersion, NodeSourceID: id}
			continue
		}
		var rename RPLocationRenameFact
		if err := json.Unmarshal([]byte(raw), &rename); err != nil {
			return nil, err
		}
		if rename.Version != "corerp.spatial.rename.v1" || rename.LocationID == "" || rename.NewName == "" {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid location rename source")
		}
		if row, exists := want[rename.LocationID]; exists {
			if row.DisplayName != rename.OldName {
				return nil, core.NewError(core.CodeProjectionDiverged, "location rename source chronology differs")
			}
			row.DisplayName = rename.NewName
			want[rename.LocationID] = row
		}
		// Declared legacy roots can also be renamed; their original display
		// name belongs to their older defining Event and is audited separately.
	}
	return want, rows.Err()
}

func rpLocationProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpLocationExpectedRows(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(want))
	for id := range want {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	var differences []ProjectionDifference
	for _, id := range keys {
		var actual rpLocationProjectionRow
		err := q.QueryRowContext(ctx, `SELECT p.place_id,p.instance_id,p.branch_id,p.display_name,p.place_kind,p.status,p.definition_event_id,
			COALESCE(n.instance_id,''),COALESCE(n.branch_id,''),COALESCE(n.parent_location_id,''),COALESCE(n.slot_key,''),COALESCE(n.readable_path,''),COALESCE(n.generator_version,''),COALESCE(n.definition_event_id,'')
			FROM agent_places p LEFT JOIN rp_location_nodes n ON n.location_id=p.place_id WHERE p.place_id=?`, id).Scan(
			&actual.LocationID, &actual.InstanceID, &actual.BranchID, &actual.DisplayName, &actual.Kind, &actual.Status, &actual.PlaceSourceID,
			&actual.NodeInstanceID, &actual.NodeBranchID, &actual.ParentID, &actual.SlotKey, &actual.ReadablePath, &actual.Generator, &actual.NodeSourceID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		expectedJSON, err := core.CanonicalJSON(want[id])
		if err != nil {
			return nil, err
		}
		actualText := "missing"
		if err == nil {
			actualJSON, encErr := core.CanonicalJSON(actual)
			if encErr != nil {
				return nil, encErr
			}
			actualText = string(actualJSON)
		}
		if string(expectedJSON) != actualText {
			differences = append(differences, ProjectionDifference{Projection: "rp_location", Key: id, ExpectedText: string(expectedJSON), ActualText: actualText})
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT location_id FROM rp_location_nodes WHERE instance_id=? AND branch_id=? AND parent_location_id IS NOT NULL ORDER BY location_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		if _, known := want[id]; !known {
			differences = append(differences, ProjectionDifference{Projection: "rp_location_extra", Key: id, ExpectedText: "absent", ActualText: "unsourced child location"})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return differences, nil
}

func repairRPLocationProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, difference := range differences {
		if difference.Projection == "rp_location_extra" {
			return core.NewError(core.CodeProjectionDiverged, "cannot infer authority for extra child location")
		}
		if difference.Projection != "rp_location" {
			continue
		}
		var row rpLocationProjectionRow
		if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
			return err
		}
		if row.InstanceID != instance || row.BranchID != branch || row.LocationID != difference.Key {
			return core.NewError(core.CodeProjectionDiverged, "location repair scope differs")
		}
		if err := execAgentOne(ctx, conn, "repair materialized place", `INSERT INTO agent_places(place_id,instance_id,branch_id,display_name,place_kind,status,definition_event_id)
			VALUES (?,?,?,?,?,?,?) ON CONFLICT(place_id) DO UPDATE SET display_name=excluded.display_name,place_kind=excluded.place_kind,status=excluded.status,definition_event_id=excluded.definition_event_id
			WHERE agent_places.instance_id=excluded.instance_id AND agent_places.branch_id=excluded.branch_id`, row.LocationID, instance, branch, row.DisplayName, row.Kind, row.Status, row.PlaceSourceID); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "repair materialized location node", `INSERT INTO rp_location_nodes(location_id,instance_id,branch_id,parent_location_id,slot_key,readable_path,generator_version,definition_event_id)
			VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(location_id) DO UPDATE SET parent_location_id=excluded.parent_location_id,slot_key=excluded.slot_key,readable_path=excluded.readable_path,generator_version=excluded.generator_version,definition_event_id=excluded.definition_event_id
			WHERE rp_location_nodes.instance_id=excluded.instance_id AND rp_location_nodes.branch_id=excluded.branch_id`, row.LocationID, instance, branch, row.ParentID, row.SlotKey, row.ReadablePath, row.Generator, row.NodeSourceID); err != nil {
			return err
		}
	}
	return nil
}
