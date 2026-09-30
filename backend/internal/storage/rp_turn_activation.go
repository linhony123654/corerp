package storage

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

type rpTurnListenerActivation struct {
	NPCEntityID             string
	Disposition             string
	ReasonCode              string
	ActivationRank          sql.NullInt64
	OwnerSourceEvent        sql.NullString
	ConversationSourceEvent sql.NullString
}

type rpActivationCandidate struct {
	id         string
	addressed  bool
	continuing bool
}

// ensureRPTurnActivationPlan persists one complete accounting row per heard
// listener. It is orchestration state, not world authority: speech hearing is
// already committed and only later validated NPC effects become Events.
func (s *Store) ensureRPTurnActivationPlan(ctx context.Context, runID, sessionID, speechText string, listenerIDs []string) ([]rpTurnListenerActivation, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "begin RP turn activation plan", err)
	}
	defer tx.Rollback(ctx)
	available, err := rpTurnActivationTableAvailable(ctx, tx.conn)
	if err != nil {
		return nil, err
	}
	if !available {
		legacy := make([]rpTurnListenerActivation, 0, len(listenerIDs))
		for index, id := range listenerIDs {
			legacy = append(legacy, rpTurnListenerActivation{NPCEntityID: id, Disposition: "activated", ReasonCode: "legacy_compatible", ActivationRank: sql.NullInt64{Int64: int64(index), Valid: true}})
		}
		return legacy, nil
	}

	var existing int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=?`, runID).Scan(&existing); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "count RP turn activations", err)
	}
	if existing != 0 {
		if existing != len(listenerIDs) {
			return nil, core.NewError(core.CodeProjectionDiverged, "RP turn activation plan is incomplete")
		}
		tx.Rollback(ctx)
		return s.loadRPTurnActivations(ctx, runID)
	}

	var instance, branch, observer string
	if err := tx.conn.QueryRowContext(ctx, `SELECT instance_id,branch_id,controlled_entity_id FROM rp_sessions WHERE session_id=?`, sessionID).Scan(&instance, &branch, &observer); err != nil {
		return nil, classifyMissing(err, "RP turn activation session")
	}
	mode, limit, err := readRPTurnExecutionMode(ctx, tx.conn, instance, branch, len(listenerIDs))
	if err != nil {
		return nil, err
	}

	knownNames := map[string][]string{}
	focusAvailable, err := rpConversationFocusAvailable(ctx, tx.conn)
	if err != nil {
		return nil, err
	}
	canInterpret := mode == "orchestrated" || mode == "multi_agent" || mode == "legacy" && focusAvailable
	if canInterpret {
		rows, err := tx.conn.QueryContext(ctx, `SELECT f.subject_agent_id,m.display_name FROM rp_identity_familiarity f JOIN materialized_entities m ON m.entity_id=f.subject_agent_id WHERE f.observer_agent_id=? AND f.instance_id=? AND f.branch_id=?`, observer, instance, branch)
		if err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "read familiar listener names", err)
		}
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, core.WrapError(core.CodeStorageFailure, "scan familiar listener name", err)
			}
			knownNames[id] = []string{name}
		}
		if err := rows.Close(); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "close familiar listener names", err)
		}
		if err := rows.Err(); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "iterate familiar listener names", err)
		}
		var head int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, instance, branch).Scan(&head); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "read addressed relationship head", err)
		}
		// The player's own authored address-to rules identify the target. A
		// reverse relationship, NPC private persona or model prior cannot add
		// an alias to this observer's addressing vocabulary.
		relations, err := readRPAuthoredRelationships(ctx, tx.conn, core.RPDecisionInput{InstanceID: instance, BranchID: branch, HeadSequence: head, NPCEntityID: observer})
		if err != nil {
			return nil, err
		}
		for _, relation := range relations {
			if _, known := knownNames[relation.SubjectEntityID]; known {
				knownNames[relation.SubjectEntityID] = append(knownNames[relation.SubjectEntityID], relation.AddressTo...)
			}
		}
	}
	var focus rpConversationFocus
	if canInterpret && focusAvailable {
		focus, err = readRPConversationFocus(ctx, tx.conn, runID)
		if err != nil {
			return nil, err
		}
	}

	type owner struct{ id, source string }
	external := make([]owner, 0)
	candidates := make([]rpActivationCandidate, 0, len(listenerIDs))
	for _, id := range listenerIDs {
		source, err := rpNonInternalDecisionOwnerSource(ctx, tx.conn, instance, branch, id)
		if err != nil {
			return nil, err
		}
		if source != "" {
			external = append(external, owner{id, source})
			continue
		}
		candidates = append(candidates, rpActivationCandidate{id: id, addressed: canInterpret && rpSpeechAddressesForms(speechText, knownNames[id]), continuing: focus.actorID == id})
	}
	if canInterpret {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].addressed != candidates[j].addressed {
				return candidates[i].addressed
			}
			if candidates[i].continuing != candidates[j].continuing {
				return candidates[i].continuing
			}
			return candidates[i].id < candidates[j].id
		})
	}
	activeCount := rpTurnActiveCandidateCount(mode, limit, candidates)

	for _, item := range external {
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,owner_source_event_id) VALUES (?,?,'externally_controlled','external_controller',?)`, runID, item.id, item.source); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "record externally controlled listener", err)
		}
		if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO rp_turn_listener_skips(turn_run_id,npc_entity_id,owner_source_event_id) VALUES (?,?,?)`, runID, item.id, item.source); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "record externally controlled listener skip", err)
		}
	}
	for index, candidate := range candidates {
		if index < activeCount {
			reason := "stable_fallback"
			var conversationSource any
			if candidate.addressed {
				reason = "direct_address"
			} else if candidate.continuing {
				reason = "conversation_continuation"
				conversationSource = focus.sourceEventID
			} else if mode == "legacy" {
				reason = "legacy_compatible"
			}
			query := `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,activation_rank) VALUES (?,?,'activated',?,?)`
			args := []any{runID, candidate.id, reason, index}
			if focusAvailable {
				query = `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,activation_rank,conversation_source_event_id) VALUES (?,?,'activated',?,?,?)`
				args = append(args, conversationSource)
			}
			if _, err := tx.conn.ExecContext(ctx, query, args...); err != nil {
				return nil, core.WrapError(core.CodeStorageFailure, "record activated listener", err)
			}
			continue
		}
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code) VALUES (?,?,'not_activated','responder_limit')`, runID, candidate.id); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "record inactive listener", err)
		}
	}
	if err := execAgentOne(ctx, tx.conn, "pin RP turn execution mode", `UPDATE rp_turn_runs SET execution_mode=?,responder_limit=?,updated_at_utc=? WHERE turn_run_id=? AND status='npc_deciding'`, mode, limit, s.now().UTC().Format(time.RFC3339Nano), runID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "commit RP turn activation plan", err)
	}
	return s.loadRPTurnActivations(ctx, runID)
}

