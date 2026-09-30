package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpNonverbalObservationRow struct {
	ID, EventID, Observer, Subject, Place, Channel, WorldTime, ClaimKey, Payload string
}

type rpNonverbalOwnRow struct {
	Actor, EventID, Action, Activity, Text, Place, WorldTime, Instance, Branch string
	Sequence                                                                   int64
}

func expectedRPNonverbalRows(ctx context.Context, q replayQuerier, instanceID, branchID string, head int64) (map[string]rpNonverbalObservationRow, map[string]rpNonverbalOwnRow, error) {
	observations := map[string]rpNonverbalObservationRow{}
	own := map[string]rpNonverbalOwnRow{}
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,actor_id,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPNonverbalAction' AND event_sequence<=? ORDER BY event_sequence`, instanceID, branchID, head)
	if err != nil {
		return nil, nil, core.WrapError(core.CodeStorageFailure, "read sourced nonverbal events", err)
	}
	for rows.Next() {
		var eventID, actor, worldTime, raw string
		var sequence int64
		if err := rows.Scan(&eventID, &sequence, &actor, &worldTime, &raw); err != nil {
			rows.Close()
			return nil, nil, err
		}
		var fact core.RPNonverbalFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.ActorEntityID != actor || fact.ClaimType != "nonverbal_action" || fact.PlaceID == "" || fact.Description == "" {
			rows.Close()
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "nonverbal Event lacks a valid actor fact")
		}
		own[eventID] = rpNonverbalOwnRow{Actor: actor, EventID: eventID, Action: "nonverbal", Activity: fact.Action, Text: fact.Description, Place: fact.PlaceID, WorldTime: worldTime, Instance: instanceID, Branch: branchID, Sequence: sequence}
		seen := map[string]bool{}
		for _, witness := range fact.Witnesses {
			if witness.ObserverEntityID == "" || witness.ObserverEntityID == actor || seen[witness.ObserverEntityID] || witness.TargetVisible && fact.TargetEntityID == "" {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "nonverbal Event has invalid frozen witness")
			}
			seen[witness.ObserverEntityID] = true
			claim, err := core.CanonicalJSON(rpNonverbalWitnessClaim(fact, witness))
			if err != nil {
				rows.Close()
				return nil, nil, err
			}
			id := "observation_" + eventID + "_" + witness.ObserverEntityID
			observations[id] = rpNonverbalObservationRow{id, eventID, witness.ObserverEntityID, actor, fact.PlaceID, "co_location", worldTime, "nonverbal:" + eventID, string(claim)}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	return observations, own, nil
}

