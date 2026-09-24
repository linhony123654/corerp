package storage

import (
	"context"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

type RPEventsReadRequest struct {
	PrincipalID string
	SessionID   string
	After       int64
	Limit       int
}

// RPClientAction intentionally excludes raw event payloads, listener lists,
// scheduler diagnostics, NPC decisions and private opportunity draw receipts.
type RPClientAction struct {
	Kind            string `json:"kind"`
	Text            string `json:"text,omitempty"`
	PlaceID         string `json:"place_id,omitempty"`
	FromPlaceID     string `json:"from_place_id,omitempty"`
	ToPlaceID       string `json:"to_place_id,omitempty"`
	FromWorldTime   string `json:"from_world_time,omitempty"`
	TargetWorldTime string `json:"target_world_time,omitempty"`
}

type RPClientEvent struct {
	EventID   string          `json:"event_id"`
	Sequence  int64           `json:"sequence"`
	WorldTime string          `json:"world_time"`
	OwnAction *RPClientAction `json:"own_action,omitempty"`
	Facts     []RPContextFact `json:"facts"`
}

type RPClientEvents struct {
	ProtocolVersion  string          `json:"protocol_version"`
	SessionID        string          `json:"session_id"`
	InstanceID       string          `json:"instance_id"`
	BranchID         string          `json:"branch_id"`
	ObserverEntityID string          `json:"observer_entity_id"`
	WorldTime        string          `json:"world_time"`
	HeadSequence     int64           `json:"head_sequence"`
	HistoryRevision  int64           `json:"history_revision"`
	NextSequence     int64           `json:"next_sequence"`
	MoreEvents       bool            `json:"more_events"`
	Events           []RPClientEvent `json:"events"`
}

// ReadRPEvents projects immutable observation_records, not latest-only
// agent_knowledge. All current observation writers attach evidence to the new
// learning event in the same commit. A future delayed-learning writer must use
// a new learning event too; an old source alone cannot be a continuation key.
func (s *Store) ReadRPEvents(ctx context.Context, r RPEventsReadRequest) (RPClientEvents, error) {
	var result RPClientEvents
	if err := (core.RPSessionReadRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID}).Validate(); err != nil {
		return result, err
	}
	if r.After < 0 || r.Limit < 0 || r.Limit > 50 {
		return result, core.NewError(core.CodeInvalidArgument, "events require nonnegative cursor and limit 1–50")
	}
	if r.Limit == 0 {
		r.Limit = 20
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return result, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if session.Status != "active" {
		return result, core.NewError(core.CodeBranchConflict, "events require an active session")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	result = RPClientEvents{ProtocolVersion: RPClientProtocolVersion, SessionID: session.SessionID, InstanceID: session.InstanceID, BranchID: session.BranchID, ObserverEntityID: session.ControlledEntityID, Events: []RPClientEvent{}}
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.current_world_time,b.head_sequence FROM world_clocks c JOIN branches b ON b.instance_id=c.instance_id AND b.branch_id=c.branch_id WHERE b.instance_id=? AND b.branch_id=?`, session.InstanceID, session.BranchID).Scan(&result.WorldTime, &result.HeadSequence); err != nil {
		return result, err
	}
	if r.After > result.HeadSequence {
		return result, core.NewError(core.CodeBranchConflict, "event cursor is ahead of this world")
	}
	// Turn narration can settle after its final world Event. A world-head-only
	// subscription would never refresh that late application view. Count only
	// this controlled observer's settled turns; this is not a new world Event.
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_sessions h ON h.session_id=r.session_id
	 WHERE h.instance_id=? AND h.branch_id=? AND h.controlled_entity_id=? AND r.status='settled' AND r.settled_sequence<=?`,
		session.InstanceID, session.BranchID, session.ControlledEntityID, result.HeadSequence).Scan(&result.HistoryRevision); err != nil {
		return result, err
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT e.event_id,e.event_sequence,e.world_time,e.actor_id,e.event_type,e.payload
	 FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence>? AND e.event_sequence<=? AND e.world_time<=?
	 AND ((e.actor_id=? AND e.event_type IN ('RPSpeechAccepted','RPPlayerMoved','RPWaitCompleted'))
	 OR EXISTS (SELECT 1 FROM observation_records o WHERE o.source_event_id=e.event_id AND o.observer_agent_id=? AND o.observed_world_time<=?
	 AND json_extract(o.claim_payload,'$.claim_type') IN ('agent_presence','speaker_said','interpersonal_action')))
	 ORDER BY e.event_sequence LIMIT ?`, session.InstanceID, session.BranchID, r.After, result.HeadSequence, result.WorldTime, session.ControlledEntityID, session.ControlledEntityID, result.WorldTime, r.Limit+1)
	if err != nil {
		return result, err
	}
	type source struct {
		event            RPClientEvent
		actor, kind, raw string
	}
	var sources []source
	for rows.Next() {
		var item source
		if err := rows.Scan(&item.event.EventID, &item.event.Sequence, &item.event.WorldTime, &item.actor, &item.kind, &item.raw); err != nil {
			rows.Close()
			return result, err
		}
		sources = append(sources, item)
	}
	err = rows.Err()
	rows.Close() // one SQLite connection: close before nested evidence queries
	if err != nil {
		return result, err
	}
	result.MoreEvents = len(sources) > r.Limit
	if result.MoreEvents {
		sources = sources[:r.Limit]
	}
	for _, item := range sources {
		item.event.Facts = []RPContextFact{}
		if item.actor == session.ControlledEntityID {
			var payload struct {
				Text            string `json:"text"`
				PlaceID         string `json:"place_id"`
				FromPlaceID     string `json:"from_place_id"`
				ToPlaceID       string `json:"to_place_id"`
				FromWorldTime   string `json:"from_world_time"`
				TargetWorldTime string `json:"target_world_time"`
			}
			if err := json.Unmarshal([]byte(item.raw), &payload); err != nil {
				return result, core.WrapError(core.CodeProjectionDiverged, "invalid own action event", err)
			}
			switch item.kind {
			case "RPSpeechAccepted":
				item.event.OwnAction = &RPClientAction{Kind: "speech", Text: payload.Text, PlaceID: payload.PlaceID}
			case "RPPlayerMoved":
				item.event.OwnAction = &RPClientAction{Kind: "move", FromPlaceID: payload.FromPlaceID, ToPlaceID: payload.ToPlaceID}
			case "RPWaitCompleted":
				item.event.OwnAction = &RPClientAction{Kind: "wait", FromWorldTime: payload.FromWorldTime, TargetWorldTime: payload.TargetWorldTime}
			}
		}
		facts, err := tx.conn.QueryContext(ctx, `SELECT subject_agent_id,place_id,observed_world_time,claim_payload FROM observation_records
		 WHERE source_event_id=? AND observer_agent_id=? AND observed_world_time<=?
		 AND json_extract(claim_payload,'$.claim_type') IN ('agent_presence','speaker_said','interpersonal_action') ORDER BY observation_id`, item.event.EventID, session.ControlledEntityID, result.WorldTime)
		if err != nil {
			return result, err
		}
		for facts.Next() {
			fact := RPContextFact{SourceEventID: item.event.EventID}
			var raw string
			if err := facts.Scan(&fact.SubjectEntityID, &fact.PlaceID, &fact.LearnedWorldTime, &raw); err != nil {
				facts.Close()
				return result, err
			}
			var claim struct {
				Kind        string `json:"claim_type"`
				Text        string `json:"text"`
				Description string `json:"description"`
				Action      string `json:"action"`
			}
			if err := json.Unmarshal([]byte(raw), &claim); err != nil {
				facts.Close()
				return result, core.WrapError(core.CodeProjectionDiverged, "invalid observed event evidence", err)
			}
			fact.Kind = claim.Kind
			switch claim.Kind {
			case "speaker_said":
				fact.Text = claim.Text
			case "interpersonal_action":
				fact.Text, fact.Action = claim.Description, claim.Action
			}
			item.event.Facts = append(item.event.Facts, fact)
		}
		err = facts.Err()
		facts.Close()
		if err != nil {
			return result, err
		}
		result.Events = append(result.Events, item.event)
	}
	result.NextSequence = result.HeadSequence
	if result.MoreEvents {
		result.NextSequence = result.Events[len(result.Events)-1].Sequence
	}
	return result, nil
}
