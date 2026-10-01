package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

// Delivery is an observable chosen by the speaker. A listener must have the
// same frozen delivery in their own hearing; private emotion is never read.
func recordedRPSpeechTone(sourceJSON, hearingJSON, speakerID, observerID string) (string, error) {
	var speech rpSpeechEvent
	if json.Unmarshal([]byte(sourceJSON), &speech) != nil || speech.SpeakerEntityID != speakerID || !core.ValidRPSpeechTone(speech.SpeechTone) {
		return "", narrativeDiverged("speech delivery lacks accepted speaker authority")
	}
	if speakerID == observerID {
		return speech.SpeechTone, nil
	}
	var hearing rpSpeechClaim
	if json.Unmarshal([]byte(hearingJSON), &hearing) != nil || hearing.ClaimType != "speaker_said" || hearing.SpeakerEntityID != speakerID || hearing.UtteranceID != speech.UtteranceID || hearing.Text != speech.Text || hearing.SpeechAct != speech.SpeechAct || hearing.SpeechTone != speech.SpeechTone {
		return "", narrativeDiverged("speech delivery differs from the observer's frozen hearing")
	}
	heard := false
	for _, id := range speech.ListenerIDs {
		heard = heard || id == observerID
	}
	if !heard {
		return "", narrativeDiverged("speech delivery is outside the accepted listener set")
	}
	return hearing.SpeechTone, nil
}

// Existing law/career/culture owners can publish attributed speaker_said
// claims without an RPSpeechAccepted payload. They define no delivery; keep
// their established knowledge path without interpreting their owner data as
// an RP speech envelope or granting it a new audible tone.
func rpKnowledgeSpeechTone(sourceKind, sourceJSON, hearingJSON, speakerID, observerID string) (string, error) {
	if sourceKind == "RPSpeechAccepted" {
		return recordedRPSpeechTone(sourceJSON, hearingJSON, speakerID, observerID)
	}
	var hearing rpSpeechClaim
	if json.Unmarshal([]byte(hearingJSON), &hearing) != nil || hearing.SpeechTone != "" {
		return "", narrativeDiverged("non-speech owner hearing has no approved delivery")
	}
	return "", nil
}

func readRPRecordedSpeechTone(ctx context.Context, conn *sql.Conn, eventID, observerID string) (string, error) {
	var speakerID, sourceJSON, hearingJSON string
	if err := conn.QueryRowContext(ctx, `SELECT e.actor_id,e.payload,COALESCE(o.claim_payload,'')
	 FROM events e LEFT JOIN observation_records o ON o.source_event_id=e.event_id
	 AND o.observer_agent_id=? AND o.subject_agent_id=e.actor_id AND o.claim_key='speech:'||e.event_id
	 WHERE e.event_id=? AND e.event_type='RPSpeechAccepted'`, observerID, eventID).Scan(&speakerID, &sourceJSON, &hearingJSON); err != nil {
		return "", classifyMissing(err, "accepted speech delivery")
	}
	return recordedRPSpeechTone(sourceJSON, hearingJSON, speakerID, observerID)
}
