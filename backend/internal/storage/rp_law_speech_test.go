package storage

import (
	"corerp.local/backend/internal/core"
	"testing"
)

func TestRPLawSpeechEvidenceRequiresSpokenDomainFact(t *testing.T) {
	for _, tc := range []struct {
		kind, raw string
		accepted  bool
	}{
		{"RPCareerFactRecorded", `{"kind":"announcement","announcement":{"speaker_id":"speaker","place_id":"room","listener_ids":["witness"]}}`, true},
		{"RPCareerFactRecorded", `{"kind":"offer","announcement":{"speaker_id":"speaker","place_id":"room"}}`, false},
		{"RPInstitutionFactRecorded", `{"kind":"law_dispute","actor_id":"speaker","place_id":"room"}`, false},
		{"RPCultureFactRecorded", `{"kind":"internalization","actor_id":"speaker","place_id":"room"}`, false},
		{"RPNPCDecisionCommitted", `{"speaker_entity_id":"speaker","place_id":"room"}`, false},
	} {
		got, err := lawSpeechEvidence(tc.kind, tc.raw)
		if tc.accepted {
			if err != nil || got.SpeakerEntityID != "speaker" || got.PlaceID != "room" || len(got.ListenerIDs) != 1 || got.ListenerIDs[0] != "witness" {
				t.Fatalf("spoken fact %+v %v", got, err)
			}
		} else if !core.HasCode(err, core.CodeNotFound) {
			t.Fatalf("nonspoken %s: %v", tc.raw, err)
		}
	}
}
