package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPSceneObjectContinuityNarrativeRestartAndRebuild(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "scene-object.db")
	s := openBootstrappedStore(t, ctx, path)
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "scene-object-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	const world = "scene-object-world"
	create := studioCreateFixture(world)
	create.Spec.Objects = []core.StudioWorldObject{
		{Key: "front_door", Name: "前门", Place: "home", Kind: "door", InitialState: "closed"},
		{Key: "square_lamp", Name: "广场灯", Place: "square", Kind: "light", InitialState: "off"},
	}
	if _, err := s.CreateStudioWorld(ctx, create); err != nil {
		t.Fatal(err)
	}
	player, _ := core.StudioWorldObjectID(world, "entity", "lin")
	door, _ := core.StudioWorldObjectID(world, "object", "front_door")
	lamp, _ := core.StudioWorldObjectID(world, "object", "square_lamp")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: "scene-object-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.SceneObjects) != 1 || view.SceneObjects[0].ObjectID != door || view.SceneObjects[0].State != "closed" || len(view.SceneObjects[0].Actions) != 1 || view.SceneObjects[0].Actions[0] != "open" {
		t.Fatalf("wrong visible object snapshot: %+v", view.SceneObjects)
	}
	if _, err := s.InteractRPSceneObject(ctx, core.RPSceneObjectActionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "remote-lamp", ObjectID: lamp, Action: "switch_on"}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("remote object was actionable: %v", err)
	}
	actionRequest := core.RPSceneObjectActionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "open-door", ObjectID: door, Action: "open"}
	action, err := s.InteractRPSceneObject(ctx, actionRequest)
	if err != nil || action.PreviousState != "closed" || action.Object.State != "open" || action.EventID == "" {
		t.Fatalf("open door: %+v %v", action, err)
	}
	replayed, err := s.InteractRPSceneObject(ctx, actionRequest)
	if err != nil || !replayed.Replayed || replayed.EventID != action.EventID {
		t.Fatalf("object action replay: %+v %v", replayed, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.SceneObjects) != 1 || view.SceneObjects[0].State != "open" || view.SceneObjects[0].Actions[0] != "close" {
		t.Fatalf("open object observation: %+v %v", view.SceneObjects, err)
	}
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "door-continuity-turn", Text: "看看刚才的门。"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(turn.NarrativeLines, "\n")
	if !strings.Contains(joined, "前门") || !strings.Contains(joined, "open") {
		t.Fatalf("object fact missing from narrative: %s", joined)
	}
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("object projection before restart: %v %v", diffs, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.SceneObjects) != 1 || view.SceneObjects[0].State != "open" {
		t.Fatalf("object state after restart: %+v %v", view.SceneObjects, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_scene_objects SET state_code='closed',state_event_id=definition_event_id,projection_version=0,last_event_sequence=(SELECT event_sequence FROM events WHERE event_id=definition_event_id) WHERE object_id=?`, door); err != nil {
		t.Fatal(err)
	}
	diffs, err := s.CompareProjections(ctx, world, "br_main")
	if err != nil || len(diffs) == 0 {
		t.Fatalf("object damage was not detected: %v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("object rebuild: %v %v", diffs, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_scene_objects WHERE object_id=? AND state_code='open' AND state_event_id=?`, []any{door, action.EventID}, 1)
}
