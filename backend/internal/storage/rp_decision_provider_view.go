package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"

	"corerp.local/backend/internal/core"
)

// External proposal generators see authorized facts with unfamiliar people's
// IDs/names replaced by stable presentation handles, within the context budget.
// Validate against this actual view before rechecking the authoritative input.
func (s *Store) rpDecisionProviderView(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionInput, error) {
	var empty core.RPDecisionInput
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "open NPC provider identity view", err)
	}
	defer conn.Close()
	var head int64
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, input.InstanceID, input.BranchID).Scan(&head); err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "read NPC provider view head", err)
	}
	if head != input.HeadSequence {
		return empty, core.NewError(core.CodeBranchConflict, "NPC provider view is stale")
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "encode NPC provider view", err)
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT a.agent_id FROM agent_profiles a
		LEFT JOIN rp_identity_familiarity f ON f.observer_agent_id=? AND f.subject_agent_id=a.agent_id
		  AND f.instance_id=a.instance_id AND f.branch_id=a.branch_id
		WHERE a.instance_id=? AND a.branch_id=? AND a.agent_id<>?
		  AND f.subject_agent_id IS NULL ORDER BY a.agent_id`, input.NPCEntityID, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "read NPC unfamiliar people", err)
	}
	var unfamiliar []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return empty, core.WrapError(core.CodeStorageFailure, "scan NPC unfamiliar person", err)
		}
		if bytes.Contains(encoded, []byte(id)) {
			unfamiliar = append(unfamiliar, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return empty, core.WrapError(core.CodeStorageFailure, "iterate NPC unfamiliar people", err)
	}
	rows.Close()
	if len(unfamiliar) == 0 {
		return core.SelectRPDecisionContext(input, core.DefaultRPDecisionContextBudgetBytes)
	}
	aliases := make(map[string]string, len(unfamiliar))
	for _, id := range unfamiliar {
		alias, err := rpAnonymousEntityID(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, id)
		if err != nil {
			return empty, core.WrapError(core.CodeStorageFailure, "mask NPC unfamiliar person", err)
		}
		aliases[id] = alias
	}
	sort.Slice(unfamiliar, func(i, j int) bool { return len(unfamiliar[i]) > len(unfamiliar[j]) })
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "decode NPC provider view", err)
	}
	root := document.(map[string]any)
	if people, ok := root["visible_entities"].([]any); ok {
		for _, item := range people {
			person := item.(map[string]any)
			if _, unknown := aliases[person["entity_id"].(string)]; unknown {
				person["display_name"] = "陌生人"
			}
		}
	}
	document = maskRPDecisionIDs(document, "", unfamiliar, aliases)
	encoded, err = json.Marshal(document)
	if err != nil || json.Unmarshal(encoded, &empty) != nil {
		return core.RPDecisionInput{}, core.NewError(core.CodeProjectionDiverged, "NPC provider view cannot be encoded")
	}
	return core.SelectRPDecisionContext(empty, core.DefaultRPDecisionContextBudgetBytes)
}

func maskRPDecisionIDs(value any, key string, unfamiliar []string, aliases map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		for name, child := range typed {
			typed[name] = maskRPDecisionIDs(child, name, unfamiliar, aliases)
		}
		return typed
	case []any:
		for i, child := range typed {
			typed[i] = maskRPDecisionIDs(child, key, unfamiliar, aliases)
		}
		return typed
	case string:
		// Accepted and observed utterances remain verbatim. A speaker may claim
		// any name or ID, but that claim must not grant canonical identity.
		if key == "text" || key == "player_speech_text" || key == "persona" {
			return typed
		}
		for _, id := range unfamiliar {
			typed = strings.ReplaceAll(typed, id, aliases[id])
		}
		return typed
	default:
		return value
	}
}
