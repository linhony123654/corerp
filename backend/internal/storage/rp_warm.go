package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

type rpWarmFact struct {
	Version             string              `json:"version"`
	SessionID           string              `json:"session_id"`
	TriggerEventID      string              `json:"trigger_event_id"`
	Selection           rpWarmCandidate     `json:"selection"`
	Decision            core.RPWarmDecision `json:"decision"`
	WorkPath            []string            `json:"work_path,omitempty"`
	RouteSourceEventIDs []string            `json:"route_source_event_ids,omitempty"`
}

func readRPWarmTrigger(ctx context.Context, conn *sql.Conn, session RPSession, r core.RPInitiativeRequest) (rpWarmCandidate, string, int64, error) {
	var raw, at string
	var sequence int64
	err := conn.QueryRowContext(ctx, `SELECT payload,world_time,event_sequence FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND json_extract(payload,'$.session_id')=? AND json_extract(payload,'$.entity_id')=?`, r.TriggerEventID, session.InstanceID, session.BranchID, session.SessionID, session.ControlledEntityID).Scan(&raw, &at, &sequence)
	if err != nil {
		return rpWarmCandidate{}, "", 0, classifyMissing(err, "own WARM wait trigger")
	}
	var event rpWaitEvent
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		return rpWarmCandidate{}, "", 0, err
	}
	for _, candidate := range event.WarmCandidates {
		if candidate.ActorID == r.NPCEntityID && candidate.SourceEventID != "" {
			return candidate, at, sequence, nil
		}
	}
	return rpWarmCandidate{}, "", 0, core.NewError(core.CodeNotFound, "actor absent from committed WARM roster")
}

