package core

import "strings"

// StudioRPReadiness describes the completeness of a validated creator
// declaration, not the quality of generated RP or the live world's state.
// It contains no persona text and never supplies missing canon.
type StudioRPReadiness struct {
	Status                  string                       `json:"status"`
	Characters              []StudioRPCharacterReadiness `json:"characters"`
	IncompleteRelationships []StudioRPRelationshipGap    `json:"incomplete_relationships"`
}

type StudioRPCharacterReadiness struct {
	Key                string             `json:"key"`
	Name               string             `json:"name"`
	Readiness          RPContextReadiness `json:"readiness"`
	PublicPresentation string             `json:"public_presentation"`
}

type StudioRPRelationshipGap struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// RPConfigurationReadiness is derived from the immutable creation request.
// An unknown relationship is valid; an explicitly declared relationship
// without an address is incomplete. No reciprocal role is inferred.
func (s StudioWorldSpec) RPConfigurationReadiness() StudioRPReadiness {
	out := StudioRPReadiness{Status: "READY", Characters: []StudioRPCharacterReadiness{}, IncompleteRelationships: []StudioRPRelationshipGap{}}
	player := ""
	for _, person := range s.People {
		if person.Player {
			player = person.Key
		}
	}
	for _, person := range s.People {
		if person.Player {
			continue // A human-controlled role does not require an NPC persona.
		}
		character := StudioRPCharacterReadiness{Key: person.Key, Name: person.Name,
			Readiness: RPContextReadiness{Persona: "MISSING", RelationshipToInterlocutor: "UNKNOWN", AddressToInterlocutor: "UNKNOWN"}, PublicPresentation: "MISSING"}
		if strings.TrimSpace(person.Persona) != "" {
			character.Readiness.Persona = "READY"
		} else {
			out.Status = "INCOMPLETE"
		}
		if strings.TrimSpace(person.PublicPresentation) != "" {
			character.PublicPresentation = "READY"
		}
		for _, relation := range s.Relationships {
			if relation.From == person.Key && relation.To == player {
				character.Readiness.RelationshipToInterlocutor = "READY"
				character.Readiness.AddressToInterlocutor = "MISSING"
				if len(relation.AddressTo) > 0 {
					character.Readiness.AddressToInterlocutor = "READY"
				}
			}
		}
		out.Characters = append(out.Characters, character)
	}
	for _, relation := range s.Relationships {
		if len(relation.AddressTo) == 0 {
			out.Status = "INCOMPLETE"
			out.IncompleteRelationships = append(out.IncompleteRelationships, StudioRPRelationshipGap{From: relation.From, To: relation.To})
		}
	}
	return out
}
