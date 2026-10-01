package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/narrative"
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
	var oldHash string
	var prior RPStyleSetResult
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
		var session RPSession
		session, err = loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
		if err != nil {
			return empty, err
		}
		if session.InstanceID != r.InstanceID || session.BranchID != r.BranchID {
			return empty, core.NewError(core.CodeUnauthorized, "style session scope differs")
		}
		currentErr := requireCurrentRPSession(ctx, tx.conn, session)
		if currentErr == nil {
			if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, r.InstanceID, r.BranchID, session.ControlledEntityID); err != nil {
				return empty, err
			}
		} else if !core.HasCode(currentErr, core.CodeBranchConflict) {
			return empty, currentErr
		}
		// The original session can recover its exact presentation revision
		// after release, but cannot create another revision under stale control.
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
		if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
			return empty, err
		}
		if session.Status != "active" {
			return empty, core.NewError(core.CodeUnauthorized, "style session is closed")
		}
		if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, r.InstanceID, r.BranchID, session.ControlledEntityID); err != nil {
			return empty, err
		}
		controlledEntityID = session.ControlledEntityID
	}
	if r.Scope == "world" {
		err = tx.conn.QueryRowContext(ctx, `SELECT revision_id,revision,request_hash FROM rp_style_revisions WHERE principal_id=? AND idempotency_key=?`, r.PrincipalID, r.IdempotencyKey).Scan(&prior.RevisionID, &prior.Revision, &oldHash)
	}
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
	packages, err := readStudioActivePackages(ctx, conn, session.InstanceID, session.BranchID)
	if err != nil {
		return result, err
	}
	if packages != nil {
		result.Profile = *packages.Narrative.Content.NarrativeStyle
		result.Sources = append(result.Sources, packages.Lock.Narrative.InstallEventID)
	}
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
	var playerTurnID, playerEventID, savedNarrative, savedMode, savedFallback string
	var savedCompositionVersion, savedGroupsJSON, savedFactIDsJSON string
	var savedAt sql.NullString
	if err := tx.conn.QueryRowContext(ctx, `SELECT player_turn_id,player_event_id,narrative_json,narrative_presentation_mode,narrative_presented_at_utc,COALESCE(narrative_fallback,''),narrative_composition_version,narrative_fact_groups_json,narrative_fact_event_ids_json FROM rp_turn_runs WHERE turn_run_id=? AND session_id=? AND status='settled'`, r.TurnRunID, r.SessionID).Scan(&playerTurnID, &playerEventID, &savedNarrative, &savedMode, &savedAt, &savedFallback, &savedCompositionVersion, &savedGroupsJSON, &savedFactIDsJSON); err != nil {
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
	if r.StyleOverride == nil && savedAt.Valid && savedMode != "base" {
		view, err := savedRPOfficialNarrative(ctx, savedNarrative, savedFallback, savedCompositionVersion, savedGroupsJSON, savedFactIDsJSON, input, emit)
		if err != nil {
			return empty, err
		}
		return RPNarrativeReadResult{Style: style, View: view}, nil
	}
	providerMode := "custom"
	if mode, ok := provider.(interface{ NarrativeMode() string }); ok {
		switch mode.NarrativeMode() {
		case "full_prose", "deterministic", "style_planner":
			providerMode = mode.NarrativeMode()
		}
	}
	metadata := rpProviderMetadata(provider, providerMode)
	callID, err := s.beginRPProviderCall(ctx, rpProviderCallScope{SessionID: r.SessionID, TurnRunID: r.TurnRunID, SubjectID: playerEventID, Phase: "narrative"}, metadata)
	if err != nil {
		return empty, err
	}
	var trace core.RPProviderTrace
	var emissionFailed bool
	wrappedEmit := emit
	if emit != nil {
		wrappedEmit = func(chunk core.RPNarrativeChunk) error {
			if err := emit(chunk); err != nil {
				emissionFailed = true
				return err
			}
			return nil
		}
	}
	view, err := provider.RenderStream(core.WithRPProviderTrace(ctx, &trace), input, wrappedEmit)
	if err != nil {
		fallback := ""
		if emissionFailed {
			fallback = "stream_interrupted"
		}
		if recordErr := s.finishRPProviderCall(ctx, callID, rpProviderErrorResult(ctx, err), fallback, "", trace.AttemptCount()); recordErr != nil {
			return empty, recordErr
		}
		return empty, err
	}
	if style.Profile.FullProse && providerMode != "full_prose" {
		// The world declares long-form narrative but no prose-capable
		// provider answered this read. The lines stay the deterministic
		// rendering; the reason is recorded, never silently dropped.
		if view.FallbackReason == "" {
			view.FallbackReason = "world_declares_full_prose_without_prose_provider"
		}
		view.Warnings = append(view.Warnings, "世界声明了 full_prose，但当前叙事 provider 不是长文模式，已按标准叙述呈现。")
	}
	// A provider's fallback string is not trusted presentation metadata.
	// Never persist a raw remote error, URL or response in the receipt.
	fallback := sanitizeRPNarrativeFallback(view.FallbackReason)
	view.FallbackReason = fallback
	if fallback != "" {
		if recordErr := s.recordRPNarrativeFallback(ctx, r.TurnRunID, fallback, providerMode, input); recordErr != nil {
			return empty, recordErr
		}
	} else {
		if err := s.saveSelectedRPNarrative(ctx, r, input, &view, metadata); err != nil {
			_ = s.finishRPProviderCall(ctx, callID, "failed", "render_persist", "", trace.AttemptCount())
			return empty, err
		}
	}
	source := "template"
	if providerMode == "full_prose" && fallback == "" {
		source = "live_prose"
	}
	result := "success"
	if strings.HasPrefix(fallback, "prose_") {
		result = "failed"
		if fallback == "prose_timeout" {
			result = "timeout"
		}
	}
	if err := s.finishRPProviderCall(ctx, callID, result, fallback, source, trace.AttemptCount()); err != nil {
		return empty, err
	}
	if r.StyleOverride == nil {
		if err := s.saveRPOfficialNarrative(ctx, r.TurnRunID, providerMode, view); err != nil {
			return empty, err
		}
	}
	return RPNarrativeReadResult{Style: style, View: view}, nil
}

