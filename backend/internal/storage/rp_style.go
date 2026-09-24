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

type RPStyleSetRequest struct {
	PrincipalID      string            `json:"principal_id"`
	InstanceID       string            `json:"instance_id"`
	BranchID         string            `json:"branch_id"`
	Scope            string            `json:"scope"`
	SessionID        string            `json:"session_id,omitempty"`
	PlaceID          string            `json:"place_id,omitempty"`
	ExpectedRevision int64             `json:"expected_revision"`
	IdempotencyKey   string            `json:"idempotency_key"`
	Patch            core.RPStylePatch `json:"patch"`
}
type RPStyleSetResult struct {
	RevisionID string `json:"revision_id"`
	Revision   int64  `json:"revision"`
	Replayed   bool   `json:"replayed"`
}
type RPResolvedStyle struct {
	Profile core.RPStyleProfile `json:"profile"`
	Sources []string            `json:"sources"`
	// Read metadata only; absent from pinned-turn and narrative variant styles.
	SessionRevision *int64 `json:"session_revision,omitempty"`
}

func styleBindingID(instance, branch, scope, session, place string) (string, error) {
	h, err := core.HashJSON([]string{instance, branch, scope, session, place})
	return "style_binding_" + h, err
}

// Style settings change application presentation, never the branch head or
// committed facts. World scope is available to existing scoped creators only.
func (s *Store) SetRPStyle(ctx context.Context, r RPStyleSetRequest) (RPStyleSetResult, error) {
	var empty RPStyleSetResult
	if strings.TrimSpace(r.PrincipalID) == "" || r.InstanceID == "" || r.BranchID == "" || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 || r.ExpectedRevision < 0 || r.ExpectedRevision >= core.MaxJSONSafeInteger {
		return empty, core.NewError(core.CodeInvalidArgument, "invalid style binding/revision")
	}
	if r.Scope != "world" && r.Scope != "session" && r.Scope != "scene" {
		return empty, core.NewError(core.CodeInvalidArgument, "style scope must be world/session/scene")
	}
	if (r.Scope == "world" && (r.SessionID != "" || r.PlaceID != "")) || (r.Scope != "world" && r.SessionID == "") || (r.Scope == "scene" && r.PlaceID == "") || (r.Scope == "session" && r.PlaceID != "") {
		return empty, core.NewError(core.CodeInvalidArgument, "inconsistent style scope target")
	}
	if _, err := core.ResolveRPStyle(r.Patch); err != nil {
		return empty, err
	}
	hash, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	var controlledEntityID string
	if r.Scope == "world" {
		var allowed int
		err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g JOIN principals p ON p.principal_id=g.principal_id WHERE p.principal_id=? AND p.principal_type='creator' AND p.status='active' AND g.instance_id=? AND g.branch_id=? AND g.status='active' AND g.capability_id IN ('world.cohort.materialize','world.simulate')`, r.PrincipalID, r.InstanceID, r.BranchID).Scan(&allowed)
		if err != nil {
			return empty, err
		}
		if allowed == 0 {
			return empty, core.NewError(core.CodeUnauthorized, "world presentation requires scoped creator write authority")
		}
	} else {
		session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
		if err != nil {
			return empty, err
		}
		if session.InstanceID != r.InstanceID || session.BranchID != r.BranchID || session.Status != "active" {
			return empty, core.NewError(core.CodeUnauthorized, "style session scope differs")
		}
		if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, r.InstanceID, r.BranchID, session.ControlledEntityID); err != nil {
			return empty, err
		}
		controlledEntityID = session.ControlledEntityID
	}
	var oldHash string
	var prior RPStyleSetResult
	err = tx.conn.QueryRowContext(ctx, `SELECT revision_id,revision,request_hash FROM rp_style_revisions WHERE principal_id=? AND idempotency_key=?`, r.PrincipalID, r.IdempotencyKey).Scan(&prior.RevisionID, &prior.Revision, &oldHash)
	if err == nil {
		if hash != oldHash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "style retry differs")
		}
		prior.Replayed = true
		return prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if r.Scope == "scene" {
		var present int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_positions p JOIN agent_places l ON l.place_id=p.place_id WHERE p.agent_id=? AND p.place_id=? AND l.instance_id=? AND l.branch_id=? AND l.status='active'`, controlledEntityID, r.PlaceID, r.InstanceID, r.BranchID).Scan(&present); err != nil {
			return empty, err
		}
		if present == 0 {
			return empty, core.NewError(core.CodeUnauthorized, "scene style requires the controlled character's current place")
		}
	}
	binding, err := styleBindingID(r.InstanceID, r.BranchID, r.Scope, r.SessionID, r.PlaceID)
	if err != nil {
		return empty, err
	}
	var revision int64
	err = tx.conn.QueryRowContext(ctx, `SELECT r.revision FROM rp_style_bindings b JOIN rp_style_revisions r ON r.revision_id=b.revision_id WHERE b.binding_id=?`, binding).Scan(&revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if revision != r.ExpectedRevision {
		return empty, core.NewError(core.CodeBranchConflict, "style revision is stale")
	}
	encoded, err := core.CanonicalJSON(r.Patch)
	if err != nil {
		return empty, err
	}
	revisionID := "style_revision_" + hash[7:]
	if err := execAgentOne(ctx, tx.conn, "style revision", `INSERT INTO rp_style_revisions(revision_id,binding_id,revision,principal_id,idempotency_key,request_hash,patch_json,created_at_utc) VALUES (?,?,?,?,?,?,?,?)`, revisionID, binding, revision+1, r.PrincipalID, r.IdempotencyKey, hash, string(encoded), s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return empty, err
	}
	_, err = tx.conn.ExecContext(ctx, `INSERT INTO rp_style_bindings(binding_id,instance_id,branch_id,scope,session_id,place_id,revision_id) VALUES (?,?,?,?,NULLIF(?,''),NULLIF(?,''),?) ON CONFLICT(binding_id) DO UPDATE SET revision_id=excluded.revision_id`, binding, r.InstanceID, r.BranchID, r.Scope, r.SessionID, r.PlaceID, revisionID)
	if err != nil {
		return empty, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return RPStyleSetResult{RevisionID: revisionID, Revision: revision + 1}, nil
}

func resolveRPStyle(ctx context.Context, conn *sql.Conn, session RPSession, override *core.RPStylePatch) (RPResolvedStyle, error) {
	result := RPResolvedStyle{Profile: core.DefaultRPStyle(), Sources: []string{}}
	// The legacy session POV supplies an unconfigured default; explicit new
	// world/session/scene layers then follow their documented precedence.
	result.Profile.POV = session.POV
	var place string
	if err := conn.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, session.ControlledEntityID).Scan(&place); err != nil {
		return result, err
	}
	for _, scope := range []string{"world", "session", "scene"} {
		sessionID, placeID := "", ""
		if scope != "world" {
			sessionID = session.SessionID
		}
		if scope == "scene" {
			placeID = place
		}
		binding, err := styleBindingID(session.InstanceID, session.BranchID, scope, sessionID, placeID)
		if err != nil {
			return result, err
		}
		var revisionID, raw string
		err = conn.QueryRowContext(ctx, `SELECT r.revision_id,r.patch_json FROM rp_style_bindings b JOIN rp_style_revisions r ON r.revision_id=b.revision_id WHERE b.binding_id=?`, binding).Scan(&revisionID, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return result, err
		}
		var patch core.RPStylePatch
		if err := json.Unmarshal([]byte(raw), &patch); err != nil {
			return result, err
		}
		result.Profile, err = core.OverlayRPStyle(result.Profile, patch)
		if err != nil {
			return result, err
		}
		result.Sources = append(result.Sources, revisionID)
	}
	if override != nil {
		var err error
		result.Profile, err = core.OverlayRPStyle(result.Profile, *override)
		if err != nil {
			return result, err
		}
	}
	return result, result.Profile.Validate()
}

