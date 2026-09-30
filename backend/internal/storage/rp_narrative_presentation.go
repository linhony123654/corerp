package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

type rpPublicPresentationSource struct {
	Text          string
	SourceEventID string
}

// The declaration is the only source of narrator-safe character style. In
// particular, agent_profiles.persona_text is private decision context and
// cannot be copied into a public presentation by inference.
func readRPPublicPresentations(ctx context.Context, conn *sql.Conn, instance, branch string) (map[string]rpPublicPresentationSource, error) {
	var eventID, raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence=1 AND event_type='StudioWorldPrepared'`, instance, branch).Scan(&eventID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // Legacy/non-Studio worlds have no public declaration.
	}
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read public RP presentation source", err)
	}
	var declaration struct {
		Request struct {
			Spec struct {
				People []struct {
					Key                string `json:"key"`
					PublicPresentation string `json:"public_presentation"`
				} `json:"people"`
			} `json:"spec"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(raw), &declaration); err != nil {
		return nil, core.WrapError(core.CodeProjectionDiverged, "invalid public RP presentation source", err)
	}
	result := make(map[string]rpPublicPresentationSource)
	for _, person := range declaration.Request.Spec.People {
		cue := person.PublicPresentation
		if cue == "" {
			continue
		}
		if !utf8.ValidString(cue) || utf8.RuneCountInString(cue) > 500 || strings.ContainsAny(cue, "\x00\r") {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid public RP presentation declaration")
		}
		actorID, err := core.StudioWorldObjectID(instance, "entity", person.Key)
		if err != nil {
			return nil, err
		}
		result[actorID] = rpPublicPresentationSource{Text: cue, SourceEventID: eventID}
	}
	return result, nil
}
