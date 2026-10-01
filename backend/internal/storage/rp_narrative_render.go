package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/narrative"
)

type RPNarrativeSelectRequest struct {
	PrincipalID string `json:"principal_id"`
	SessionID   string `json:"session_id"`
	TurnRunID   string `json:"turn_run_id"`
	// Empty chooses the canonical, pinned turn narration.
	RenderID string `json:"render_id"`
}

type RPNarrativeSelectResult struct {
	RenderID           string     `json:"render_id,omitempty"`
	Lines              []string   `json:"lines"`
	CompositionVersion string     `json:"composition_version,omitempty"`
	FactGroups         [][]string `json:"fact_groups,omitempty"`
}

const rpNarrativeCompositionVersion = narrative.CompositionVersion

// Validate the receipt against facts, never against the broader presentation
// provenance list (which may also contain authored style/cue sources).
func validateRPNarrativeComposition(view core.RPNarrativeView) error {
	if view.CompositionVersion == "" {
		if len(view.FactGroups) != 0 {
			return core.NewError(core.CodeProjectionDiverged, "unversioned narrative has composition groups")
		}
		return nil // Saved legacy prose does not claim closed composition.
	}
	if view.CompositionVersion != rpNarrativeCompositionVersion || len(view.FactGroups) == 0 || len(view.FactGroups) != len(view.Lines) {
		return core.NewError(core.CodeProjectionDiverged, "narrative composition version or groups are invalid")
	}
	next := 0
	seen := make(map[string]bool, len(view.EventIDs))
	for _, group := range view.FactGroups {
		if len(group) == 0 {
			return core.NewError(core.CodeProjectionDiverged, "narrative composition group is empty")
		}
		for _, id := range group {
			if id == "" || seen[id] || next >= len(view.EventIDs) || id != view.EventIDs[next] {
				return core.NewError(core.CodeProjectionDiverged, "narrative composition source order differs from committed facts")
			}
			seen[id] = true
			next++
		}
	}
	if next != len(view.EventIDs) {
		return core.NewError(core.CodeProjectionDiverged, "narrative composition omits committed facts")
	}
	return nil
}

func decodeRPNarrativeComposition(version, groupsJSON, factIDsJSON string, lines []string) ([][]string, error) {
	var groups [][]string
	if err := json.Unmarshal([]byte(groupsJSON), &groups); err != nil {
		return nil, core.WrapError(core.CodeProjectionDiverged, "decode narrative composition groups", err)
	}
	var ids []string
	if err := json.Unmarshal([]byte(factIDsJSON), &ids); err != nil {
		return nil, core.WrapError(core.CodeProjectionDiverged, "decode narrative fact source sequence", err)
	}
	if err := validateRPNarrativeComposition(core.RPNarrativeView{CompositionVersion: version, FactGroups: groups, Lines: lines, EventIDs: ids}); err != nil {
		return nil, err
	}
	return groups, nil
}