func rpNonverbalProjectionDifferences(ctx context.Context, q replayQuerier, instanceID, branchID string, head int64) ([]ProjectionDifference, error) {
	observations, own, err := expectedRPNonverbalRows(ctx, q, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	rows, err := q.QueryContext(ctx, `SELECT o.observation_id,o.source_event_id,o.observer_agent_id,o.subject_agent_id,o.place_id,o.channel,o.observed_world_time,o.claim_key,o.claim_payload FROM observation_records o JOIN events e ON e.event_id=o.source_event_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPNonverbalAction' AND e.event_sequence<=?`, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var actual rpNonverbalObservationRow
		if err := rows.Scan(&actual.ID, &actual.EventID, &actual.Observer, &actual.Subject, &actual.Place, &actual.Channel, &actual.WorldTime, &actual.ClaimKey, &actual.Payload); err != nil {
			rows.Close()
			return nil, err
		}
		if expected, ok := observations[actual.ID]; !ok || expected != actual {
			differences = append(differences, ProjectionDifference{Projection: "rp_nonverbal_observation", Key: actual.ID, ExpectedText: fmt.Sprint(expected), ActualText: fmt.Sprint(actual)})
		}
		delete(observations, actual.ID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for id, expected := range observations {
		differences = append(differences, ProjectionDifference{Projection: "rp_nonverbal_observation", Key: id, ExpectedText: fmt.Sprint(expected)})
	}
	rows, err = q.QueryContext(ctx, `SELECT a.agent_id,a.event_id,a.action,COALESCE(a.activity_code,''),COALESCE(a.text,''),COALESCE(a.place_id,''),a.world_time,a.instance_id,a.branch_id,a.last_event_sequence FROM rp_own_actions a JOIN events e ON e.event_id=a.event_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPNonverbalAction' AND e.event_sequence<=?`, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var actual rpNonverbalOwnRow
		if err := rows.Scan(&actual.Actor, &actual.EventID, &actual.Action, &actual.Activity, &actual.Text, &actual.Place, &actual.WorldTime, &actual.Instance, &actual.Branch, &actual.Sequence); err != nil {
			rows.Close()
			return nil, err
		}
		if expected, ok := own[actual.EventID]; !ok || expected != actual {
			differences = append(differences, ProjectionDifference{Projection: "rp_nonverbal_own_action", Key: actual.EventID, ExpectedText: fmt.Sprint(expected), ActualText: fmt.Sprint(actual)})
		}
		delete(own, actual.EventID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for id, expected := range own {
		differences = append(differences, ProjectionDifference{Projection: "rp_nonverbal_own_action", Key: id, ExpectedText: fmt.Sprint(expected)})
	}
	sort.Slice(differences, func(i, j int) bool {
		if differences[i].Projection == differences[j].Projection {
			return differences[i].Key < differences[j].Key
		}
		return differences[i].Projection < differences[j].Projection
	})
	return differences, nil
}

func repairRPNonverbalProjections(ctx context.Context, conn *sql.Conn, instanceID, branchID string, head int64, differences []ProjectionDifference) error {
	if len(differences) == 0 {
		return nil
	}
	observations, own, err := expectedRPNonverbalRows(ctx, conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM agent_knowledge WHERE source_event_id IN (SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPNonverbalAction' AND event_sequence<=?)`, instanceID, branchID, head); err != nil {
		return core.WrapError(core.CodeStorageFailure, "clear sourced nonverbal knowledge", err)
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM observation_records WHERE source_event_id IN (SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPNonverbalAction' AND event_sequence<=?)`, instanceID, branchID, head); err != nil {
		return core.WrapError(core.CodeStorageFailure, "clear sourced nonverbal observations", err)
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM rp_own_actions WHERE event_id IN (SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPNonverbalAction' AND event_sequence<=?)`, instanceID, branchID, head); err != nil {
		return core.WrapError(core.CodeStorageFailure, "clear sourced nonverbal own actions", err)
	}
	obsIDs := make([]string, 0, len(observations))
	for id := range observations {
		obsIDs = append(obsIDs, id)
	}
	sort.Strings(obsIDs)
	for _, id := range obsIDs {
		row := observations[id]
		if _, err := conn.ExecContext(ctx, `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,?,?,?,?)`, row.ID, row.EventID, row.Observer, row.Subject, row.Place, row.Channel, row.WorldTime, row.ClaimKey, row.Payload); err != nil {
			return core.WrapError(core.CodeStorageFailure, "restore frozen nonverbal witness", err)
		}
	}
	ownIDs := make([]string, 0, len(own))
	for id := range own {
		ownIDs = append(ownIDs, id)
	}
	sort.Strings(ownIDs)
	for _, id := range ownIDs {
		row := own[id]
		if _, err := conn.ExecContext(ctx, `INSERT INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence) VALUES (?,?,?,?,?,?,?,NULL,?,?,?)`, row.Actor, row.EventID, row.Action, row.Activity, row.Text, row.Place, row.WorldTime, row.Instance, row.Branch, row.Sequence); err != nil {
			return core.WrapError(core.CodeStorageFailure, "restore nonverbal own action", err)
		}
	}
	return nil
}
