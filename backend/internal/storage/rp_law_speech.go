package storage

import (
	"encoding/json"

	"corerp.local/backend/internal/core"
)

// Only typed, actually spoken domain events supply violation evidence. Private
// definitions, proposals, filings and beliefs are not speech merely because
// their payload contains text or an actor.
func lawSpeechEvidence(eventType, raw string) (rpSpeechEvent, error) {
	var speech rpSpeechEvent
	switch eventType {
	case "RPSpeechAccepted":
		if err := json.Unmarshal([]byte(raw), &speech); err != nil {
			return speech, err
		}
	case "RPCultureFactRecorded":
		var fact CultureFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return speech, err
		}
		if fact.Kind != "transmission" {
			return speech, core.NewError(core.CodeNotFound, "event is not spoken culture transmission")
		}
		speech.SpeakerEntityID, speech.PlaceID, speech.ListenerIDs = fact.ActorID, fact.PlaceID, fact.ListenerIDs
	case "RPCareerFactRecorded":
		var fact CareerFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return speech, err
		}
		if fact.Kind != "announcement" || fact.Announcement == nil {
			return speech, core.NewError(core.CodeNotFound, "event is not spoken career announcement")
		}
		a := fact.Announcement
		speech.SpeakerEntityID, speech.PlaceID, speech.ListenerIDs = a.SpeakerID, a.PlaceID, a.ListenerIDs
	case "RPInstitutionFactRecorded":
		var fact InstitutionFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return speech, err
		}
		if fact.Kind != "law_announcement" || fact.Announcement == nil {
			return speech, core.NewError(core.CodeNotFound, "event is not spoken law announcement")
		}
		a := fact.Announcement
		speech.SpeakerEntityID, speech.PlaceID, speech.ListenerIDs = a.SpeakerID, a.PlaceID, a.ListenerIDs
	default:
		return speech, core.NewError(core.CodeNotFound, "event is not accepted speech")
	}
	if speech.SpeakerEntityID == "" || speech.PlaceID == "" {
		return speech, core.NewError(core.CodeProjectionDiverged, "speech event lacks actor/place")
	}
	return speech, nil
}
