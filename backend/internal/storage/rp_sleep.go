package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// Sleep is an actor-specific fact. A world-clock wait or presence at home is
// not, by itself, evidence that the actor slept.
type RPSleepRequest struct {
	Binding   core.CareerBinding `json:"binding"`
	SessionID string             `json:"session_id"`
}

type RPSleepStartFact struct {
	Version  string `json:"version"`
	EntityID string `json:"entity_id"`
	PlaceID  string `json:"place_id"`
}

type RPSleepEndFact struct {
	Version               string `json:"version"`
	EntityID              string `json:"entity_id"`
	StartEventID          string `json:"start_event_id"`
	StartedWorldTime      string `json:"started_world_time"`
	EffectiveEndWorldTime string `json:"effective_end_world_time"`
	RestMinutes           int64  `json:"rest_minutes"`
	Status                string `json:"status"`
	InterruptedByEventID  string `json:"interrupted_by_event_id,omitempty"`
}

type RPSleepStartRecord = privateFactRecord[RPSleepStartFact]
type RPSleepEndRecord = privateFactRecord[RPSleepEndFact]

func validateRPSleepRequest(r RPSleepRequest) error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if !studioID(r.SessionID) || strings.TrimSpace(r.SessionID) != r.SessionID {
		return core.NewError(core.CodeInvalidArgument, "bounded RP sleep session required")
	}
	return nil
}

func rpActorActionAuthorizers(ctx context.Context, binding core.CareerBinding, sessionID string) (func(*sql.Conn) error, func(*sql.Conn) error) {
	replay := func(conn *sql.Conn) error {
		session, err := loadRPSessionRecord(ctx, conn, binding.PrincipalID, sessionID)
		if err != nil {
			return err
		}
		if session.InstanceID != binding.InstanceID || session.BranchID != binding.BranchID {
			return core.NewError(core.CodeNotFound, "RP actor session is outside requested branch")
		}
		return nil
	}
	fresh := func(conn *sql.Conn) error {
		if err := replay(conn); err != nil {
			return err
		}
		session, err := loadRPSessionRecord(ctx, conn, binding.PrincipalID, sessionID)
		if err != nil {
			return err
		}
		if session.Status != "active" || session.ObservationCursor != binding.ExpectedHead {
			return core.NewError(core.CodeBranchConflict, "observe current world before RP actor action")
		}
		if err := requireCurrentRPSession(ctx, conn, session); err != nil {
			return err
		}
		return authorizeRPControl(ctx, conn, binding.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID)
	}
	return replay, fresh
}