func savedRPOfficialNarrative(ctx context.Context, encoded, fallback, compositionVersion, groupsJSON, factIDsJSON string, input core.RPNarrativeInput, emit func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
	var lines []string
	if err := json.Unmarshal([]byte(encoded), &lines); err != nil || len(lines) == 0 || len(input.Facts) == 0 {
		return core.RPNarrativeView{}, core.NewError(core.CodeProjectionDiverged, "saved RP narrative is invalid")
	}
	view := core.RPNarrativeView{Lines: lines, EventIDs: make([]string, 0, len(input.Facts)), Warnings: []string{}}
	for _, fact := range input.Facts {
		view.EventIDs = append(view.EventIDs, fact.EventID)
	}
	var err error
	view.CompositionVersion = compositionVersion
	view.FactGroups, err = decodeRPNarrativeComposition(compositionVersion, groupsJSON, factIDsJSON, lines)
	if err != nil {
		return core.RPNarrativeView{}, err
	}
	if err := validateRPNarrativeComposition(view); err != nil {
		return core.RPNarrativeView{}, err
	}
	if compositionVersion == narrative.CompositionVersion {
		view.Warnings = append(view.Warnings, narrative.CompositionCapabilityWarning)
	}
	if fallback != "" {
		var record struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal([]byte(fallback), &record) == nil {
			view.FallbackReason = record.Reason
		}
		view.Warnings = append(view.Warnings, "小说式呈现暂不可用或正文未通过事实校验，已回退为标准叙述。")
	}
	for i, line := range lines {
		if err := ctx.Err(); err != nil {
			return core.RPNarrativeView{}, err
		}
		if emit == nil {
			continue
		}
		source := i
		if source >= len(input.Facts) {
			source = len(input.Facts) - 1
		}
		chunk := core.RPNarrativeChunk{Index: i, EventID: input.Facts[source].EventID, Line: line}
		if compositionVersion != "" {
			chunk.EventIDs = append([]string(nil), view.FactGroups[i]...)
			chunk.EventID = ""
		}
		if err := emit(chunk); err != nil {
			return core.RPNarrativeView{}, err
		}
	}
	return view, nil
}