func (s *Store) runRPWarmDecision(ctx context.Context, r core.RPInitiativeRequest) (privateFactRecord[rpWarmFact], error) {
	var empty privateFactRecord[rpWarmFact]
	if err := r.Validate(); err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	var head int64
	if err == nil {
		err = tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, session.InstanceID, session.BranchID).Scan(&head)
	}
	tx.Rollback(ctx)
	if err != nil {
		return empty, err
	}
	key, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	b := core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: session.InstanceID, BranchID: session.BranchID, ExpectedHead: head, IdempotencyKey: key}
	return executePrivateFactCommand(s, ctx, b, "RPWarmDecision", r, privateFactDomain{"rp_warm", "RPWarmDecisionRecorded", `{"authorization":"rp-warm-trigger-v1"}`}, func(conn *sql.Conn) error {
		current, err := loadRPSession(ctx, conn, r.PrincipalID, r.SessionID)
		if err != nil {
			return err
		}
		if err := authorizeRPControl(ctx, conn, r.PrincipalID, current.InstanceID, current.BranchID, current.ControlledEntityID); err != nil {
			return err
		}
		_, _, _, err = readRPWarmTrigger(ctx, conn, current, r)
		return err
	}, func(conn *sql.Conn, c privateFactContext) (rpWarmFact, func() error, error) {
		var no rpWarmFact
		current, err := loadRPSession(ctx, conn, r.PrincipalID, r.SessionID)
		if err != nil {
			return no, nil, err
		}
		if current.Status != "active" {
			return no, nil, core.NewError(core.CodeBranchConflict, "WARM session is no longer active")
		}
		selection, at, sequence, err := readRPWarmTrigger(ctx, conn, current, r)
		if err != nil {
			return no, nil, err
		}
		if at != c.WorldTime {
			return no, nil, core.NewError(core.CodeBranchConflict, "WARM trigger time is stale")
		}
		var intervening int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands cmd ON cmd.command_id=b.command_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence>? AND NOT (cmd.command_type IN ('RPWarmDecision','RPNPCInitiative') AND COALESCE(json_extract(e.payload,'$.trigger_event_id'),'')=?)`, current.InstanceID, current.BranchID, sequence, r.TriggerEventID).Scan(&intervening); err != nil {
			return no, nil, err
		}
		if intervening != 0 {
			return no, nil, core.NewError(core.CodeBranchConflict, "another world action superseded WARM trigger")
		}
		when, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return no, nil, err
		}
		var recent int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWarmDecisionRecorded' AND json_extract(payload,'$.decision.actor_id')=? AND world_time>?`, current.InstanceID, current.BranchID, r.NPCEntityID, when.Add(-core.RPWarmDecisionInterval).Format(time.RFC3339)).Scan(&recent); err != nil {
			return no, nil, err
		}
		if recent != 0 {
			return no, nil, core.NewError(core.CodeBranchConflict, "WARM actor cooldown changed")
		}
		input, err := readRPOwnDecisionContext(ctx, conn, core.RPDecisionInput{InstanceID: current.InstanceID, BranchID: current.BranchID, NPCEntityID: r.NPCEntityID, LegalActions: []string{"wait"}})
		if err != nil {
			return no, nil, err
		}
		var playerPlace string
		if err := conn.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, current.ControlledEntityID).Scan(&playerPlace); err != nil {
			return no, nil, err
		}
		if input.PlaceID == playerPlace {
			return no, nil, core.NewError(core.CodeBranchConflict, "WARM actor is now in current scene")
		}
		own := core.RPWarmDecisionInput{ActorID: r.NPCEntityID, WorldTime: at, PlaceID: input.PlaceID, ActivityCode: input.ActivityCode, NextSchedule: input.NextSchedule, ReachablePlaceIDs: input.ReachablePlaceIDs}
		if next := input.NextSchedule; next != nil && next.ActivityCode == "work" && next.PlaceID != input.PlaceID {
			arrival, err := readRPTransitArrival(ctx, conn, current.InstanceID, current.BranchID, input.PlaceID, next.PlaceID, at)
			if err != nil {
				return no, nil, err
			}
			if arrival.Reachable && arrival.WorldTime == at {
				own.WorkPath = arrival.Path
				for i := 1; i < len(arrival.Path); i++ {
					var source string
					if err := conn.QueryRowContext(ctx, `SELECT definition_event_id FROM rp_place_links WHERE instance_id=? AND branch_id=? AND from_place_id=? AND to_place_id=?`, current.InstanceID, current.BranchID, arrival.Path[i-1], arrival.Path[i]).Scan(&source); err != nil {
						return no, nil, err
					}
					own.RouteSourceEventIDs = append(own.RouteSourceEventIDs, source)
				}
			}
		}
		decision, err := core.ProposeRPWarmDecision(own)
		if err != nil {
			return no, nil, err
		}
		if decision.Action == "leave" && input.Law != nil {
			lawful := false
			for _, action := range input.Law.LawfulActions {
				lawful = lawful || action == "leave"
			}
			if !lawful {
				decision.Action = "wait"
				decision.Reason = "known_law_constraint"
				decision.ToPlaceID = ""
			}
		}
		if err := core.ValidateRPDecisionProposal(input, core.RPDecisionProposal{Action: decision.Action, DestinationPlaceID: decision.ToPlaceID}); err != nil {
			return no, nil, err
		}
		fact := rpWarmFact{Version: "corerp.warm.v1", SessionID: r.SessionID, TriggerEventID: r.TriggerEventID, Selection: selection, Decision: decision, WorkPath: own.WorkPath, RouteSourceEventIDs: own.RouteSourceEventIDs}
		return fact, func() error {
			if decision.Action != "leave" {
				return nil
			}
			var day int64
			if err := conn.QueryRowContext(ctx, `SELECT current_day FROM world_clocks WHERE instance_id=? AND branch_id=?`, current.InstanceID, current.BranchID).Scan(&day); err != nil {
				return err
			}
			return commitRPNPCMovement(ctx, conn, current.InstanceID, current.BranchID, c.EventID, c.Sequence, r.NPCEntityID, decision.FromPlaceID, decision.ToPlaceID, at, day, c.EventID)
		}, nil
	})
}
