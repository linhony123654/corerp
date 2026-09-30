package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestStudioRPReadinessDoesNotInferCanonOrLeakPersona(t *testing.T) {
	spec := StudioWorldSpec{People: []StudioWorldPerson{{Key: "player", Name: "宝玉", Player: true}, {Key: "elder", Name: "贾母"}}}
	before, _ := json.Marshal(spec)
	result := spec.RPConfigurationReadiness()
	if result.Status != "INCOMPLETE" || len(result.Characters) != 1 || result.Characters[0].Readiness != (RPContextReadiness{Persona: "MISSING", RelationshipToInterlocutor: "UNKNOWN", AddressToInterlocutor: "UNKNOWN"}) {
		t.Fatalf("missing named-character data was filled from prior: %+v", result)
	}
	after, _ := json.Marshal(spec)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("readiness changed the declaration")
	}
	spec.People[1].Persona = "私有人设标记：只对本角色可用"
	result = spec.RPConfigurationReadiness()
	encoded, _ := json.Marshal(result)
	if result.Status != "READY" || result.Characters[0].Readiness.RelationshipToInterlocutor != "UNKNOWN" || result.Characters[0].PublicPresentation != "MISSING" || strings.Contains(string(encoded), "私有人设标记") {
		t.Fatalf("unknown relationship was made false/required or private text leaked: %s", encoded)
	}
	spec.People[1].Persona = " \n\t"
	if result := spec.RPConfigurationReadiness(); result.Status != "INCOMPLETE" || result.Characters[0].Readiness.Persona != "MISSING" {
		t.Fatal("whitespace became a persona", result)
	}
}

func TestStudioRPReadinessPreservesRelationshipDirectionsAndMissingAddresses(t *testing.T) {
	spec := StudioWorldSpec{People: []StudioWorldPerson{{Key: "player", Name: "Lin", Player: true}, {Key: "nora", Name: "Nora", Persona: "温和的旧友", PublicPresentation: "亲切的措辞"}}, Acquaintances: [][2]string{{"player", "nora"}}}
	// A player's declaration cannot silently create the reverse role/address.
	spec.Relationships = []StudioWorldRelationship{{From: "player", To: "nora", Role: "朋友", AddressTo: []string{"Nora"}}}
	result := spec.RPConfigurationReadiness()
	if result.Status != "READY" || result.Characters[0].Readiness.RelationshipToInterlocutor != "UNKNOWN" || result.Characters[0].Readiness.AddressToInterlocutor != "UNKNOWN" {
		t.Fatal("reverse relationship was inferred", result)
	}
	spec.Relationships = append(spec.Relationships, StudioWorldRelationship{From: "nora", To: "player", Role: "朋友"})
	result = spec.RPConfigurationReadiness()
	if result.Status != "INCOMPLETE" || result.Characters[0].Readiness.RelationshipToInterlocutor != "READY" || result.Characters[0].Readiness.AddressToInterlocutor != "MISSING" || !reflect.DeepEqual(result.IncompleteRelationships, []StudioRPRelationshipGap{{From: "nora", To: "player"}}) {
		t.Fatal("declared missing address was hidden", result)
	}
	spec.Relationships[1].AddressTo = []string{"Lin"}
	result = spec.RPConfigurationReadiness()
	if result.Status != "READY" || result.Characters[0].Readiness != (RPContextReadiness{Persona: "READY", RelationshipToInterlocutor: "READY", AddressToInterlocutor: "READY"}) || result.Characters[0].PublicPresentation != "READY" {
		t.Fatal("complete declaration was not ready", result)
	}
	// A missing address in the other direction is still a configuration gap.
	spec.Relationships[0].AddressTo = nil
	if result := spec.RPConfigurationReadiness(); result.Status != "INCOMPLETE" || !reflect.DeepEqual(result.IncompleteRelationships, []StudioRPRelationshipGap{{From: "player", To: "nora"}}) {
		t.Fatal("player-side address gap was ignored", result)
	}
}
