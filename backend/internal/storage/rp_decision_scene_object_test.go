package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPDecisionSceneObjectsRealWorldSnapshotAndProviderBoundary(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "decision-objects.db"))
	defer s.Close()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "decision-object-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	const world = "decision-object-world"
	create := studioCreateFixture(world)
	create.Spec.Objects = []core.StudioWorldObject{{Key: "door", Name: "前门", Place: "home", Kind: "door", InitialState: "closed"}, {Key: "lamp", Name: "远处灯", Place: "square", Kind: "light", InitialState: "off"}}
	if _, err := s.CreateStudioWorld(ctx, create); err != nil {
		t.Fatal(err)
	}
	id := func(kind, key string) string {
		v, err := core.StudioWorldObjectID(world, kind, key)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	player, npc, door := id("entity", "lin"), id("entity", "cai"), id("object", "door")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: "objects-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "objects-speech", Text: "我在原处。"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: speech.TurnID, NPCEntityID: npc}
	input, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.SceneObjects) != 1 {
		t.Fatalf("local object boundary: %+v", input.SceneObjects)
	}
	object := input.SceneObjects[0]
	if object.ObjectID != door || object.Kind != "door" || object.State != "closed" || object.DisplayName != "前门" || object.PlaceID != input.PlaceID || object.WorldTime != input.WorldTime || object.DefinitionSourceEventID == "" || object.StateSourceEventID != object.DefinitionSourceEventID {
		t.Fatalf("unsourced current object: %+v", object)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='StudioSpatialPrepared' AND event_sequence<=?`, []any{object.DefinitionSourceEventID, world, "br_main", input.HeadSequence}, 1)
	projected, err := s.rpDecisionProviderView(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if projected.InterlocutorEntityID == player || len(projected.SceneObjects) != 1 || projected.SceneObjects[0] != object || projected.PlayerSpeechText != "我在原处。" {
		t.Fatalf("provider lost object sources or stranger masking: %+v", projected)
	}
	// No storage field turns this attributed sentence into a body pose,
	// seat occupancy, or NPC object-operation authority.
	raw, err := json.Marshal(projected.SceneObjects)
	if err != nil {
		t.Fatal(err)
	}
	for _, invented := range []string{`"actions"`, `"posture"`, `"occupancy"`, `"seat"`} {
		if strings.Contains(string(raw), invented) {
			t.Fatalf("invented object authority: %s", raw)
		}
	}
	for _, action := range projected.LegalActions {
		if action == "open" || action == "close" || action == "sit" || action == "stand" {
			t.Fatalf("object observation granted action %q", action)
		}
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.InteractRPSceneObject(ctx, core.RPSceneObjectActionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "objects-open", ObjectID: door, Action: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.rpDecisionProviderView(ctx, input); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale provider snapshot accepted: %v", err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := readRPDecisionSceneObjects(ctx, conn, input); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("later object attached to old head: %v", err)
	}
	current := input
	current.HeadSequence, current.WorldTime = changed.EventSequence, changed.WorldTime
	objects, err := readRPDecisionSceneObjects(ctx, conn, current)
	if err != nil || len(objects) != 1 || objects[0].State != "open" || objects[0].StateSourceEventID != changed.EventID || objects[0].DefinitionSourceEventID != object.DefinitionSourceEventID {
		t.Fatalf("changed object lineage: %+v %v", objects, err)
	}
	conn.Close()
	for _, damage := range []struct{ name, sql string }{
		{"state", `UPDATE rp_scene_objects SET state_code='closed' WHERE object_id=?`},
		{"state source", `UPDATE rp_scene_objects SET state_event_id=definition_event_id WHERE object_id=?`},
		{"sequence", `UPDATE rp_scene_objects SET last_event_sequence=last_event_sequence+1 WHERE object_id=?`},
		{"definition", `UPDATE rp_scene_objects SET definition_event_id=state_event_id WHERE object_id=?`},
		{"missing", `DELETE FROM rp_scene_objects WHERE object_id=?`},
	} {
		t.Run(damage.name, func(t *testing.T) {
			tx, err := beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.conn.ExecContext(ctx, damage.sql, door); err != nil {
				t.Fatal(err)
			}
			if _, err := readRPDecisionSceneObjects(ctx, tx.conn, current); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatalf("damaged object accepted: %v", err)
			}
		})
	}
	t.Run("pre061", func(t *testing.T) {
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.conn.ExecContext(ctx, `DROP TABLE rp_scene_objects`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.conn.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version=?`, RPSceneObjectSchemaVersion); err != nil {
			t.Fatal(err)
		}
		objects, err := readRPDecisionSceneObjects(ctx, tx.conn, current)
		if err != nil || len(objects) != 0 {
			t.Fatalf("pre061 compatibility: %+v %v", objects, err)
		}
	})
}
