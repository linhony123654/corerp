package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"sort"
)

type LawAnnouncementRequest struct {
	Binding          core.CareerBinding `json:"binding"`
	SpeakerID        string             `json:"speaker_id"`
	EnactmentEventID string             `json:"enactment_event_id"`
}
type LawAnnouncementFact struct {
	SpeakerID        string          `json:"speaker_id"`
	EnactmentEventID string          `json:"enactment_event_id"`
	PlaceID          string          `json:"place_id"`
	ListenerIDs      []string        `json:"listener_ids"`
	Law              core.RPKnownLaw `json:"law"`
}

func publicEnactedLaw(institution string, enactment LawEnactmentFact, definition InstitutionDefinitionFact, eventID string) core.RPKnownLaw {
	return core.RPKnownLaw{InstitutionID: institution, LawID: enactment.Law.LawID, EnactmentEventID: eventID, ScopeKind: definition.Territory.ScopeKind, ScopeID: definition.Territory.ScopeID, PlaceIDs: append([]string(nil), definition.Territory.PlaceIDs...), EffectiveWorldTime: enactment.EffectiveWorldTime, ProhibitedAction: enactment.Law.ProhibitedAction, FineMinor: enactment.Law.FineMinor, Text: enactment.Law.Text, PreviousEnactmentEventID: enactment.PreviousEnactmentEventID, Repealed: enactment.Repealed}
}

func (s *Store) AnnounceRPLaw(ctx context.Context, r LawAnnouncementRequest) (InstitutionRecord, error) {
	return executePrivateFactCommand(s, ctx, r.Binding, "AnnounceRPLaw", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"known-law-speaker-v1"}`}, func(conn *sql.Conn) error { return authorizeCareerCandidate(ctx, conn, r.Binding, r.SpeakerID) }, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		known, err := readKnownRPLaws(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.SpeakerID)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		var law *core.RPKnownLaw
		for i := range known {
			if known[i].EnactmentEventID == r.EnactmentEventID {
				copy := known[i]
				law = &copy
			}
		}
		if law == nil {
			return InstitutionFact{}, nil, core.NewError(core.CodeUnauthorized, "speaker has not authored or heard this enacted law")
		}
		var place string
		if err := conn.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, r.SpeakerID).Scan(&place); err != nil {
			return InstitutionFact{}, nil, err
		}
		listeners, err := rpPerceivedEntityIDs(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, place, r.SpeakerID, "audio", "voice")
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		law.KnowledgeEventID = c.EventID
		encoded, err := core.CanonicalJSON(*law)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		text := "Law announcement: " + string(encoded)
		if len(text) > 2000 {
			return InstitutionFact{}, nil, core.NewError(core.CodeInvalidArgument, "law announcement exceeds bounded speech")
		}
		announcement := &LawAnnouncementFact{SpeakerID: r.SpeakerID, EnactmentEventID: r.EnactmentEventID, PlaceID: place, ListenerIDs: listeners, Law: *law}
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "law_announcement", InstitutionID: law.InstitutionID, Announcement: announcement}, func() error {
			return insertRPSpeechHearings(ctx, conn, c.EventID, c.Sequence, r.SpeakerID, place, c.WorldTime, "utterance_"+c.EventID, text, "statement", listeners)
		}, nil
	})
}

func readKnownRPLaws(ctx context.Context, conn *sql.Conn, instance, branch, actor string) ([]core.RPKnownLaw, error) {
	rows, err := conn.QueryContext(ctx, `SELECT e.event_id,e.payload,d.payload FROM events e LEFT JOIN events d ON d.event_id=json_extract(e.payload,'$.enactment.institution_event_id') AND d.instance_id=e.instance_id AND d.branch_id=e.branch_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPInstitutionFactRecorded' AND ((json_extract(e.payload,'$.kind')='law_enactment' AND json_extract(e.payload,'$.enactment.legislator_id')=?) OR (json_extract(e.payload,'$.kind')='law_announcement' AND EXISTS (SELECT 1 FROM json_each(e.payload,'$.announcement.listener_ids') l WHERE l.value=?))) ORDER BY e.event_sequence`, instance, branch, actor, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]core.RPKnownLaw{}
	for rows.Next() {
		var id, raw string
		var source sql.NullString
		var fact InstitutionFact
		if err := rows.Scan(&id, &raw, &source); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return nil, err
		}
		var law core.RPKnownLaw
		if fact.Announcement != nil {
			law = fact.Announcement.Law
			law.KnowledgeEventID = id
		} else if fact.Enactment != nil && source.Valid {
			var definition InstitutionFact
			if err := json.Unmarshal([]byte(source.String), &definition); err != nil {
				return nil, err
			}
			if definition.Definition == nil {
				return nil, core.NewError(core.CodeProjectionDiverged, "law lacks institution source")
			}
			law = publicEnactedLaw(fact.InstitutionID, *fact.Enactment, *definition.Definition, id)
			law.KnowledgeEventID = id
		} else {
			return nil, core.NewError(core.CodeProjectionDiverged, "known law lacks typed source")
		}
		known[law.EnactmentEventID] = law
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var result []core.RPKnownLaw
	for _, law := range known {
		result = append(result, law)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].EnactmentEventID < result[j].EnactmentEventID })
	return result, nil
}

// Authoritative rule evaluation is separate and must never populate NPC belief.
func readEffectiveInstitutionLaws(ctx context.Context, conn *sql.Conn, instance, branch, worldTime, place string) ([]core.RPKnownLaw, error) {
	rows, err := conn.QueryContext(ctx, `SELECT e.event_id,e.payload,d.payload FROM events e JOIN events d ON d.event_id=json_extract(e.payload,'$.enactment.institution_event_id') AND d.instance_id=e.instance_id AND d.branch_id=e.branch_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPInstitutionFactRecorded' AND json_extract(e.payload,'$.kind')='law_enactment' ORDER BY e.event_sequence`, instance, branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []core.RPKnownLaw
	for rows.Next() {
		var id, raw, source string
		var fact, definition InstitutionFact
		if err := rows.Scan(&id, &raw, &source); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(source), &definition); err != nil {
			return nil, err
		}
		if fact.Enactment == nil || definition.Definition == nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "effective law lacks source")
		}
		law := publicEnactedLaw(fact.InstitutionID, *fact.Enactment, *definition.Definition, id)
		result = append(result, law)
	}
	return core.EffectiveRPLaws(result, worldTime, place), rows.Err()
}
