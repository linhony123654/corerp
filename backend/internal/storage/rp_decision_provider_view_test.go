package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPDecisionProviderViewPreservesClaimsAndProvenance(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "provider-view.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _, view := newRPWaitTestSession(t, ctx, s)
	id := M2AgentAdaID
	words := "我听说 " + id + "，这只是原话。"
	input := core.RPDecisionInput{
		ContextVersion: core.RPContextVersion, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		HeadSequence: view.ObservationCursor, WorldTime: view.WorldTime, NPCEntityID: M2RPNPCID,
		InterlocutorEntityID: id, Persona: words, PlaceID: "place_" + id, CurrencyID: id,
		PersonaSourceEventID: "event_" + id, LegalActions: []string{"respond", "silence"},
		VisibleEntities:  []core.RPDecisionVisibleEntity{{EntityID: id, DisplayName: "Ada"}},
		PlayerSpeechText: words, SpeechEventID: "speech_" + id,
		RecentDialogue: []core.RPDecisionDialogue{{SpeakerEntityID: id, Text: words, EventID: "dialogue_" + id, WorldTime: view.WorldTime}},
		HeardPlayerHistory: []core.RPDecisionSpeechExcerpt{
			{EventID: "excerpt_" + id, Excerpt: words, WorldTime: view.WorldTime},
			{EventID: "partial_" + id, Excerpt: words + "…", Truncated: true, WorldTime: view.WorldTime},
		},
		RecentPrivateDecisions: []core.RPDecisionPrivateMemory{{DecisionID: "decision_" + id, SourceEventID: "private_" + id, InterlocutorEntityID: id,
			Private: core.RPDecisionPrivate{Intent: words, Emotion: id, RelationshipStance: words, BasisEventIDs: []string{"basis_" + id}}}},
		Life: &core.RPLifeContext{Goals: []core.RPGoal{{SubjectEntityID: id, ConflictsWith: []string{"social_contact:" + id}, SourceEventIDs: []string{"goal_" + id}}}},
	}
	got, err := s.rpDecisionProviderView(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.HeardPlayerHistory) != 2 || got.HeardPlayerHistory[0].Excerpt != words || got.HeardPlayerHistory[1].Excerpt != words+"…" || !got.HeardPlayerHistory[1].Truncated || got.HeardPlayerHistory[1].EventID != input.HeardPlayerHistory[1].EventID {
		t.Fatalf("heard words changed: %+v", got.HeardPlayerHistory)
	}
	if got.PlaceID != input.PlaceID || got.CurrencyID != input.CurrencyID || got.PersonaSourceEventID != input.PersonaSourceEventID || got.Persona != words {
		t.Fatal("unrelated identifiers or persona changed")
	}
	if len(got.RecentPrivateDecisions) != 1 || !reflect.DeepEqual(got.RecentPrivateDecisions[0].Private, input.RecentPrivateDecisions[0].Private) || got.RecentPrivateDecisions[0].SourceEventID != input.RecentPrivateDecisions[0].SourceEventID || got.RecentPrivateDecisions[0].DecisionID != input.RecentPrivateDecisions[0].DecisionID {
		t.Fatalf("private phrases or provenance changed: %+v", got.RecentPrivateDecisions)
	}
	if !reflect.DeepEqual(got.Life.Goals[0].ConflictsWith, input.Life.Goals[0].ConflictsWith) || !reflect.DeepEqual(got.Life.Goals[0].SourceEventIDs, input.Life.Goals[0].SourceEventIDs) {
		t.Fatal("goal phrases or provenance changed")
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, s, input.InstanceID, input.BranchID, input.NPCEntityID, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.InterlocutorEntityID != alias || got.VisibleEntities[0].EntityID != alias || got.VisibleEntities[0].DisplayName != "陌生人" || got.RecentPrivateDecisions[0].InterlocutorEntityID != alias || got.Life.Goals[0].SubjectEntityID != alias {
		t.Fatal("typed unfamiliar references were not consistently masked")
	}
	if got.PlayerSpeechText != words || got.SpeechEventID != input.SpeechEventID || got.RecentDialogue[0].SpeakerEntityID != alias {
		t.Fatal("current speech or historical speaker changed incorrectly")
	}
	if text, complete := core.ResolveRPDecisionSpeech(got, input.RecentDialogue[0].EventID, alias); !complete || text != words {
		t.Fatal("full historical speech lost its exact body/provenance")
	}
	if input.VisibleEntities[0].EntityID != id || input.VisibleEntities[0].DisplayName != "Ada" {
		t.Fatal("provider projection mutated authoritative input")
	}
	if input.Presentation != nil || got.Presentation == nil || got.Presentation.PolicyVersion != "corerp.rp-identity-view.v2" || got.Presentation.SourceHeadSequence != input.HeadSequence || got.Presentation.IdentityMode != "observer_relative" {
		t.Fatal("presentation policy missing or authoritative input mutated")
	}
	stale := input
	stale.HeadSequence--
	if _, err := s.rpDecisionProviderView(ctx, stale); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale snapshot accepted: %v", err)
	}
	// A subsequent writer proves the short snapshot was released on return.
	if _, err := s.db.ExecContext(ctx, `UPDATE branches SET head_sequence=head_sequence WHERE instance_id=? AND branch_id=?`, input.InstanceID, input.BranchID); err != nil {
		t.Fatalf("provider view retained database transaction: %v", err)
	}
	controlled := input
	controlled.NPCEntityID = M2RPPlayerID
	if _, err := s.rpDecisionProviderView(ctx, controlled); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("externally controlled actor accepted at snapshot: %v", err)
	}
	known := input
	known.InterlocutorEntityID = known.NPCEntityID
	known.VisibleEntities = []core.RPDecisionVisibleEntity{{EntityID: known.NPCEntityID, DisplayName: "self"}}
	known.RecentDialogue, known.HeardPlayerHistory, known.RecentPrivateDecisions, known.Life = nil, nil, nil, nil
	got, err = s.rpDecisionProviderView(ctx, known)
	if err != nil || got.InterlocutorEntityID != known.NPCEntityID || got.VisibleEntities[0].DisplayName != "self" || got.Presentation == nil {
		t.Fatalf("known identity or no-mask presentation changed: %+v %v", got, err)
	}
}