func (s *Store) saveRPOfficialNarrative(ctx context.Context, turnRunID, providerMode string, view core.RPNarrativeView) error {
	if err := validateRPNarrativeComposition(view); err != nil {
		return err
	}
	groups := view.FactGroups
	if groups == nil {
		groups = [][]string{}
	}
	groupsJSON, err := json.Marshal(groups)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "encode canonical narrative composition", err)
	}
	factIDsJSON, err := json.Marshal(view.EventIDs)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "encode canonical narrative fact sources", err)
	}
	mode := providerMode
	switch mode {
	case "deterministic", "style_planner", "full_prose":
	default:
		mode = "custom"
	}
	encoded, err := core.CanonicalJSON(view.Lines)
	if err != nil {
		return err
	}
	fallbackAssignment := "narrative_fallback"
	if view.FallbackReason == "" {
		fallbackAssignment = "NULL"
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	query := `UPDATE rp_turn_runs SET narrative_json=?,narrative_presentation_mode=?,narrative_presented_at_utc=?,updated_at_utc=?,narrative_composition_version=?,narrative_fact_groups_json=?,narrative_fact_event_ids_json=?,narrative_fallback=` + fallbackAssignment + ` WHERE turn_run_id=? AND status='settled' AND narrative_presented_at_utc IS NULL`
	if _, err := s.db.ExecContext(ctx, query, string(encoded), mode, now, now, view.CompositionVersion, string(groupsJSON), string(factIDsJSON), turnRunID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "save official RP narrative", err)
	}
	return nil
}

func sanitizeRPNarrativeFallback(reason string) string {
	if reason == "" || reason == "world_declares_full_prose_without_prose_provider" {
		return reason
	}
	if strings.HasPrefix(reason, "prose_") {
		switch {
		case strings.Contains(reason, "timeout"), strings.Contains(reason, "cancellation"):
			return "prose_timeout"
		case strings.Contains(reason, "invalid composition plan"), strings.Contains(reason, "composition fact coverage mismatch"), strings.Contains(reason, "invalid composition template"):
			return "prose_validation_failure"
		case strings.Contains(reason, "speech"), strings.Contains(reason, "dialogue"), strings.Contains(reason, "forbidden"), strings.Contains(reason, "prose empty"), strings.Contains(reason, "agency"), strings.Contains(reason, "expression in prose"), strings.Contains(reason, "money claim"), strings.Contains(reason, "time change"), strings.Contains(reason, "object in prose"), strings.Contains(reason, "person in prose"), strings.Contains(reason, "name in prose"), strings.Contains(reason, "NPC action"), strings.Contains(reason, "relationship"), strings.Contains(reason, "quotation"):
			return "prose_validation_failure"
		default:
			return "prose_unavailable"
		}
	}
	return "narrative_provider_fallback"
}

// recordRPNarrativeFallback persists why the displayed narrative fell back to
// the deterministic renderer. Presentation metadata only: it never influences
// facts, settlement or later reads.
func (s *Store) recordRPNarrativeFallback(ctx context.Context, turnRunID, reason, providerMode string, input core.RPNarrativeInput) error {
	factEvents := make([]string, 0, len(input.Facts))
	for i, fact := range input.Facts {
		if i >= 8 {
			break
		}
		factEvents = append(factEvents, fact.EventID)
	}
	payload, err := core.CanonicalJSON(struct {
		Reason       string   `json:"reason"`
		ProviderMode string   `json:"provider_mode"`
		FactCount    int      `json:"fact_count"`
		FactEvents   []string `json:"fact_events"`
		RecordedAt   string   `json:"recorded_at_utc"`
	}{reason, providerMode, len(input.Facts), factEvents, s.now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	if err := execAgentOne(ctx, s.db, "record narrative fallback", `UPDATE rp_turn_runs SET narrative_fallback=? WHERE turn_run_id=?`, string(payload), turnRunID); err != nil {
		return err
	}
	return nil
}