func rpTurnActivationTableAvailable(ctx context.Context, q replayQuerier) (bool, error) {
	var tableCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rp_turn_listener_activations'`).Scan(&tableCount); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "inspect RP turn activation projection", err)
	}
	if tableCount != 0 {
		return true, nil
	}
	var versionCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, RPTurnActivationSchemaVersion).Scan(&versionCount); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "inspect RP turn activation schema version", err)
	}
	if versionCount != 0 {
		return false, core.NewError(core.CodeStorageFailure, "RP turn activation projection is missing after migration")
	}
	return false, nil
}

// Execution modes deliberately differ while sharing one authoritative commit
// path. deterministic selects one stable actor without interpreting names;
// orchestrated selects only explicitly addressed familiar actors (or one
// publicly grounded continuing partner / stable fallback); multi_agent fills
// the declared bounded responder budget. Legacy retains every heard listener.
func rpTurnActiveCandidateCount(mode string, limit int, candidates []rpActivationCandidate) int {
	if len(candidates) == 0 {
		return 0
	}
	switch mode {
	case "legacy":
		return len(candidates)
	case "deterministic":
		return 1
	case "orchestrated":
		addressed := 0
		for _, candidate := range candidates {
			if candidate.addressed {
				addressed++
			}
		}
		if addressed == 0 {
			return 1
		}
		if addressed < limit {
			return addressed
		}
		return limit
	case "multi_agent":
		if len(candidates) < limit {
			return len(candidates)
		}
		return limit
	default:
		return 0
	}
}

func readRPTurnExecutionMode(ctx context.Context, conn *sql.Conn, instance, branch string, listenerCount int) (string, int, error) {
	packages, err := readStudioActivePackages(ctx, conn, instance, branch)
	if err != nil {
		return "", 0, err
	}
	if packages == nil || packages.System.Content.SystemRules == nil || packages.System.Content.SystemRules.RPExecutionMode == "" {
		return "legacy", listenerCount, nil
	}
	rules := packages.System.Content.SystemRules
	return rules.RPExecutionMode, rules.MaxActiveResponders, nil
}

func rpSpeechAddressesForms(text string, forms []string) bool {
	for _, form := range forms {
		if rpSpeechAddressesName(text, form) {
			return true
		}
	}
	return false
}

func rpSpeechAddressesName(text, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	text, name = strings.ToLower(text), strings.ToLower(name)
	if utf8.RuneCountInString(name) >= 2 {
		ascii := true
		for _, char := range name {
			if char > 127 {
				ascii = false
				break
			}
		}
		if ascii {
			// Short Latin names must not match inside book/Anna/etc. This
			// remains a lexical cue, not a semantic addressee classifier.
			for offset := 0; offset < len(text); {
				index := strings.Index(text[offset:], name)
				if index < 0 {
					return false
				}
				index += offset
				end := index + len(name)
				if (index == 0 || !rpASCIIWordCharacter(text[index-1])) && (end == len(text) || !rpASCIIWordCharacter(text[end])) {
					return true
				}
				offset = index + 1
			}
			return false
		}
		return strings.Contains(text, name)
	}
	for _, form := range []string{"@" + name, name + "，", name + ",", name + "：", name + ":"} {
		if strings.Contains(text, form) {
			return true
		}
	}
	return false
}

func rpASCIIWordCharacter(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_'
}

func (s *Store) loadRPTurnActivations(ctx context.Context, runID string) ([]rpTurnListenerActivation, error) {
	available, err := rpConversationFocusAvailable(ctx, s.db)
	if err != nil {
		return nil, err
	}
	column := `NULL`
	if available {
		column = `conversation_source_event_id`
	}
	rows, err := s.db.QueryContext(ctx, `SELECT npc_entity_id,disposition,reason_code,activation_rank,owner_source_event_id,`+column+` FROM rp_turn_listener_activations WHERE turn_run_id=? ORDER BY CASE disposition WHEN 'activated' THEN 0 WHEN 'externally_controlled' THEN 1 ELSE 2 END,activation_rank,npc_entity_id`, runID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read RP turn activations", err)
	}
	defer rows.Close()
	result := make([]rpTurnListenerActivation, 0)
	for rows.Next() {
		var item rpTurnListenerActivation
		if err := rows.Scan(&item.NPCEntityID, &item.Disposition, &item.ReasonCode, &item.ActivationRank, &item.OwnerSourceEvent, &item.ConversationSourceEvent); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan RP turn activation", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate RP turn activations", err)
	}
	return result, nil
}
