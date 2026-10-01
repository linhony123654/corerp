package storage

import (
	"bytes"
	"context"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

// External proposal generators see authorized facts with unfamiliar people's
// IDs/names replaced by stable presentation handles, within the context budget.
// Validate against this actual view before rechecking the authoritative input.
func (s *Store) rpDecisionProviderView(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionInput, error) {
	var empty core.RPDecisionInput
	// Use the existing short transaction helper so head, familiarity and alias
	// secret cannot change independently. No transaction spans a provider call.
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "open NPC provider identity view", err)
	}
	defer tx.Rollback(ctx)
	conn := tx.conn
	var head int64
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, input.InstanceID, input.BranchID).Scan(&head); err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "read NPC provider view head", err)
	}
	if head != input.HeadSequence {
		return empty, core.NewError(core.CodeBranchConflict, "NPC provider view is stale")
	}
	if err := requireInternalRPDecisionOwner(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID); err != nil {
		return empty, err
	}
	input.Presentation = &core.RPDecisionPresentation{
		PolicyVersion: "corerp.rp-identity-view.v2", SourceHeadSequence: input.HeadSequence,
		IdentityMode: "observer_relative",
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "encode NPC provider view", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "decode NPC provider view", err)
	}
	// Only declared entity references participate in familiarity projection.
	// Text containing an ID is a claim, not an authoritative identity reference.
	references := make(map[string]bool)
	walkRPDecisionEntityReferences(document, "", func(id string) string {
		references[id] = true
		return id
	})
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
		if references[id] {
			unfamiliar = append(unfamiliar, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return empty, core.WrapError(core.CodeStorageFailure, "iterate NPC unfamiliar people", err)
	}
	rows.Close()
	if len(unfamiliar) == 0 {
		tx.Rollback(ctx)
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
	// Release the snapshot before packet finalization and any provider invocation.
	tx.Rollback(ctx)
	root := document.(map[string]any)
	if people, ok := root["visible_entities"].([]any); ok {
		for _, item := range people {
			person := item.(map[string]any)
			if _, unknown := aliases[person["entity_id"].(string)]; unknown {
				person["display_name"] = "陌生人"
			}
		}
	}
	document = walkRPDecisionEntityReferences(document, "", func(id string) string {
		if alias, found := aliases[id]; found {
			return alias
		}
		return id
	})
	encoded, err = json.Marshal(document)
	if err != nil || json.Unmarshal(encoded, &empty) != nil {
		return core.RPDecisionInput{}, core.NewError(core.CodeProjectionDiverged, "NPC provider view cannot be encoded")
	}
	return core.SelectRPDecisionContext(empty, core.DefaultRPDecisionContextBudgetBytes)
}

// These keys are the entity-reference fields in RPDecisionInput and its nested
// typed views. Provenance, places, contracts, handles and subjective phrases
// are deliberately absent; future entity fields must opt into this projection.
func rpDecisionEntityReference(key string) bool {
	switch key {
	case "entity_id", "npc_entity_id", "interlocutor_entity_id", "subject_entity_id",
		"speaker_entity_id", "actor_entity_id", "target_entity_id", "actor_id",
		"store_actor_id", "counterparty_entity_id", "friend_id":
		return true
	default:
		return false
	}
}

func walkRPDecisionEntityReferences(value any, key string, project func(string) string) any {
	switch typed := value.(type) {
	case map[string]any:
		for name, child := range typed {
			typed[name] = walkRPDecisionEntityReferences(child, name, project)
		}
		return typed
	case []any:
		for i, child := range typed {
			typed[i] = walkRPDecisionEntityReferences(child, key, project)
		}
		return typed
	case string:
		if rpDecisionEntityReference(key) {
			return project(typed)
		}
		return typed
	default:
		return value
	}
}
