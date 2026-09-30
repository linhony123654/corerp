package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

// Neutral by construction: even a witness who sees the actor has not thereby
// seen another participant, their consent, the SKU, or the inventory source.
func rpObjectNeutralClaim() (string, error) {
	encoded, err := core.CanonicalJSON(struct {
		ClaimType   string `json:"claim_type"`
		Action      string `json:"action"`
		Description string `json:"description"`
	}{"object_interaction", "interact", "有人在近处做出与物品相关的动作。"})
	return string(encoded), err
}

// Called before committing the Event, so historical sight can be rebuilt
// without consulting a character's later position, zone or relationship.
func readRPObjectWitnesses(ctx context.Context, conn *sql.Conn, session RPSession, place, target string) ([]string, []string, error) {
	others, err := rpCoLocatedEntityIDs(ctx, conn, session.InstanceID, session.BranchID, place, session.ControlledEntityID)
	if err != nil {
		return nil, nil, err
	}
	witnesses, targetVisible := []string{}, []string{}
	for _, other := range others {
		visible, err := rpCanPerceive(ctx, conn, session.InstanceID, session.BranchID, other, session.ControlledEntityID, "visual", "")
		if err != nil {
			return nil, nil, err
		}
		if !visible {
			continue
		}
		witnesses = append(witnesses, other)
		if target == "" {
			continue
		}
		visible, err = rpCanPerceive(ctx, conn, session.InstanceID, session.BranchID, other, target, "visual", "")
		if err != nil {
			return nil, nil, err
		}
		if visible {
			targetVisible = append(targetVisible, other)
		}
	}
	return witnesses, targetVisible, nil
}

type rpObjectObservationProjection struct {
	ID, Event, Observer, Subject, Place, Channel, WorldTime, ClaimKey, ClaimPayload string
}

type rpObjectOutboxProjection struct {
	ID, Event, Topic, AudienceScope, AudienceHash, Payload string
}

func rpObjectObservationExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpObjectObservationProjection, map[string]rpObjectOutboxProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT e.event_id,e.event_sequence,e.actor_id,e.world_time,e.payload,b.command_id FROM events e JOIN event_batches b ON b.batch_id=e.batch_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND e.event_type='RPObjectInteracted' ORDER BY e.event_sequence`, instance, branch, through)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	claim, err := rpObjectNeutralClaim()
	if err != nil {
		return nil, nil, err
	}
	observations := map[string]rpObjectObservationProjection{}
	outboxes := map[string]rpObjectOutboxProjection{}
	for rows.Next() {
		var id, actor, worldTime, raw, commandID string
		var sequence int64
		if err := rows.Scan(&id, &sequence, &actor, &worldTime, &raw, &commandID); err != nil {
			return nil, nil, err
		}
		var fact core.RPObjectEvidence
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return nil, nil, err
		}
		if fact.Version != "corerp.object.interaction.v1" || fact.ActorEntityID != actor || fact.PlaceID == "" || fact.SessionID == "" || !sort.StringsAreSorted(fact.WitnessIDs) || !sort.StringsAreSorted(fact.VisibleTargetWitnessIDs) {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid frozen object visibility Event")
		}
		seen := map[string]bool{}
		for _, witness := range fact.WitnessIDs {
			if witness == "" || witness == actor || seen[witness] {
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "duplicated or self-witnessed object Event")
			}
			seen[witness] = true
			observationID := "observation_" + id + "_" + witness
			observations[observationID] = rpObjectObservationProjection{observationID, id, witness, actor, fact.PlaceID, "co_location", worldTime, "object:" + id, claim}
		}
		targetSeen := map[string]bool{}
		for _, witness := range fact.VisibleTargetWitnessIDs {
			if fact.TargetEntityID == "" || !seen[witness] || targetSeen[witness] {
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "target sight without actor sight in object Event")
			}
			targetSeen[witness] = true
		}
		// No read of current positions: both the claim and audience derive only
		// from the committed Event and the immutable neutral serialization.
		audience := struct {
			Kind       string   `json:"kind"`
			InstanceID string   `json:"instance_id"`
			BranchID   string   `json:"branch_id"`
			EntityIDs  []string `json:"entity_ids"`
		}{"rp_participants", instance, branch, []string{actor}}
		audienceJSON, err := core.CanonicalJSON(audience)
		if err != nil {
			return nil, nil, err
		}
		hash, err := core.HashJSON(audience)
		if err != nil {
			return nil, nil, err
		}
		outboxID := "outbox_" + commandID
		outboxes[outboxID] = rpObjectOutboxProjection{outboxID, id, "rp.object.interacted", string(audienceJSON), hash, claim}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return observations, outboxes, nil
}

func rpObjectObservationProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	expected, outboxes, err := rpObjectObservationExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	compare := func(kind, key string, want, got any, err error) error {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		expectedJSON, err := core.CanonicalJSON(want)
		if err != nil {
			return err
		}
		actual := "missing"
		if err == nil {
			gotJSON, err := core.CanonicalJSON(got)
			if err != nil {
				return err
			}
			actual = string(gotJSON)
		}
		if actual != string(expectedJSON) {
			differences = append(differences, ProjectionDifference{Projection: kind, Key: key, ExpectedText: string(expectedJSON), ActualText: actual})
		}
		return nil
	}
	for _, key := range rpObjectSortedKeys(expected) {
		var got rpObjectObservationProjection
		err := q.QueryRowContext(ctx, `SELECT observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload FROM observation_records WHERE observation_id=?`, key).Scan(&got.ID, &got.Event, &got.Observer, &got.Subject, &got.Place, &got.Channel, &got.WorldTime, &got.ClaimKey, &got.ClaimPayload)
		if err := compare("rp_object_observation", key, expected[key], got, err); err != nil {
			return nil, err
		}
	}
	for _, key := range rpObjectSortedKeys(outboxes) {
		var got rpObjectOutboxProjection
		err := q.QueryRowContext(ctx, `SELECT outbox_id,event_id,topic,audience_scope,audience_scope_hash,payload FROM outbox WHERE outbox_id=?`, key).Scan(&got.ID, &got.Event, &got.Topic, &got.AudienceScope, &got.AudienceHash, &got.Payload)
		if err := compare("rp_object_outbox", key, outboxes[key], got, err); err != nil {
			return nil, err
		}
	}
	for _, scope := range []struct {
		query, kind string
		known       func(string) bool
	}{
		{`SELECT o.observation_id FROM observation_records o JOIN events e ON e.event_id=o.source_event_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPObjectInteracted'`, "rp_object_observation_extra", func(id string) bool { _, ok := expected[id]; return ok }},
		{`SELECT o.outbox_id FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPObjectInteracted'`, "rp_object_outbox_extra", func(id string) bool { _, ok := outboxes[id]; return ok }},
	} {
		rows, err := q.QueryContext(ctx, scope.query, instance, branch)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			if !scope.known(id) {
				differences = append(differences, ProjectionDifference{Projection: scope.kind, Key: id, ExpectedText: "absent", ActualText: "unsourced object witness or outbox"})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return differences, nil
}

// Must run before replayRange: replay consumes observation_records for
// agent_knowledge. Repairing only agent_knowledge would copy a forged claim.
func repairRPObjectObservationProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, d := range differences {
		if d.Projection == "rp_object_observation_extra" || d.Projection == "rp_object_outbox_extra" {
			return core.NewError(core.CodeProjectionDiverged, "cannot repair unexpected object witness or published outbox")
		}
	}
	for _, d := range differences {
		switch d.Projection {
		case "rp_object_observation":
			var row rpObjectObservationProjection
			if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil || row.ID != d.Key {
				return core.NewError(core.CodeProjectionDiverged, "object witness repair fact invalid")
			}
			var scoped int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPObjectInteracted'`, row.Event, instance, branch).Scan(&scoped); err != nil || scoped != 1 {
				return core.NewError(core.CodeProjectionDiverged, "object witness repair Event outside branch")
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(observation_id) DO UPDATE SET source_event_id=excluded.source_event_id,observer_agent_id=excluded.observer_agent_id,subject_agent_id=excluded.subject_agent_id,place_id=excluded.place_id,channel=excluded.channel,observed_world_time=excluded.observed_world_time,claim_key=excluded.claim_key,claim_payload=excluded.claim_payload`, row.ID, row.Event, row.Observer, row.Subject, row.Place, row.Channel, row.WorldTime, row.ClaimKey, row.ClaimPayload); err != nil {
				return err
			}
		case "rp_object_outbox":
			var row rpObjectOutboxProjection
			if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil || row.ID != d.Key {
				return core.NewError(core.CodeProjectionDiverged, "object outbox repair fact invalid")
			}
			var scoped, published int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPObjectInteracted'`, row.Event, instance, branch).Scan(&scoped); err != nil || scoped != 1 {
				return core.NewError(core.CodeProjectionDiverged, "object outbox repair Event outside branch")
			}
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox WHERE outbox_id=? AND published_at_utc IS NOT NULL`, row.ID).Scan(&published); err != nil || published != 0 {
				return core.NewError(core.CodeProjectionDiverged, "published object outbox cannot be retracted safely")
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO outbox(outbox_id,event_id,topic,audience_scope,audience_scope_hash,payload) VALUES (?,?,?,?,?,?) ON CONFLICT(outbox_id) DO UPDATE SET event_id=excluded.event_id,topic=excluded.topic,audience_scope=excluded.audience_scope,audience_scope_hash=excluded.audience_scope_hash,payload=excluded.payload`, row.ID, row.Event, row.Topic, row.AudienceScope, row.AudienceHash, row.Payload); err != nil {
				return err
			}
		}
	}
	return nil
}