// A complete, non-fallback render becomes a versioned presentation artifact.
// Selection is per settled turn. Neither insert nor selection writes a world
// Event, moves the branch head, or alters the canonical narrative_json.
func (s *Store) saveSelectedRPNarrative(ctx context.Context, request RPNarrativeReadRequest, input core.RPNarrativeInput, view *core.RPNarrativeView, metadata core.RPProviderMetadata) error {
	if view.FallbackReason != "" {
		return nil // Failed regeneration leaves the previously selected version intact.
	}
	if len(view.Lines) == 0 || len(view.EventIDs) != len(input.Facts) {
		return core.NewError(core.CodeProjectionDiverged, "render source set does not match committed facts")
	}
	for i, fact := range input.Facts {
		if fact.EventID != view.EventIDs[i] {
			return core.NewError(core.CodeProjectionDiverged, "render source order differs from committed facts")
		}
	}
	if err := validateRPNarrativeComposition(*view); err != nil {
		return err
	}
	groups := view.FactGroups
	if groups == nil {
		groups = [][]string{}
	}
	groupsJSON, err := json.Marshal(groups)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "encode narrative composition groups", err)
	}
	factIDsJSON, err := json.Marshal(view.EventIDs)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "encode narrative fact source sequence", err)
	}
	styleJSON, err := json.Marshal(input.Style)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "encode selected narrative style", err)
	}
	// The view's IDs remain an exact fact-by-fact attribution. The persisted
	// source set additionally records any authored public context sent to the
	// renderer, without mistaking that context for an observable turn fact.
	sources := append([]string(nil), view.EventIDs...)
	seen := make(map[string]bool, len(sources))
	for _, id := range sources {
		seen[id] = true
	}
	for _, cue := range input.PublicPresentations {
		if !seen[cue.SourceEventID] {
			sources = append(sources, cue.SourceEventID)
			seen[cue.SourceEventID] = true
		}
	}
	sourcesJSON, err := json.Marshal(sources)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "encode selected narrative sources", err)
	}
	linesJSON, err := json.Marshal(view.Lines)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "encode selected narrative lines", err)
	}
	generated, err := newRPSessionID()
	if err != nil {
		return err
	}
	renderID := "rpr_" + strings.TrimPrefix(generated, "rps_")
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return err
	}
	var status string
	if err := tx.conn.QueryRowContext(ctx, `SELECT status FROM rp_turn_runs WHERE turn_run_id=? AND session_id=?`, request.TurnRunID, request.SessionID).Scan(&status); err != nil {
		return classifyMissing(err, "selected narrative turn")
	}
	if status != "settled" {
		return core.NewError(core.CodeBranchConflict, "narrative turn is no longer settled")
	}
	if err := execAgentOne(ctx, tx.conn, "save narrative render", `INSERT INTO rp_narrative_renders(render_id,turn_run_id,style_json,source_event_ids_json,lines_json,provider_kind,model_id,created_at_utc,composition_version,fact_groups_json,fact_event_ids_json) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, renderID, request.TurnRunID, string(styleJSON), string(sourcesJSON), string(linesJSON), metadata.Kind, metadata.Model, s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), view.CompositionVersion, string(groupsJSON), string(factIDsJSON)); err != nil {
		return err
	}
	if request.StyleOverride != nil {
		if err := execAgentOne(ctx, tx.conn, "select narrative render", `INSERT INTO rp_narrative_selections(turn_run_id,render_id,selected_at_utc) VALUES (?,?,?) ON CONFLICT(turn_run_id) DO UPDATE SET render_id=excluded.render_id,selected_at_utc=excluded.selected_at_utc`, request.TurnRunID, renderID, s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	view.RenderID = renderID
	return nil
}

// SelectRPNarrative changes only the displayed revision of an own settled
// turn. Cross-turn and cross-branch render IDs never become selectable.
func (s *Store) SelectRPNarrative(ctx context.Context, request RPNarrativeSelectRequest) (RPNarrativeSelectResult, error) {
	var result RPNarrativeSelectResult
	if request.PrincipalID == "" || request.SessionID == "" || request.TurnRunID == "" || len(request.RenderID) > 128 {
		return result, core.NewError(core.CodeInvalidArgument, "narrative selection requires session and settled turn")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return result, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	var baseline string
	groupsJSON := "[]"
	factIDsJSON := "[]"
	if err := tx.conn.QueryRowContext(ctx, `SELECT narrative_json,narrative_composition_version,narrative_fact_groups_json,narrative_fact_event_ids_json FROM rp_turn_runs WHERE turn_run_id=? AND session_id=? AND status='settled'`, request.TurnRunID, request.SessionID).Scan(&baseline, &result.CompositionVersion, &groupsJSON, &factIDsJSON); err != nil {
		return result, classifyMissing(err, "selectable narrative turn")
	}
	selectedJSON := baseline
	if request.RenderID == "" {
		if _, err := tx.conn.ExecContext(ctx, `DELETE FROM rp_narrative_selections WHERE turn_run_id=?`, request.TurnRunID); err != nil {
			return result, core.WrapError(core.CodeStorageFailure, "restore canonical narrative", err)
		}
	} else {
		err := tx.conn.QueryRowContext(ctx, `SELECT lines_json,composition_version,fact_groups_json,fact_event_ids_json FROM rp_narrative_renders WHERE render_id=? AND turn_run_id=?`, request.RenderID, request.TurnRunID).Scan(&selectedJSON, &result.CompositionVersion, &groupsJSON, &factIDsJSON)
		if errors.Is(err, sql.ErrNoRows) {
			return result, core.NewError(core.CodeNotFound, "narrative render does not belong to this turn")
		}
		if err != nil {
			return result, core.WrapError(core.CodeStorageFailure, "read selectable narrative render", err)
		}
		if err := execAgentOne(ctx, tx.conn, "select narrative version", `INSERT INTO rp_narrative_selections(turn_run_id,render_id,selected_at_utc) VALUES (?,?,?) ON CONFLICT(turn_run_id) DO UPDATE SET render_id=excluded.render_id,selected_at_utc=excluded.selected_at_utc`, request.TurnRunID, request.RenderID, s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")); err != nil {
			return result, err
		}
	}
	if err := json.Unmarshal([]byte(selectedJSON), &result.Lines); err != nil {
		return result, core.WrapError(core.CodeProjectionDiverged, "decode selected narrative lines", err)
	}
	result.FactGroups, err = decodeRPNarrativeComposition(result.CompositionVersion, groupsJSON, factIDsJSON, result.Lines)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	result.RenderID = request.RenderID
	return result, nil
}
