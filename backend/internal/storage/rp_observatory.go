package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"corerp.local/backend/internal/core"
)

type RPObservatoryRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	BeforeSequence int64  `json:"before_sequence,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

type RPObservatoryProjectionHealth struct {
	Status          string `json:"status"`
	DifferenceCount int    `json:"difference_count"`
	CheckedThrough  int64  `json:"checked_through_sequence"`
}

type RPObservatoryBackgroundHealth struct {
	Enabled             bool   `json:"enabled"`
	Status              string `json:"status"`
	LastTargetWorldTime string `json:"last_target_world_time,omitempty"`
	ProcessedItems      int    `json:"processed_items,omitempty"`
	UpdatedAtUTC        string `json:"updated_at_utc,omitempty"`
}

type RPObservatoryActivation struct {
	ActorRef       string `json:"actor_ref"`
	DisplayName    string `json:"display_name"`
	Disposition    string `json:"disposition"`
	ReasonCode     string `json:"reason_code"`
	ActivationRank *int   `json:"activation_rank,omitempty"`
}

type RPObservatoryAction struct {
	ActorRef    string `json:"actor_ref"`
	DisplayName string `json:"display_name"`
	Action      string `json:"action"`
	Detail      string `json:"detail,omitempty"`
	PlaceName   string `json:"place_name,omitempty"`
	Activity    string `json:"activity,omitempty"`
}

type RPObservatoryFallback struct {
	Used         bool   `json:"used"`
	ReasonCode   string `json:"reason_code,omitempty"`
	ProviderMode string `json:"provider_mode,omitempty"`
}

type RPObservatoryTrace struct {
	TraceID           string                    `json:"trace_id"`
	Sequence          int64                     `json:"sequence"`
	WorldTime         string                    `json:"world_time,omitempty"`
	PlaceName         string                    `json:"place_name,omitempty"`
	Stage             string                    `json:"stage"`
	ExecutionMode     string                    `json:"execution_mode"`
	ResponderLimit    int                       `json:"responder_limit"`
	PlayerAction      string                    `json:"player_action,omitempty"`
	Activations       []RPObservatoryActivation `json:"activations"`
	CommittedActions  []RPObservatoryAction     `json:"committed_actions"`
	NarrativeFallback RPObservatoryFallback     `json:"narrative_fallback"`
}

type RPObservatoryView struct {
	WorldTime          string                        `json:"world_time"`
	CurrentPlace       string                        `json:"current_place"`
	Traces             []RPObservatoryTrace          `json:"traces"`
	NextBeforeSequence int64                         `json:"next_before_sequence,omitempty"`
	ProjectionHealth   RPObservatoryProjectionHealth `json:"projection_health"`
	BackgroundHealth   RPObservatoryBackgroundHealth `json:"background_health"`
}

func (r RPObservatoryRequest) validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || r.BeforeSequence < 0 || r.Limit < 0 || r.Limit > 50 {
		return core.NewError(core.CodeInvalidArgument, "principal_id, session_id and bounded observatory pagination are required")
	}
	return nil
}

func (s *Store) ReadRPObservatory(ctx context.Context, request RPObservatoryRequest) (RPObservatoryView, error) {
	if err := request.validate(); err != nil {
		return RPObservatoryView{}, err
	}
	limit := request.Limit
	if limit == 0 {
		limit = 20
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return RPObservatoryView{}, core.WrapError(core.CodeStorageFailure, "acquire observatory connection", err)
	}
	session, err := loadRPSession(ctx, conn, request.PrincipalID, request.SessionID)
	if err != nil {
		conn.Close()
		return RPObservatoryView{}, err
	}
	var head int64
	view := RPObservatoryView{Traces: make([]RPObservatoryTrace, 0, limit)}
	if err := conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,l.display_name
		FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
		JOIN agent_positions p ON p.agent_id=? JOIN agent_places l ON l.place_id=p.place_id
		WHERE b.instance_id=? AND b.branch_id=? AND p.agent_id=? AND l.instance_id=b.instance_id AND l.branch_id=b.branch_id`,
		session.ControlledEntityID, session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(&head, &view.WorldTime, &view.CurrentPlace); err != nil {
		conn.Close()
		return RPObservatoryView{}, classifyMissing(err, "observatory world")
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT r.turn_run_id,r.status,r.execution_mode,r.responder_limit,
		       COALESCE(r.settled_sequence,e.event_sequence,0),COALESCE(e.world_time,''),
		       COALESCE(l.display_name,''),COALESCE(u.speech_text,''),COALESCE(r.narrative_fallback,''),
		       COALESCE(r.player_turn_id,'')
		FROM rp_turn_runs r
		LEFT JOIN events e ON e.event_id=r.player_event_id
		LEFT JOIN rp_utterances u ON u.turn_id=r.player_turn_id AND u.session_id=r.session_id
		LEFT JOIN agent_places l ON l.place_id=u.place_id
		WHERE r.session_id=? AND (?=0 OR COALESCE(r.settled_sequence,e.event_sequence,0)<?)
		ORDER BY CASE WHEN e.event_sequence IS NULL THEN ? ELSE COALESCE(r.settled_sequence,e.event_sequence) END DESC,r.created_at_utc DESC
		LIMIT ?`, session.SessionID, request.BeforeSequence, request.BeforeSequence, head+1, limit+1)
	if err != nil {
		conn.Close()
		return RPObservatoryView{}, core.WrapError(core.CodeStorageFailure, "read observatory turns", err)
	}
	type rawTrace struct {
		turnID, fallback, playerTurnID string
		trace                          RPObservatoryTrace
	}
	raw := make([]rawTrace, 0, limit+1)
	for rows.Next() {
		item := rawTrace{trace: RPObservatoryTrace{Activations: []RPObservatoryActivation{}, CommittedActions: []RPObservatoryAction{}}}
		if err := rows.Scan(&item.turnID, &item.trace.Stage, &item.trace.ExecutionMode, &item.trace.ResponderLimit, &item.trace.Sequence, &item.trace.WorldTime, &item.trace.PlaceName, &item.trace.PlayerAction, &item.fallback, &item.playerTurnID); err != nil {
			rows.Close()
			conn.Close()
			return RPObservatoryView{}, core.WrapError(core.CodeStorageFailure, "scan observatory turn", err)
		}
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		conn.Close()
		return RPObservatoryView{}, core.WrapError(core.CodeStorageFailure, "iterate observatory turns", err)
	}
	rows.Close()
	if len(raw) > limit {
		view.NextBeforeSequence = raw[limit-1].trace.Sequence
		raw = raw[:limit]
	}
	labels := map[string]string{}
	if packages, err := readStudioActivePackages(ctx, conn, session.InstanceID, session.BranchID); err != nil {
		conn.Close()
		return RPObservatoryView{}, err
	} else if packages != nil {
		labels = packages.Narrative.Content.ActivityLabels
		progression := packages.System.Content.SystemRules.BackgroundProgression
		if progression != nil && progression.Enabled {
			view.BackgroundHealth.Enabled = true
			view.BackgroundHealth.Status = "idle"
			var finished sql.NullString
			err := conn.QueryRowContext(ctx, `SELECT status,target_world_time,processed_items,COALESCE(finished_at_utc,started_at_utc) FROM rp_background_runs WHERE instance_id=? AND branch_id=? ORDER BY started_at_utc DESC LIMIT 1`, session.InstanceID, session.BranchID).Scan(&view.BackgroundHealth.Status, &view.BackgroundHealth.LastTargetWorldTime, &view.BackgroundHealth.ProcessedItems, &finished)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				conn.Close()
				return RPObservatoryView{}, core.WrapError(core.CodeStorageFailure, "read observatory background health", err)
			}
			if finished.Valid {
				view.BackgroundHealth.UpdatedAtUTC = finished.String
			}
		}
	}
	if view.BackgroundHealth.Status == "" {
		view.BackgroundHealth.Status = "disabled"
	}
	for _, item := range raw {
		trace := item.trace
		trace.TraceID, err = rpAnonymousReference(ctx, conn, "trace", session.InstanceID, session.BranchID, session.ControlledEntityID, item.turnID)
		if err != nil {
			conn.Close()
			return RPObservatoryView{}, err
		}
		trace.NarrativeFallback = observatoryFallback(item.fallback)
		trace.Activations, err = readRPObservatoryActivations(ctx, conn, session, item.turnID, trace.WorldTime)
		if err != nil {
			conn.Close()
			return RPObservatoryView{}, err
		}
		if item.playerTurnID != "" {
			trace.CommittedActions, err = readRPObservatoryActions(ctx, conn, session, item.playerTurnID, trace.WorldTime, labels)
			if err != nil {
				conn.Close()
				return RPObservatoryView{}, err
			}
		}
		view.Traces = append(view.Traces, trace)
	}
	if err := conn.Close(); err != nil {
		return RPObservatoryView{}, core.WrapError(core.CodeStorageFailure, "close observatory connection", err)
	}
	differences, err := s.CompareProjections(ctx, session.InstanceID, session.BranchID)
	if err != nil {
		return RPObservatoryView{}, err
	}
	view.ProjectionHealth = RPObservatoryProjectionHealth{Status: "healthy", DifferenceCount: len(differences), CheckedThrough: head}
	if len(differences) > 0 {
		view.ProjectionHealth.Status = "degraded"
	}
	return view, nil
}

func readRPObservatoryActivations(ctx context.Context, conn *sql.Conn, session RPSession, turnID, worldTime string) ([]RPObservatoryActivation, error) {
	rows, err := conn.QueryContext(ctx, `SELECT npc_entity_id,disposition,reason_code,activation_rank FROM rp_turn_listener_activations WHERE turn_run_id=? ORDER BY CASE WHEN activation_rank IS NULL THEN 99 ELSE activation_rank END,npc_entity_id`, turnID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read observatory activations", err)
	}
	type rawActivation struct {
		actorID                 string
		disposition, reasonCode string
		rank                    sql.NullInt64
	}
	raw := make([]rawActivation, 0)
	for rows.Next() {
		var item rawActivation
		if err := rows.Scan(&item.actorID, &item.disposition, &item.reasonCode, &item.rank); err != nil {
			rows.Close()
			return nil, err
		}
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]RPObservatoryActivation, 0, len(raw))
	for _, source := range raw {
		item := RPObservatoryActivation{Disposition: source.disposition, ReasonCode: source.reasonCode}
		item.ActorRef, item.DisplayName, err = rpObservatoryActor(ctx, conn, session, source.actorID, worldTime)
		if err != nil {
			return nil, err
		}
		if source.rank.Valid {
			value := int(source.rank.Int64)
			item.ActivationRank = &value
		}
		out = append(out, item)
	}
	return out, nil
}

func readRPObservatoryActions(ctx context.Context, conn *sql.Conn, session RPSession, playerTurnID, worldTime string, labels map[string]string) ([]RPObservatoryAction, error) {
	rows, err := conn.QueryContext(ctx, `SELECT d.npc_entity_id,d.action,d.proposal_json FROM rp_npc_decisions d JOIN events e ON e.event_id=d.event_id WHERE d.session_id=? AND d.parent_turn_id=? ORDER BY e.event_sequence`, session.SessionID, playerTurnID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read observatory committed actions", err)
	}
	type rawAction struct{ actorID, action, proposal string }
	rawActions := make([]rawAction, 0)
	for rows.Next() {
		var item rawAction
		if err := rows.Scan(&item.actorID, &item.action, &item.proposal); err != nil {
			rows.Close()
			return nil, err
		}
		rawActions = append(rawActions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]RPObservatoryAction, 0, len(rawActions))
	for _, source := range rawActions {
		item := RPObservatoryAction{Action: source.action}
		item.ActorRef, item.DisplayName, err = rpObservatoryActor(ctx, conn, session, source.actorID, worldTime)
		if err != nil {
			return nil, err
		}
		var proposal core.RPDecisionProposal
		if err := json.Unmarshal([]byte(source.proposal), &proposal); err != nil {
			return nil, core.WrapError(core.CodeProjectionDiverged, "decode committed observatory action", err)
		}
		switch item.Action {
		case "respond", "refuse":
			item.Detail = proposal.Text
		case "leave":
			if err := conn.QueryRowContext(ctx, `SELECT display_name FROM agent_places WHERE instance_id=? AND branch_id=? AND place_id=?`, session.InstanceID, session.BranchID, proposal.DestinationPlaceID).Scan(&item.PlaceName); err != nil {
				return nil, classifyMissing(err, "observatory action place")
			}
		case "act":
			item.Activity = labels[proposal.ActivityCode]
			if item.Activity == "" {
				item.Activity = proposal.ActivityCode
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func rpObservatoryActor(ctx context.Context, conn *sql.Conn, session RPSession, actorID, worldTime string) (string, string, error) {
	ref, err := rpAnonymousReference(ctx, conn, "actor", session.InstanceID, session.BranchID, session.ControlledEntityID, actorID)
	if err != nil {
		return "", "", err
	}
	known := actorID == session.ControlledEntityID
	if !known {
		var count int
		query := `SELECT COUNT(*) FROM rp_identity_familiarity WHERE instance_id=? AND branch_id=? AND observer_agent_id=? AND subject_agent_id=?`
		args := []any{session.InstanceID, session.BranchID, session.ControlledEntityID, actorID}
		if worldTime != "" {
			query += ` AND learned_world_time<=?`
			args = append(args, worldTime)
		}
		if err := conn.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return "", "", err
		}
		known = count > 0
	}
	if !known {
		return ref, "陌生人", nil
	}
	var name string
	if err := conn.QueryRowContext(ctx, `SELECT display_name FROM materialized_entities WHERE entity_id=?`, actorID).Scan(&name); err != nil {
		return "", "", classifyMissing(err, "observatory actor")
	}
	return ref, name, nil
}

func observatoryFallback(raw string) RPObservatoryFallback {
	if strings.TrimSpace(raw) == "" {
		return RPObservatoryFallback{}
	}
	var record struct {
		Reason       string `json:"reason"`
		ProviderMode string `json:"provider_mode"`
	}
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return RPObservatoryFallback{Used: true, ReasonCode: "record_unreadable"}
	}
	reason := "deterministic_fallback"
	switch {
	case strings.Contains(record.Reason, "without_prose_provider"):
		reason = "provider_unavailable"
	case strings.HasPrefix(record.Reason, "prose_"):
		reason = "provider_error"
	case strings.Contains(record.Reason, "invalid"), strings.Contains(record.Reason, "fact"):
		reason = "presentation_rejected"
	}
	return RPObservatoryFallback{Used: true, ReasonCode: reason, ProviderMode: record.ProviderMode}
}
