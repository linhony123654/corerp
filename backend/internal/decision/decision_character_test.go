package decision

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func decodePresentationJSON(t *testing.T, raw []byte) any {
	t.Helper()
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// Inspect the delivered layout directly. This test-only conservation assertion
// compares all original values; it is not a production decoder into flat input.
func assertDecisionCharacter(t *testing.T, got decisionCharacterPresentation, in core.RPDecisionInput) {
	t.Helper()
	raw, _ := json.Marshal(in)
	var expected map[string]json.RawMessage
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	sections := map[string]decisionCharacterSection{"current_turn": got.CurrentTurn, "canon": got.Canon, "current_snapshot": got.CurrentSnapshot, "accepted_utterances": got.AcceptedUtterances, "recorded_actions": got.RecordedActions, "past_private": got.PastPrivate, "future_plans": got.FuturePlans, "typed_domains": got.TypedDomains, "provenance": got.Provenance, "legal": got.Legal}
	values := map[string][]json.RawMessage{}
	for name, section := range sections {
		if section.Meaning == "" || section.Data == nil {
			t.Fatal("missing section contract", name)
		}
		for field, value := range section.Data {
			original := field
			if field == "own_speech" {
				original = "own_actions"
			}
			if field == "knowledge_speech" {
				original = "knowledge"
			}
			if _, ok := expected[original]; !ok {
				t.Fatal("invented data field", name, field)
			}
			values[original] = append(values[original], value)
		}
	}
	for field, value := range expected {
		if field != "own_actions" && field != "scene_activities" && field != "knowledge" {
			if len(values[field]) != 1 || !reflect.DeepEqual(decodePresentationJSON(t, value), decodePresentationJSON(t, values[field][0])) {
				t.Fatal("field lost/duplicated/rewritten", field)
			}
			continue
		}
		original := decodePresentationJSON(t, value)
		entries, nonemptyArray := original.([]any)
		if !nonemptyArray || len(entries) == 0 {
			if len(values[field]) != 1 || !reflect.DeepEqual(original, decodePresentationJSON(t, values[field][0])) {
				t.Fatal("null/empty array changed", field)
			}
			continue
		}
		// Consume each input element exactly once, preserving multiplicity even
		// when identical accepted words have several original source entries.
		remaining := append([]any{}, entries...)
		for _, part := range values[field] {
			for _, entry := range decodePresentationJSON(t, part).([]any) {
				match := -1
				for i, old := range remaining {
					if reflect.DeepEqual(old, entry) {
						match = i
						break
					}
				}
				if match < 0 {
					t.Fatal("extra or rewritten array member", field)
				}
				remaining = append(remaining[:match], remaining[match+1:]...)
			}
		}
		if len(remaining) != 0 {
			t.Fatal("dropped array values", field, len(remaining))
		}
	}
	for field, section := range map[string]decisionCharacterSection{"player_speech_text": got.CurrentTurn, "npc_entity_id": got.Canon, "place_id": got.CurrentSnapshot, "context_version": got.Provenance, "legal_actions": got.Legal} {
		if _, present := expected[field]; present {
			if _, ok := section.Data[field]; !ok {
				t.Fatal("field assigned wrong authority section", field)
			}
		}
	}
}

func presentedValue(t *testing.T, section decisionCharacterSection, field string) any {
	t.Helper()
	raw, ok := section.Data[field]
	if !ok {
		t.Fatal("missing presented field", field)
	}
	return decodePresentationJSON(t, raw)
}

func TestDecisionCharacterPartitionsEvidenceWithoutChangingValuesOrInput(t *testing.T) {
	in := contextFixture()
	in.NPCEntityID = "self_alias"
	in.InterlocutorEntityID = "peer_alias"
	in.WorldTime = "same-time"
	in.PlayerSpeechText = "我还在原处。"
	in.Persona = "温和的长辈。"
	in.PersonaSourceEventID = "identity"
	in.Relationships = []core.RPCharacterRelationship{{SubjectEntityID: "peer_alias", SourceEventID: "identity", Role: "祖母", AddressTo: []string{"玉儿"}}}
	in.OwnActions = []core.RPOwnAction{{EventID: "speech-a", Action: "speech", Text: "坐在这里。", WorldTime: in.WorldTime}, {EventID: "action-b", Action: "activity", ActivityCode: "read", Status: "completed", WorldTime: in.WorldTime}, {EventID: "speech-c", Action: "speech", TextFromEvent: true, WorldTime: in.WorldTime}, {EventID: "action-d", Action: "nonverbal", ActivityCode: "nod", WorldTime: in.WorldTime}}
	in.SceneActivities = []core.RPSceneActivity{{SourceEventID: "past-activity", ActorID: in.NPCEntityID, ObservationBasis: "own_action", Status: "completed", WorldTime: in.WorldTime}, {SourceEventID: "current-activity", ActorID: "peer_alias", ObservationBasis: "current_visibility", Status: "in_progress", WorldTime: in.WorldTime}, {SourceEventID: "future-domain", ObservationBasis: "new_unknown_kind", Status: "unknown"}}
	in.Knowledge = []core.RPDecisionKnowledge{{SourceEventID: "presence", ClaimType: "agent_presence", SubjectEntityID: "peer_alias", PlaceID: in.PlaceID}, {SourceEventID: "speech-a", ClaimType: "speaker_said", SubjectEntityID: in.NPCEntityID, TextFromEvent: true}, {SourceEventID: "future-domain", ClaimType: "new_unknown_claim", Text: "原型字段保留"}}
	in.RecentDialogue = []core.RPDecisionDialogue{{EventID: "speech-a", SpeakerEntityID: in.NPCEntityID, Text: "坐在这里。", WorldTime: in.WorldTime}}
	in.HeardPlayerHistory = []core.RPDecisionSpeechExcerpt{{EventID: "excerpt", Excerpt: "我会……", Truncated: true, WorldTime: in.WorldTime}}
	in.RelevantDialogue = []core.RPDecisionExchange{{PeerContext: true, Dialogue: []core.RPDecisionDialogue{{EventID: "promise", SpeakerEntityID: in.InterlocutorEntityID, Text: "明天我会读书。", WorldTime: in.WorldTime}}}}
	in.RecentPrivateDecisions = []core.RPDecisionPrivateMemory{{SourceEventID: "private", InterlocutorEntityID: "other_alias", WorldTime: "earlier", Private: core.RPDecisionPrivate{Intent: "private-token陪他坐着", BasisEventIDs: []string{"ancestral"}}}}
	in.NextSchedule = &core.RPDecisionSchedule{SourceEventID: "plan", WorldTime: "tomorrow", ActivityCode: "work"}
	in.SceneObjects = []core.RPDecisionSceneObject{{ObjectID: "object_alias", DisplayName: "侧门", Kind: "door", State: "closed", PlaceID: in.PlaceID, WorldTime: in.WorldTime, DefinitionSourceEventID: "object-definition", StateSourceEventID: "object-state"}}
	before, _ := core.HashJSON(in)
	schema, err := decisionResponseSchema(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := buildDecisionContext(in, schema)
	if err != nil {
		t.Fatal(err)
	}
	assertDecisionCharacter(t, got.Character, in)
	if got.CharacterLayoutVersion != decisionCharacterLayoutVersion || got.Version != "corerp.decision.v3" {
		t.Fatal("layout or reply version changed")
	}
	if presentedValue(t, got.Character.CurrentTurn, "player_speech_text") != "我还在原处。" {
		t.Fatal("speaker report rewritten into pose")
	}
	for _, tc := range []struct {
		section decisionCharacterSection
		field   string
		ids     []string
	}{
		{got.Character.AcceptedUtterances, "own_speech", []string{"speech-a", "speech-c"}},
		{got.Character.RecordedActions, "own_actions", []string{"action-b", "action-d"}},
		{got.Character.CurrentSnapshot, "scene_activities", []string{"current-activity"}},
		{got.Character.RecordedActions, "scene_activities", []string{"past-activity"}},
		{got.Character.TypedDomains, "scene_activities", []string{"future-domain"}},
	} {
		entries := presentedValue(t, tc.section, tc.field).([]any)
		if len(entries) != len(tc.ids) {
			t.Fatal("wrong partition size", tc.field)
		}
		for i, id := range tc.ids {
			entry := entries[i].(map[string]any)
			actual := entry["event_id"]
			if actual == nil {
				actual = entry["source_event_id"]
			}
			if actual != id {
				t.Fatal("partition changed order", tc.field, actual, id)
			}
		}
	}
	if objects := presentedValue(t, got.Character.CurrentSnapshot, "scene_objects").([]any); len(objects) != 1 || objects[0].(map[string]any)["state"] != "closed" {
		t.Fatal("listed object snapshot lost")
	}
	if _, exists := got.Character.RecordedActions.Data["own_speech"]; exists {
		t.Fatal("speech in action records")
	}
	if !strings.Contains(got.Character.CurrentSnapshot.Meaning, "unprovided does not mean nonexistent") {
		t.Fatal("absence converted into negative fact")
	}
	snapshot, _ := json.Marshal(got.Character.CurrentSnapshot.Data)
	if strings.Contains(string(snapshot), "private-token") || strings.Contains(string(snapshot), "坐在这里") {
		t.Fatal("history promoted to snapshot")
	}
	metadata, _ := json.Marshal(got.GroundingSources)
	if strings.Contains(string(metadata), "private-token") {
		t.Fatal("metadata copied private text")
	}
	verifyDecisionSupportPaths(t, in, got)
	after, _ := core.HashJSON(in)
	again, err := buildDecisionContext(in, schema)
	first, _ := json.Marshal(got)
	second, _ := json.Marshal(again)
	if err != nil || before != after || !bytes.Equal(first, second) {
		t.Fatal("presentation mutated input or is unstable")
	}
}

func jsonPointerValue(t *testing.T, root any, pointer string) any {
	t.Helper()
	value := root
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch typed := value.(type) {
		case map[string]any:
			var found bool
			value, found = typed[part]
			if !found {
				t.Fatal("dangling pointer", pointer)
			}
		case []any:
			var index int
			if _, err := fmt.Sscan(part, &index); err != nil || index < 0 || index >= len(typed) {
				t.Fatal("bad array pointer", pointer)
			}
			value = typed[index]
		default:
			t.Fatal("pointer traverses scalar", pointer)
		}
	}
	return value
}

func verifyDecisionSupportPaths(t *testing.T, input core.RPDecisionInput, packet decisionContext) {
	t.Helper()
	oldRaw, _ := json.Marshal(input)
	newRaw, _ := json.Marshal(packet.Character)
	oldRoot := decodePresentationJSON(t, oldRaw)
	newRoot := decodePresentationJSON(t, newRaw)
	original := decisionSourceRefs(input)
	if len(original) != len(packet.GroundingSources) {
		t.Fatal("source set changed")
	}
	for i, source := range original {
		actual := packet.GroundingSources[i]
		if source.Ref != actual.Ref || source.SourceEventID != actual.SourceEventID || len(source.SupportRanges) != len(actual.SupportRanges) {
			t.Fatal("handle or range lost")
		}
		for j, r := range source.SupportRanges {
			moved := expandDecisionSupportRange(t, packet, actual, j)
			if !reflect.DeepEqual(jsonPointerValue(t, oldRoot, r.Locator), jsonPointerValue(t, newRoot, moved.Locator)) {
				t.Fatal("range points to a different value", r.Locator, moved.Locator)
			}
			moved.Locator = r.Locator
			if !reflect.DeepEqual(moved, r) {
				t.Fatal("range semantics changed during move")
			}
		}
	}
}

func decisionCharacterMatches(t *testing.T, got decisionCharacterPresentation, input core.RPDecisionInput) bool {
	t.Helper()
	assertDecisionCharacter(t, got, input)
	return true
}

func TestDecisionCharacterConservesEveryPresentInputField(t *testing.T) {
	var in core.RPDecisionInput
	// Exercise every currently declared field, including nested domain values.
	// Sentinels are not a valid world; this tests a lossless adapter projection.
	var fill func(reflect.Value, string)
	fill = func(v reflect.Value, path string) {
		switch v.Kind() {
		case reflect.Pointer:
			v.Set(reflect.New(v.Type().Elem()))
			fill(v.Elem(), path)
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Field(i).CanSet() {
					fill(v.Field(i), path+"_"+v.Type().Field(i).Name)
				}
			}
		case reflect.String:
			v.SetString("fixture" + path)
		case reflect.Bool:
			v.SetBool(true)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v.SetInt(9007199254740993)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			v.SetUint(7)
		case reflect.Float32, reflect.Float64:
			v.SetFloat(0.25)
		case reflect.Slice:
			v.Set(reflect.MakeSlice(v.Type(), 1, 1))
			fill(v.Index(0), path+"_0")
		}
	}
	fill(reflect.ValueOf(&in).Elem(), "")
	before, _ := core.HashJSON(in)
	packet, err := buildDecisionContext(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertDecisionCharacter(t, packet.Character, in)
	verifyDecisionSupportPaths(t, in, packet)
	after, _ := core.HashJSON(in)
	if before != after {
		t.Fatal("adapter mutated provider input")
	}
}

// Only tests decode the wire; production never treats model-returned metadata
// as authority or reconstructs an alternative world/input.
func expandDecisionSupportRange(t *testing.T, packet decisionContext, source decisionWireSourceRef, index int) core.RPEvidenceSupportRange {
	t.Helper()
	fields := map[string]json.RawMessage{}
	for key, value := range source.SupportDefaults {
		fields[key] = value
	}
	var local map[string]json.RawMessage
	if err := json.Unmarshal(source.SupportRanges[index], &local); err != nil {
		t.Fatal(err)
	}
	for key, value := range local {
		fields[key] = value
	}
	for ref, table := range map[string]map[string]map[string]json.RawMessage{"profile_ref": packet.SupportProfiles, "scope_ref": packet.SupportScopes} {
		if value, ok := fields[ref]; ok {
			var id string
			if err := json.Unmarshal(value, &id); err != nil {
				t.Fatal(err)
			}
			entry, exists := table[id]
			if !exists {
				t.Fatal("dangling dictionary reference", ref, id)
			}
			delete(fields, ref)
			for key, value := range entry {
				if _, duplicate := fields[key]; duplicate {
					t.Fatal("ambiguous dictionary override", key)
				}
				fields[key] = value
			}
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var expanded core.RPEvidenceSupportRange
	if err = json.Unmarshal(raw, &expanded); err != nil {
		t.Fatal(err)
	}
	return expanded
}

func TestDecisionSupportV2DefaultsAndDictionariesAreExactlyLossless(t *testing.T) {
	shared := core.RPEvidenceSupportRange{Locator: "/first", Kind: "accepted_speech", ActorEntityID: "self_alias", TargetEntityID: "peer_alias", WorldTime: "then", Completeness: "complete", Status: "said", AllowedUses: []string{"attributed_speech", "exact_quote"}}
	second := shared
	second.Locator = "/second"
	absent := shared
	absent.Locator = "/absent"
	absent.ActorEntityID = ""
	absent.WorldTime = ""
	different := shared
	different.Locator = "/different"
	different.Kind = "own_private"
	different.AllowedUses = []string{"private_continuity"}
	sources := []decisionSourceRef{{Ref: "src_1", SourceEventID: "single", SupportRanges: []core.RPEvidenceSupportRange{shared}}, {Ref: "src_2", SourceEventID: "same", SupportRanges: []core.RPEvidenceSupportRange{shared, second}}, {Ref: "src_3", SourceEventID: "mixed", SupportRanges: []core.RPEvidenceSupportRange{shared, absent, different}}}
	compact, err := compactDecisionSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	singleRaw, _ := json.Marshal(shared)
	if compact[0].SupportDefaults != nil || !bytes.Equal(compact[0].SupportRanges[0], singleRaw) {
		t.Fatal("single range changed")
	}
	if _, sharedLocator := compact[1].SupportDefaults["locator"]; sharedLocator {
		t.Fatal("locator was shared")
	}
	if len(compact[1].SupportDefaults) != 7 {
		t.Fatal("identical present fields not shared", compact[1].SupportDefaults)
	}
	for _, key := range []string{"actor_entity_id", "world_time", "kind", "allowed_uses"} {
		if _, shared := compact[2].SupportDefaults[key]; shared {
			t.Fatal("absent/different metadata inferred", key)
		}
	}
	dict, profiles, scopes, err := dictionaryDecisionSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	for _, packet := range []decisionContext{{GroundingSources: compact}, {GroundingSources: dict, SupportProfiles: profiles, SupportScopes: scopes}} {
		raw, _ := json.Marshal(packet)
		var roundtrip decisionContext
		if err := json.Unmarshal(raw, &roundtrip); err != nil {
			t.Fatal(err)
		}
		for i, source := range sources {
			wire := roundtrip.GroundingSources[i]
			if wire.Ref != source.Ref || wire.SourceEventID != source.SourceEventID || len(wire.SupportRanges) != len(source.SupportRanges) {
				t.Fatal("source/range identity lost")
			}
			for j, r := range source.SupportRanges {
				if !reflect.DeepEqual(expandDecisionSupportRange(t, roundtrip, wire, j), r) {
					t.Fatal("encoding changed metadata", i, j)
				}
			}
		}
	}
	again, p2, s2, err := dictionaryDecisionSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(decisionContext{GroundingSources: dict, SupportProfiles: profiles, SupportScopes: scopes})
	secondRaw, _ := json.Marshal(decisionContext{GroundingSources: again, SupportProfiles: p2, SupportScopes: s2})
	if !bytes.Equal(first, secondRaw) {
		t.Fatal("dictionary IDs depend on map iteration")
	}
}