func requireSoloRPHealthActionWindow(ctx context.Context, conn *sql.Conn, instance, branch string) error {
	if err := requireNoActiveRPSharedRound(ctx, conn, instance, branch); err != nil {
		return err
	}
	var external int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND status='active'`, instance, branch).Scan(&external); err != nil {
		return err
	}
	if external != 0 {
		return core.NewError(core.CodeCommandInProgress, "RP health/work action requires a shared action window with external residents")
	}
	return nil
}

// Only the exact child pinned by a selected health round may bypass the solo
// guard. The check runs in the same transaction that writes the sleep Event.
func requireRPHealthActionWindow(ctx context.Context, conn *sql.Conn, r RPSleepRequest, kind string) error {
	return requireRPHealthChildWindow(ctx, conn, r.Binding, r.SessionID, r, kind)
}

func requireRPHealthChildWindow(ctx context.Context, conn *sql.Conn, binding core.CareerBinding, sessionID string, request any, kind string) error {
	var roundID, raw string
	var baseline int64
	err := conn.QueryRowContext(ctx, `SELECT r.round_id,r.baseline_head,a.request_json FROM rp_shared_rounds r
		JOIN rp_shared_round_actions a ON a.round_id=r.round_id AND a.session_id=r.selected_session_id
		WHERE r.instance_id=? AND r.branch_id=? AND r.status='advancing' AND r.settlement_kind='health'
		AND r.selected_action_kind=? AND a.action_kind=? AND r.selected_session_id=?`,
		binding.InstanceID, binding.BranchID, kind, kind, sessionID).Scan(&roundID, &baseline, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return requireSoloRPHealthActionWindow(ctx, conn, binding.InstanceID, binding.BranchID)
	}
	if err != nil {
		return err
	}
	if binding.IdempotencyKey != "shared_action_"+roundID || binding.ExpectedHead != baseline {
		return core.NewError(core.CodeUnauthorized, "health action differs from selected shared child")
	}
	pinned, err := core.CanonicalJSON(request)
	if err != nil {
		return err
	}
	if string(pinned) != raw {
		return core.NewError(core.CodeUnauthorized, "health action payload differs from selected shared child")
	}
	return nil
}

func openRPSleep(ctx context.Context, conn *sql.Conn, instance, branch, entity string) (string, string, int64, error) {
	rows, err := conn.QueryContext(ctx, `SELECT e.event_id,e.world_time,e.event_sequence,e.payload FROM events e
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPSleepStarted' AND json_extract(e.payload,'$.entity_id')=?
		AND NOT EXISTS(SELECT 1 FROM events ended WHERE ended.instance_id=e.instance_id AND ended.branch_id=e.branch_id
			AND ended.event_type='RPSleepEnded' AND json_extract(ended.payload,'$.start_event_id')=e.event_id)
		ORDER BY e.event_sequence LIMIT 2`, instance, branch, entity)
	if err != nil {
		return "", "", 0, err
	}
	defer rows.Close()
	var id, at, raw string
	var sequence int64
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", "", 0, err
		}
		return "", "", 0, nil
	}
	if err := rows.Scan(&id, &at, &sequence, &raw); err != nil {
		return "", "", 0, err
	}
	var fact RPSleepStartFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return "", "", 0, err
	}
	if fact.Version != "corerp.sleep.start.v1" || fact.EntityID != entity || fact.PlaceID == "" {
		return "", "", 0, core.NewError(core.CodeProjectionDiverged, "invalid sourced RP sleep start")
	}
	if rows.Next() {
		return "", "", 0, core.NewError(core.CodeProjectionDiverged, "multiple open RP sleep intervals")
	}
	return id, at, sequence, rows.Err()
}

func (s *Store) StartRPSleep(ctx context.Context, r RPSleepRequest) (RPSleepStartRecord, error) {
	if err := validateRPSleepRequest(r); err != nil {
		return RPSleepStartRecord{}, err
	}
	replay, fresh := rpActorActionAuthorizers(ctx, r.Binding, r.SessionID)
	return executePrivateFactCommandWithOptions(s, ctx, r.Binding, "StartRPSleep", r,
		privateFactDomain{"rp_sleep_start", "RPSleepStarted", `{"authorization":"current-actor-sleep-v1"}`},
		privateFactOptions{replayAuthorize: replay}, fresh,
		func(conn *sql.Conn, c privateFactContext) (RPSleepStartFact, func() error, error) {
			var fact RPSleepStartFact
			if err := requireRPHealthActionWindow(ctx, conn, r, "sleep_start"); err != nil {
				return fact, nil, err
			}
			session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return fact, nil, err
			}
			if open, _, _, err := openRPSleep(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, session.ControlledEntityID); err != nil {
				return fact, nil, err
			} else if open != "" {
				return fact, nil, core.NewError(core.CodeBranchConflict, "actor already has an open sleep interval")
			}
			var place string
			err = conn.QueryRowContext(ctx, `SELECT p.place_id FROM agent_positions p JOIN agent_profiles a ON a.agent_id=p.agent_id
				JOIN agent_places l ON l.place_id=p.place_id WHERE p.agent_id=? AND a.instance_id=? AND a.branch_id=?
				AND a.status='active' AND l.status='active' AND l.place_kind='home'`, session.ControlledEntityID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&place)
			if err != nil {
				return fact, nil, classifyMissing(err, "actor at an active home place")
			}
			fact = RPSleepStartFact{Version: "corerp.sleep.start.v1", EntityID: session.ControlledEntityID, PlaceID: place}
			return fact, nil, nil
		})
}

func (s *Store) EndRPSleep(ctx context.Context, r RPSleepRequest) (RPSleepEndRecord, error) {
	if err := validateRPSleepRequest(r); err != nil {
		return RPSleepEndRecord{}, err
	}
	replay, fresh := rpActorActionAuthorizers(ctx, r.Binding, r.SessionID)
	return executePrivateFactCommandWithOptions(s, ctx, r.Binding, "EndRPSleep", r,
		privateFactDomain{"rp_sleep_end", "RPSleepEnded", `{"authorization":"current-actor-sleep-v1"}`},
		privateFactOptions{replayAuthorize: replay}, fresh,
		func(conn *sql.Conn, c privateFactContext) (RPSleepEndFact, func() error, error) {
			var fact RPSleepEndFact
			if err := requireRPHealthActionWindow(ctx, conn, r, "sleep_end"); err != nil {
				return fact, nil, err
			}
			session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return fact, nil, err
			}
			startID, startText, startSequence, err := openRPSleep(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, session.ControlledEntityID)
			if err != nil {
				return fact, nil, err
			}
			if startID == "" {
				return fact, nil, core.NewError(core.CodeNotFound, "open actor sleep interval not found")
			}
			start, err := time.Parse(time.RFC3339Nano, startText)
			if err != nil {
				return fact, nil, err
			}
			end, err := time.Parse(time.RFC3339Nano, c.WorldTime)
			if err != nil || !end.After(start) {
				return fact, nil, core.NewError(core.CodeBranchConflict, "sleep requires positive elapsed world time")
			}
			status := "completed"
			effective := end
			if capAt := start.Add(12 * time.Hour); effective.After(capAt) {
				effective, status = capAt, "capped"
			}
			var interruptedID, interruptedAt string
			err = conn.QueryRowContext(ctx, `SELECT event_id,world_time FROM events WHERE instance_id=? AND branch_id=?
				AND event_sequence>? AND event_sequence<? AND actor_id=? AND event_type IN
				('AgentMoved','AgentActivityStarted','RPPlayerMoved','RPJourneyStarted','RPSpeechAccepted','RPInterpersonalAction','RPNPCDecisionRecorded')
				ORDER BY event_sequence LIMIT 1`, r.Binding.InstanceID, r.Binding.BranchID, startSequence, c.Sequence, session.ControlledEntityID).Scan(&interruptedID, &interruptedAt)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fact, nil, err
			}
			if err == nil {
				at, parseErr := time.Parse(time.RFC3339Nano, interruptedAt)
				if parseErr != nil || at.Before(start) || at.After(end) {
					return fact, nil, core.NewError(core.CodeProjectionDiverged, "sleep interruption chronology differs")
				}
				if !at.After(effective) {
					effective, status = at, "interrupted"
				} else {
					interruptedID = ""
				}
			}
			fact = RPSleepEndFact{Version: "corerp.sleep.end.v1", EntityID: session.ControlledEntityID, StartEventID: startID,
				StartedWorldTime: startText, EffectiveEndWorldTime: effective.UTC().Format(time.RFC3339),
				RestMinutes: int64(effective.Sub(start) / time.Minute), Status: status, InterruptedByEventID: interruptedID}
			return fact, nil, nil
		})
}
