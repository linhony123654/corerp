package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"corerp.local/backend/internal/core"
)

type rpSceneObjectProjection struct {
	ObjectID          string
	ObjectKey         string
	DisplayName       string
	ObjectKind        string
	PlaceID           string
	State             string
	DefinitionEvent   string
	StateEvent        string
	ProjectionVersion int64
	LastSequence      int64
}

func rpSceneObjectsExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpSceneObjectProjection, error) {
	var genesisRaw string
	err := q.QueryRowContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='StudioWorldPrepared' AND event_sequence<=?`, instance, branch, through).Scan(&genesisRaw)
	if err == sql.ErrNoRows {
		return map[string]rpSceneObjectProjection{}, nil
	}
	if err != nil {
		return nil, err
	}
	var genesis struct {
		Request StudioGenesisRequest `json:"request"`
	}
	if err := json.Unmarshal([]byte(genesisRaw), &genesis); err != nil {
		return nil, err
	}
	if genesis.Request.InstanceID != instance || genesis.Request.Spec.Version != core.StudioWorldSpecVersion {
		return nil, core.NewError(core.CodeProjectionDiverged, "scene object genesis differs")
	}
	var definitionEvent string
	var definitionSequence int64
	var spatialRaw string
	if err := q.QueryRowContext(ctx, `SELECT event_id,event_sequence,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='StudioSpatialPrepared' AND event_sequence<=?`, instance, branch, through).Scan(&definitionEvent, &definitionSequence, &spatialRaw); err == sql.ErrNoRows {
		// World preparation and participant materialization precede the spatial
		// event. Objects are not projected until that authoritative definition.
		return map[string]rpSceneObjectProjection{}, nil
	} else if err != nil {
		return nil, err
	}
	var spatial StudioSpatialFact
	if err := json.Unmarshal([]byte(spatialRaw), &spatial); err != nil {
		return nil, err
	}
	if spatial.ObjectCount != len(genesis.Request.Spec.Objects) {
		return nil, core.NewError(core.CodeProjectionDiverged, "scene object definition count differs")
	}
	want := make(map[string]rpSceneObjectProjection, len(genesis.Request.Spec.Objects))
	for _, object := range genesis.Request.Spec.Objects {
		objectID, _ := core.StudioWorldObjectID(instance, "object", object.Key)
		placeID, _ := core.StudioWorldObjectID(instance, "place", object.Place)
		want[objectID] = rpSceneObjectProjection{ObjectID: objectID, ObjectKey: object.Key, DisplayName: object.Name, ObjectKind: object.Kind, PlaceID: placeID, State: object.InitialState, DefinitionEvent: definitionEvent, StateEvent: definitionEvent, LastSequence: definitionSequence}
	}
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,actor_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPSceneObjectStateChanged' AND event_sequence<=? ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var eventID, actor, raw string
		var sequence int64
		if err := rows.Scan(&eventID, &sequence, &actor, &raw); err != nil {
			return nil, err
		}
		var fact rpSceneObjectStateChanged
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return nil, err
		}
		current, ok := want[fact.ObjectID]
		if !ok || actor == "" || fact.Version != "corerp.scene-object-state.v1" || fact.ObjectKey != current.ObjectKey || fact.DisplayName != current.DisplayName || fact.ObjectKind != current.ObjectKind || fact.PlaceID != current.PlaceID || fact.PreviousState != current.State {
			return nil, core.NewError(core.CodeProjectionDiverged, "scene object state event contradicts definition")
		}
		if next, legal := sceneObjectTransition(current.ObjectKind, current.State, fact.Action); !legal || next != fact.State {
			return nil, core.NewError(core.CodeProjectionDiverged, "scene object state transition is invalid")
		}
		current.State, current.StateEvent, current.LastSequence, current.ProjectionVersion = fact.State, eventID, sequence, current.ProjectionVersion+1
		want[fact.ObjectID] = current
	}
	return want, rows.Err()
}

func rpSceneObjectProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	available, err := rpSceneObjectProjectionAvailable(ctx, q)
	if err != nil || !available {
		return nil, err
	}
	want, err := rpSceneObjectsExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT object_id,object_key,display_name,object_kind,place_id,state_code,definition_event_id,state_event_id,projection_version,last_event_sequence FROM rp_scene_objects WHERE instance_id=? AND branch_id=? ORDER BY object_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	got := map[string]rpSceneObjectProjection{}
	for rows.Next() {
		var row rpSceneObjectProjection
		if err := rows.Scan(&row.ObjectID, &row.ObjectKey, &row.DisplayName, &row.ObjectKind, &row.PlaceID, &row.State, &row.DefinitionEvent, &row.StateEvent, &row.ProjectionVersion, &row.LastSequence); err != nil {
			rows.Close()
			return nil, err
		}
		got[row.ObjectID] = row
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	for _, id := range sortedKeys(want) {
		expected := want[id]
		expectedJSON, _ := core.CanonicalJSON(expected)
		actual, ok := got[id]
		actualText := "missing"
		if ok {
			actualJSON, _ := core.CanonicalJSON(actual)
			actualText = string(actualJSON)
		}
		if actualText != string(expectedJSON) {
			projection := "rp_scene_objects"
			if ok && !sceneObjectDefinitionEqual(expected, actual) {
				projection = "rp_scene_objects_history"
			}
			differences = append(differences, ProjectionDifference{Projection: projection, Key: id, ExpectedText: string(expectedJSON), ActualText: actualText})
		}
	}
	for _, id := range sortedKeys(got) {
		if _, ok := want[id]; !ok {
			differences = append(differences, ProjectionDifference{Projection: "rp_scene_objects_extra", Key: id, ExpectedText: "absent", ActualText: "unsourced projection row"})
		}
	}
	return differences, nil
}

func rpSceneObjectProjectionAvailable(ctx context.Context, q replayQuerier) (bool, error) {
	var tableCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rp_scene_objects'`).Scan(&tableCount); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "inspect scene-object projection", err)
	}
	if tableCount != 0 {
		return true, nil
	}
	var versionCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, RPSceneObjectSchemaVersion).Scan(&versionCount); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "inspect scene-object schema version", err)
	}
	if versionCount != 0 {
		return false, core.NewError(core.CodeStorageFailure, "scene-object projection is missing after migration")
	}
	return false, nil
}

