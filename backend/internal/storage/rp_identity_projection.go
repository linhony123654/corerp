package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpIdentityProjection struct {
	Observer string `json:"observer"`
	Subject  string `json:"subject"`
	Instance string `json:"instance"`
	Branch   string `json:"branch"`
	SourceID string `json:"source_id"`
	Time     string `json:"time"`
	Origin   string `json:"origin"`
}

func rpIdentityKey(observer, subject string) string { return observer + "|" + subject }

func rpIdentityExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpIdentityProjection, error) {
	want := map[string]rpIdentityProjection{}
	var cutover int64
	err := q.QueryRowContext(ctx, `SELECT through_sequence FROM rp_identity_cutovers WHERE instance_id=? AND branch_id=?`, instance, branch).Scan(&cutover)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT l.observer_agent_id,l.subject_agent_id,l.source_event_id,l.learned_world_time,e.event_sequence FROM rp_identity_legacy_sources l JOIN events e ON e.event_id=l.source_event_id WHERE l.instance_id=? AND l.branch_id=? ORDER BY l.observer_agent_id,l.subject_agent_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var observer, subject, source, worldTime string
		var sequence int64
		if err := rows.Scan(&observer, &subject, &source, &worldTime, &sequence); err != nil {
			rows.Close()
			return nil, err
		}
		if observer == subject || sequence > cutover || sequence > through {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid identity legacy source")
		}
		want[rpIdentityKey(observer, subject)] = rpIdentityProjection{observer, subject, instance, branch, source, worldTime, "legacy"}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = q.QueryContext(ctx, `SELECT e.event_id,e.event_type,e.world_time,e.payload,COALESCE(m.display_name,'') FROM events e LEFT JOIN materialized_entities m ON m.entity_id=e.actor_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND (e.event_type='RPParticipantsInitialized' OR e.event_type='RPIdentitiesDeclared' OR (e.event_type='RPSpeechAccepted' AND e.event_sequence>?)) ORDER BY e.event_sequence`, instance, branch, through, cutover)
	if err != nil {
		return nil, err
	}
	add := func(observer, subject, source, worldTime, origin string) error {
		if observer == "" || subject == "" || observer == subject {
			return core.NewError(core.CodeProjectionDiverged, "invalid identity source pair")
		}
		key := rpIdentityKey(observer, subject)
		if _, exists := want[key]; !exists {
			want[key] = rpIdentityProjection{observer, subject, instance, branch, source, worldTime, origin}
		}
		return nil
	}
	for rows.Next() {
		var id, kind, worldTime, raw, speakerName string
		if err := rows.Scan(&id, &kind, &worldTime, &raw, &speakerName); err != nil {
			rows.Close()
			return nil, err
		}
		if kind == "RPParticipantsInitialized" {
			var fact struct {
				PlayerEntityID string   `json:"player_entity_id"`
				NPCEntityIDs   []string `json:"npc_entity_ids"`
			}
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, err
			}
			// The demo's declared acquaintance is its player and Cai, not every initialized NPC.
			if fact.PlayerEntityID != M2RPPlayerID {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid demo identity source")
			}
			if err := add(M2RPPlayerID, M2RPNPCID, id, worldTime, "demo"); err != nil {
				rows.Close()
				return nil, err
			}
			if err := add(M2RPNPCID, M2RPPlayerID, id, worldTime, "demo"); err != nil {
				rows.Close()
				return nil, err
			}
			continue
		}
		if kind == "RPIdentitiesDeclared" {
			var fact struct {
				Pairs [][2]string `json:"pairs"`
			}
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, err
			}
			for _, pair := range fact.Pairs {
				if err := add(pair[0], pair[1], id, worldTime, "declared"); err != nil {
					rows.Close()
					return nil, err
				}
				if err := add(pair[1], pair[0], id, worldTime, "declared"); err != nil {
					rows.Close()
					return nil, err
				}
			}
			continue
		}
		var fact rpSpeechEvent
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return nil, err
		}
		if !fact.IntroduceSelf || !core.ExplicitSelfIntroduction(fact.Text, speakerName) {
			continue
		}
		for _, listener := range fact.ListenerIDs {
			if err := add(listener, fact.SpeakerEntityID, id, worldTime, "introduction"); err != nil {
				rows.Close()
				return nil, err
			}
		}
	}
	err = rows.Err()
	rows.Close()
	return want, err
}

func rpIdentityProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpIdentityExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var got rpIdentityProjection
		err := q.QueryRowContext(ctx, `SELECT observer_agent_id,subject_agent_id,instance_id,branch_id,source_event_id,learned_world_time,origin_kind FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=?`, want[key].Observer, want[key].Subject).Scan(&got.Observer, &got.Subject, &got.Instance, &got.Branch, &got.SourceID, &got.Time, &got.Origin)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		expected, err := core.CanonicalJSON(want[key])
		if err != nil {
			return nil, err
		}
		actual := "missing"
		if err == nil {
			encoded, err := core.CanonicalJSON(got)
			if err != nil {
				return nil, err
			}
			actual = string(encoded)
		}
		if string(expected) != actual {
			differences = append(differences, ProjectionDifference{Projection: "rp_identity", Key: key, ExpectedText: string(expected), ActualText: actual})
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT observer_agent_id,subject_agent_id FROM rp_identity_familiarity WHERE instance_id=? AND branch_id=? ORDER BY observer_agent_id,subject_agent_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var observer, subject string
		if err := rows.Scan(&observer, &subject); err != nil {
			rows.Close()
			return nil, err
		}
		key := rpIdentityKey(observer, subject)
		if _, ok := want[key]; !ok {
			differences = append(differences, ProjectionDifference{Projection: "rp_identity_extra", Key: key, ExpectedText: "absent", ActualText: "unsourced projection row"})
		}
	}
	err = rows.Err()
	rows.Close()
	return differences, err
}

func repairRPIdentityProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, difference := range differences {
		if difference.Projection != "rp_identity" || difference.ActualText != "missing" {
			return core.NewError(core.CodeProjectionDiverged, "cannot rewrite identity history")
		}
		var row rpIdentityProjection
		if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
			return err
		}
		if row.Instance != instance || row.Branch != branch || rpIdentityKey(row.Observer, row.Subject) != difference.Key {
			return core.NewError(core.CodeProjectionDiverged, "identity repair scope differs")
		}
		if err := execAgentOne(ctx, conn, "repair identity familiarity", `INSERT INTO rp_identity_familiarity(observer_agent_id,subject_agent_id,instance_id,branch_id,source_event_id,learned_world_time,origin_kind) VALUES (?,?,?,?,?,?,?)`, row.Observer, row.Subject, instance, branch, row.SourceID, row.Time, row.Origin); err != nil {
			return err
		}
	}
	return nil
}
