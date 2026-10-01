package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPKnownNameQuestionCommitsThroughExistingOwnerWithoutNewFamiliarity(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "known-name.db"))
	defer s.Close()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "known-name-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	const world = "known-name-world"
	create := studioCreateFixture(world)
	create.Spec.People[1].Persona = "Speaks plainly; answers Lin's questions directly."
	create.Spec.Acquaintances = [][2]string{{"lin", "cai"}}
	create.Spec.Relationships = []core.StudioWorldRelationship{{From: "cai", To: "lin", Role: "friend", AddressTo: []string{"Lin"}, SelfReference: "Cai"}}
	if _, err := s.CreateStudioWorld(ctx, create); err != nil {
		t.Fatal(err)
	}
	player, _ := core.StudioWorldObjectID(world, "entity", "lin")
	npc, _ := core.StudioWorldObjectID(world, "entity", "cai")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: "known-name-session"})
	if err != nil {
		t.Fatal(err)
	}
	for index, disclose := range []bool{false, true} {
		view, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID})
		if err != nil {
			t.Fatal(err)
		}
		speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: fmt.Sprintf("name-question-%d", index), Text: "您叫什么？"})
		if err != nil {
			t.Fatal(err)
		}
		r := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: npc}
		candidate, err := s.DecideRP(ctx, r, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
			if in.Readiness.Incomplete() || in.Readiness.RelationshipToInterlocutor != "READY" || in.PlayerSpeechText != "您叫什么？" {
				t.Fatalf("test did not reach a declared known relationship: %#v", in.Readiness)
			}
			return core.RPDecisionProposal{Action: "respond", Text: "我是Cai。", IntroduceSelf: disclose,
				Private: &core.RPDecisionPrivate{Intent: "answer the name question", BasisEventIDs: []string{in.SpeechEventID}}}, nil
		}))
		if err != nil || candidate.Status != "validated" {
			t.Fatalf("known-name answer was rejected: %#v %v", candidate, err)
		}
		committed, err := s.CommitRPDecision(ctx, r, candidate)
		if err != nil || committed.Action != "respond" || committed.EventSequence != speech.EventSequence+1 {
			t.Fatalf("name answer did not use the normal owner: %#v %v", committed, err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE event_id=? AND speech_text='我是Cai。' AND speaker_entity_id=?`, []any{committed.EventID, npc}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE instance_id=? AND branch_id='br_main' AND observer_agent_id=? AND subject_agent_id=?`, []any{world, player, npc}, 1)
		retry, err := s.CommitRPDecision(ctx, r, candidate)
		if err != nil || !retry.Replayed || retry.EventID != committed.EventID {
			t.Fatalf("name answer duplicated on retry: %#v %v", retry, err)
		}
		assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{world}, committed.EventSequence)
	}
}