func sceneObjectDefinitionEqual(a, b rpSceneObjectProjection) bool {
	return a.ObjectID == b.ObjectID && a.ObjectKey == b.ObjectKey && a.DisplayName == b.DisplayName && a.ObjectKind == b.ObjectKind && a.PlaceID == b.PlaceID && a.DefinitionEvent == b.DefinitionEvent
}

func repairRPSceneObjectProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, difference := range differences {
		if difference.Projection != "rp_scene_objects" {
			return core.NewError(core.CodeProjectionDiverged, fmt.Sprintf("cannot repair %s for scene object %s", difference.Projection, difference.Key))
		}
		var want rpSceneObjectProjection
		if err := json.Unmarshal([]byte(difference.ExpectedText), &want); err != nil {
			return err
		}
		if difference.ActualText == "missing" {
			if err := execAgentOne(ctx, conn, "repair scene object projection", `INSERT INTO rp_scene_objects(object_id,instance_id,branch_id,object_key,display_name,object_kind,place_id,state_code,definition_event_id,state_event_id,projection_version,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, want.ObjectID, instance, branch, want.ObjectKey, want.DisplayName, want.ObjectKind, want.PlaceID, want.State, want.DefinitionEvent, want.StateEvent, want.ProjectionVersion, want.LastSequence); err != nil {
				return err
			}
			continue
		}
		var got rpSceneObjectProjection
		if err := json.Unmarshal([]byte(difference.ActualText), &got); err != nil {
			return err
		}
		if !sceneObjectDefinitionEqual(want, got) {
			return core.NewError(core.CodeProjectionDiverged, "cannot rewrite scene object definition")
		}
		if err := execAgentOne(ctx, conn, "repair scene object state", `UPDATE rp_scene_objects SET state_code=?,state_event_id=?,projection_version=?,last_event_sequence=? WHERE object_id=? AND instance_id=? AND branch_id=?`, want.State, want.StateEvent, want.ProjectionVersion, want.LastSequence, want.ObjectID, instance, branch); err != nil {
			return err
		}
	}
	return nil
}
