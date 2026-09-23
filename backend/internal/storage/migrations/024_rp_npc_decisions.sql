-- Committed NPC effects are child decisions of an accepted player turn.
CREATE TABLE rp_npc_decisions (
  decision_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  parent_turn_id TEXT NOT NULL,
  npc_entity_id TEXT NOT NULL,
  event_id TEXT NOT NULL UNIQUE,
  action TEXT NOT NULL CHECK (action IN ('respond', 'refuse', 'silence', 'wait', 'leave')),
  input_hash TEXT NOT NULL,
  proposal_hash TEXT NOT NULL,
  proposal_json TEXT NOT NULL CHECK (json_valid(proposal_json)),
  UNIQUE (session_id, parent_turn_id, npc_entity_id),
  FOREIGN KEY (session_id) REFERENCES rp_sessions(session_id),
  FOREIGN KEY (parent_turn_id) REFERENCES rp_utterances(turn_id),
  FOREIGN KEY (npc_entity_id) REFERENCES materialized_entities(entity_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TRIGGER rp_npc_decisions_no_update BEFORE UPDATE ON rp_npc_decisions
BEGIN SELECT RAISE(ABORT, 'NPC_DECISION_IMMUTABLE'); END;

CREATE TRIGGER rp_npc_decisions_no_delete BEFORE DELETE ON rp_npc_decisions
BEGIN SELECT RAISE(ABORT, 'NPC_DECISION_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp1-npc-decisions-024-2026-09-23', '2026-09-23T00:00:00Z');
