package storage

import (
	"context"
	"database/sql"
	"time"

	"corerp.local/backend/internal/core"
)

type rpCommunityChangeSource struct {
	Law            core.RPKnownLaw `json:"law"`
	KnownSince     string          `json:"known_since"`
	AvailableSince string          `json:"available_since"`
}

// Only the actor's current place and actual hearing/legislator evidence. This
// reader creates no opportunity, speech, Knowledge or community change.
func readRPCommunityChangeSources(ctx context.Context, conn *sql.Conn, instance, branch, actor, start, target string) ([]rpCommunityChangeSource, error) {
	from, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339, target)
	if err != nil {
		return nil, err
	}
	if from.After(at) {
		return nil, core.NewError(core.CodeInvalidArgument, "community history starts after its endpoint")
	}
	var place string
	if err := conn.QueryRowContext(ctx, `SELECT p.place_id FROM agent_positions p JOIN agent_profiles a ON a.agent_id=p.agent_id WHERE a.agent_id=? AND a.instance_id=? AND a.branch_id=? AND a.status='active'`, actor, instance, branch).Scan(&place); err != nil {
		return nil, classifyMissing(err, "community observer")
	}
	known, err := readKnownRPLaws(ctx, conn, instance, branch, actor)
	if err != nil {
		return nil, err
	}
	// Knowledge may be newer than the requested historical endpoint. Filter
	// before selecting versions, so a later hearing cannot suppress an older
	// actually-known revision. First hearing also prevents announcement spam
	// from refreshing the age of an unchanged source.
	var eligible []core.RPKnownLaw
	firstKnown := map[string]string{}
	for _, law := range known {
		var firstID, when string
		if err := conn.QueryRowContext(ctx, `SELECT event_id,world_time FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND ((event_id=? AND json_extract(payload,'$.kind')='law_enactment' AND json_extract(payload,'$.enactment.legislator_id')=?) OR (json_extract(payload,'$.kind')='law_announcement' AND json_extract(payload,'$.announcement.enactment_event_id')=? AND EXISTS (SELECT 1 FROM json_each(payload,'$.announcement.listener_ids') l WHERE l.value=?))) ORDER BY event_sequence LIMIT 1`, instance, branch, law.EnactmentEventID, actor, law.EnactmentEventID, actor).Scan(&firstID, &when); err != nil {
			return nil, err
		}
		learned, err := time.Parse(time.RFC3339, when)
		if err != nil {
			return nil, err
		}
		if learned.After(at) {
			continue
		}
		law.KnowledgeEventID = firstID
		eligible = append(eligible, law)
		firstKnown[law.EnactmentEventID] = learned.UTC().Format(time.RFC3339)
	}
	var result []rpCommunityChangeSource
	for _, law := range core.CurrentKnownRPCommunityChanges(eligible, target, place) {
		learned, _ := time.Parse(time.RFC3339, firstKnown[law.EnactmentEventID])
		effective, _ := time.Parse(time.RFC3339, law.EffectiveWorldTime)
		available := learned
		if effective.After(available) {
			available = effective
		}
		if available.Before(from) {
			continue
		}
		result = append(result, rpCommunityChangeSource{Law: law, KnownSince: learned.UTC().Format(time.RFC3339), AvailableSince: available.UTC().Format(time.RFC3339)})
	}
	return result, nil
}