// Inventory the real reachable schema rather than restating the projection's
// JSON allowlist. Semantic Go field suffixes identify people independently of
// serialization names; unrelated IDs must remain outside the projection.
func TestRPDecisionProviderProjectionCoversReachablePersonSchema(t *testing.T) {
	seen := map[reflect.Type]bool{}
	personFields, otherIDFields := 0, 0
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			key := strings.Split(field.Tag.Get("json"), ",")[0]
			if field.PkgPath != "" || key == "-" {
				continue
			}
			if key == "" {
				key = field.Name
			}
			name := strings.TrimSuffix(field.Name, "s") // ID and IDs.
			if strings.HasSuffix(name, "ID") {
				// These identify provenance, locations, world scope or non-person
				// records. Exclude them before person suffix classification.
				unrelated := false
				for _, suffix := range []string{"EventID", "ObservationID", "InstanceID", "BranchID", "TurnID", "DecisionID", "SessionID", "PlaceID", "ActivityID", "CurrencyID", "CohortID", "ContractID", "PositionID", "OccupationID", "WorkplaceID", "OrganizationID", "InstitutionID", "LawID", "ScopeID", "CultureID", "NormID", "SKUID", "MessageID"} {
					unrelated = unrelated || strings.HasSuffix(name, suffix)
				}
				person := false
				if !unrelated {
					for _, suffix := range []string{"EntityID", "ActorID", "AgentID", "FriendID", "SpeakerID", "InterlocutorID", "CounterpartyID", "PersonID", "OwnerID", "HolderID", "RecipientID", "SenderID", "WitnessID", "ObserverID", "TargetID", "NPCID"} {
						person = person || strings.HasSuffix(name, suffix)
					}
				}
				if !unrelated && !person {
					t.Errorf("classify new ID semantics: %s.%s (%s)", typ.Name(), field.Name, key)
					continue
				}
				if person {
					personFields++
				} else {
					otherIDFields++
				}
				const id, alias = "schema_person", "person_schema_alias"
				value := any(id)
				if field.Type.Kind() == reflect.Slice {
					value = []any{id}
				}
				got := walkRPDecisionEntityReferences(map[string]any{key: value}, "", func(string) string { return alias }).(map[string]any)[key]
				want := value
				if person {
					want = alias
					if field.Type.Kind() == reflect.Slice {
						want = []any{alias}
					}
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s.%s (%s): projection=%v, want=%v", typ.Name(), field.Name, key, got, want)
				}
			}
			walk(field.Type)
		}
	}
	walk(reflect.TypeOf(core.RPDecisionInput{}))
	if personFields == 0 || otherIDFields == 0 {
		t.Fatal("schema inventory did not exercise both person and unrelated IDs")
	}
	t.Logf("inventoried %d reachable types, %d person-reference fields, %d unrelated ID fields", len(seen), personFields, otherIDFields)
}

func TestRPDecisionProviderProjectionMapsOnlyExactEntityReferences(t *testing.T) {
	id, alias := "agent_unknown", "person_alias"
	for _, key := range []string{"entity_id", "npc_entity_id", "interlocutor_entity_id", "subject_entity_id", "speaker_entity_id", "actor_entity_id", "target_entity_id", "actor_id", "store_actor_id", "counterparty_entity_id", "friend_id"} {
		t.Run(key, func(t *testing.T) {
			input := map[string]any{key: id, "nested": []any{map[string]any{key: "prefix_" + id}}}
			got := walkRPDecisionEntityReferences(input, "", func(value string) string {
				if value == id {
					return alias
				}
				return value
			}).(map[string]any)
			if got[key] != alias || got["nested"].([]any)[0].(map[string]any)[key] != "prefix_"+id {
				t.Fatal(got)
			}
		})
	}
	for _, key := range []string{"text", "excerpt", "player_speech_text", "persona", "intent", "emotion", "relationship_stance", "source_event_id", "event_id", "decision_id", "basis_event_ids", "source_event_ids", "place_id", "contract_id", "message_id", "sender_handle", "conflicts_with"} {
		input := map[string]any{key: id}
		got := walkRPDecisionEntityReferences(input, "", func(string) string { t.Fatalf("unrelated field %s projected", key); return alias }).(map[string]any)
		if got[key] != id {
			t.Fatalf("%s changed", key)
		}
	}
}