func pinRPTurnStyle(ctx context.Context, conn *sql.Conn, runID string, session RPSession, override *core.RPStylePatch) error {
	resolved, err := resolveRPStyle(ctx, conn, session, override)
	if err != nil {
		return err
	}
	profile, err := core.CanonicalJSON(resolved.Profile)
	if err != nil {
		return err
	}
	sources, err := core.CanonicalJSON(resolved.Sources)
	if err != nil {
		return err
	}
	return execAgentOne(ctx, conn, "pin turn style", `INSERT INTO rp_turn_styles(turn_run_id,profile_json,sources_json) VALUES (?,?,?)`, runID, string(profile), string(sources))
}

func (s *Store) ReadRPStyle(ctx context.Context, r core.RPSessionReadRequest) (RPResolvedStyle, error) {
	if err := r.Validate(); err != nil {
		return RPResolvedStyle{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPResolvedStyle{}, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return RPResolvedStyle{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPResolvedStyle{}, err
	}
	result, err := resolveRPStyle(ctx, tx.conn, session, nil)
	if err != nil {
		return RPResolvedStyle{}, err
	}
	binding, err := styleBindingID(session.InstanceID, session.BranchID, "session", session.SessionID, "")
	if err != nil {
		return RPResolvedStyle{}, err
	}
	var revision int64
	err = tx.conn.QueryRowContext(ctx, `SELECT r.revision FROM rp_style_bindings b JOIN rp_style_revisions r ON r.revision_id=b.revision_id WHERE b.binding_id=?`, binding).Scan(&revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RPResolvedStyle{}, err
	}
	result.SessionRevision = &revision
	return result, nil
}

func (s *Store) loadRPTurnStyle(ctx context.Context, runID string) (RPResolvedStyle, error) {
	var profile, sources string
	var result RPResolvedStyle
	if err := s.db.QueryRowContext(ctx, `SELECT profile_json,sources_json FROM rp_turn_styles WHERE turn_run_id=?`, runID).Scan(&profile, &sources); err != nil {
		return result, classifyMissing(err, "pinned turn style")
	}
	if err := json.Unmarshal([]byte(profile), &result.Profile); err != nil {
		return result, err
	}
	if err := json.Unmarshal([]byte(sources), &result.Sources); err != nil {
		return result, err
	}
	return result, result.Profile.Validate()
}

type RPNarrativeReadRequest struct {
	PrincipalID   string             `json:"principal_id"`
	SessionID     string             `json:"session_id"`
	TurnRunID     string             `json:"turn_run_id"`
	StyleOverride *core.RPStylePatch `json:"style_override,omitempty"`
}
type RPNarrativeReadResult struct {
	Style RPResolvedStyle      `json:"style"`
	View  core.RPNarrativeView `json:"view"`
}

// Render a variant of the same settled facts. No retry of the decision model,
// no transcript rewrite, and no rollback/command/world mutation.
func (s *Store) ReadRPNarrative(ctx context.Context, r RPNarrativeReadRequest) (RPNarrativeReadResult, error) {
	return s.StreamRPNarrative(ctx, r, nil)
}

// Authorization and immutable input gathering finish before the first emission.
// No database connection is retained while the client consumes the stream.
func (s *Store) StreamRPNarrative(ctx context.Context, r RPNarrativeReadRequest, emit func(core.RPNarrativeChunk) error) (RPNarrativeReadResult, error) {
	return s.streamRPNarrativeWithProvider(ctx, r, emit, core.DeterministicRPNarrativeProvider{})
}

func (s *Store) streamRPNarrativeWithProvider(ctx context.Context, r RPNarrativeReadRequest, emit func(core.RPNarrativeChunk) error, provider core.RPStreamingNarrativeProvider) (RPNarrativeReadResult, error) {
	var empty RPNarrativeReadResult
	if r.TurnRunID == "" {
		return empty, core.NewError(core.CodeInvalidArgument, "turn_run_id required")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return empty, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	var playerTurnID, playerEventID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT player_turn_id,player_event_id FROM rp_turn_runs WHERE turn_run_id=? AND session_id=? AND status='settled'`, r.TurnRunID, r.SessionID).Scan(&playerTurnID, &playerEventID); err != nil {
		return empty, classifyMissing(err, "settled own turn narrative")
	}
	tx.Rollback(ctx)
	style, err := s.loadRPTurnStyle(ctx, r.TurnRunID)
	if err != nil {
		return empty, err
	}
	if r.StyleOverride != nil {
		style.Profile, err = core.OverlayRPStyle(style.Profile, *r.StyleOverride)
		if err != nil {
			return empty, err
		}
	}
	input, err := s.readRPNarrativeInput(ctx, r.SessionID, playerTurnID, playerEventID)
	if err != nil {
		return empty, err
	}
	input.Style = style.Profile
	if err := input.ValidateReadBudget(); err != nil {
		return empty, err
	}
	view, err := provider.RenderStream(ctx, input, emit)
	return RPNarrativeReadResult{Style: style, View: view}, err
}
