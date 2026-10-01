-- Keep FK enforcement active. Incoming references are NO ACTION; defer them
-- only until the SAME parent name has been recreated and rows reinserted.
-- Do not rename a referenced parent or swap in an already-populated staging
-- table: restoring rows under the original name resolves deferred references.
PRAGMA defer_foreign_keys = ON;
CREATE TEMP TABLE rp_turn_runs_080_saved AS SELECT * FROM rp_turn_runs;
DROP TABLE rp_turn_runs;
CREATE TABLE rp_turn_runs (
  turn_run_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  idempotency_key TEXT NOT NULL,
  player_speech_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  request_json TEXT NOT NULL CHECK (json_valid(request_json)),
  status TEXT NOT NULL CHECK (status IN ('open','player_committed','npc_deciding','npc_effects_committed','narrative_ready','settled')),
  player_turn_id TEXT REFERENCES rp_utterances(turn_id),
  player_event_id TEXT REFERENCES events(event_id),
  listener_ids_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(listener_ids_json)),
  narrative_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(narrative_json)),
  settled_sequence INTEGER CHECK (settled_sequence >= 0),
  created_at_utc TEXT NOT NULL,
  updated_at_utc TEXT NOT NULL,
  settled_at_utc TEXT,
  narrative_fallback TEXT,
  execution_mode TEXT NOT NULL DEFAULT 'legacy' CHECK (execution_mode IN ('legacy','deterministic','orchestrated','multi_agent')),
  responder_limit INTEGER NOT NULL DEFAULT 0 CHECK (responder_limit BETWEEN 0 AND 8),
  narrative_presentation_mode TEXT NOT NULL DEFAULT 'base' CHECK (narrative_presentation_mode IN ('base','deterministic','style_planner','full_prose','custom')),
  narrative_presented_at_utc TEXT,
  narrative_composition_version TEXT NOT NULL DEFAULT '',
  narrative_fact_groups_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(narrative_fact_groups_json) AND json_type(narrative_fact_groups_json)='array'),
  narrative_fact_event_ids_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(narrative_fact_event_ids_json) AND json_type(narrative_fact_event_ids_json)='array'),
  narrative_artifact_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(narrative_artifact_json) AND json_type(narrative_artifact_json)='object'),
  trigger_kind TEXT NOT NULL DEFAULT 'speech' CHECK (trigger_kind IN ('speech','nonverbal')),
  UNIQUE(session_id,idempotency_key),
  UNIQUE(session_id,player_speech_key),
  CHECK ((trigger_kind='speech'
    AND ((player_turn_id IS NULL AND player_event_id IS NULL) OR (player_turn_id IS NOT NULL AND player_event_id IS NOT NULL))
    AND (status='open' OR player_turn_id IS NOT NULL))
    OR (trigger_kind='nonverbal' AND player_turn_id IS NULL
      AND ((status='open' AND player_event_id IS NULL) OR (status<>'open' AND player_event_id IS NOT NULL)))),
  CHECK (status<>'settled' OR (settled_at_utc IS NOT NULL AND settled_sequence IS NOT NULL))
) STRICT;
INSERT INTO rp_turn_runs (
  turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,status,
  player_turn_id,player_event_id,listener_ids_json,narrative_json,settled_sequence,
  created_at_utc,updated_at_utc,settled_at_utc,narrative_fallback,execution_mode,responder_limit,
  narrative_presentation_mode,narrative_presented_at_utc,narrative_composition_version,
  narrative_fact_groups_json,narrative_fact_event_ids_json,narrative_artifact_json,trigger_kind
)
SELECT turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,status,
  player_turn_id,player_event_id,listener_ids_json,narrative_json,settled_sequence,
  created_at_utc,updated_at_utc,settled_at_utc,narrative_fallback,execution_mode,responder_limit,
  narrative_presentation_mode,narrative_presented_at_utc,narrative_composition_version,
  narrative_fact_groups_json,narrative_fact_event_ids_json,narrative_artifact_json,'speech'
FROM rp_turn_runs_080_saved;
DROP TABLE rp_turn_runs_080_saved;
CREATE UNIQUE INDEX ux_rp_turn_one_unsettled_per_session ON rp_turn_runs(session_id) WHERE status<>'settled';

-- Activation rows have no inbound FK. Preserve every existing column/value,
-- including 076 conversation provenance, and add a typed action preference.
CREATE TABLE rp_turn_listener_activations_080 (
  turn_run_id TEXT NOT NULL REFERENCES rp_turn_runs(turn_run_id),
  npc_entity_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  disposition TEXT NOT NULL CHECK (disposition IN ('activated','externally_controlled','not_activated')),
  reason_code TEXT NOT NULL CHECK (reason_code IN ('legacy_compatible','direct_address','direct_action','conversation_continuation','stable_fallback','external_controller','responder_limit')),
  activation_rank INTEGER CHECK (activation_rank IS NULL OR activation_rank >= 0),
  owner_source_event_id TEXT REFERENCES events(event_id),
  conversation_source_event_id TEXT REFERENCES events(event_id),
  PRIMARY KEY(turn_run_id,npc_entity_id),
  CHECK ((disposition='activated' AND activation_rank IS NOT NULL AND owner_source_event_id IS NULL)
    OR (disposition='externally_controlled' AND activation_rank IS NULL AND owner_source_event_id IS NOT NULL)
    OR (disposition='not_activated' AND activation_rank IS NULL AND owner_source_event_id IS NULL)),
  CHECK ((reason_code='conversation_continuation' AND disposition='activated' AND conversation_source_event_id IS NOT NULL)
    OR (reason_code<>'conversation_continuation' AND conversation_source_event_id IS NULL))
) STRICT;
INSERT INTO rp_turn_listener_activations_080
SELECT turn_run_id,npc_entity_id,disposition,reason_code,activation_rank,owner_source_event_id,conversation_source_event_id
FROM rp_turn_listener_activations;
DROP TABLE rp_turn_listener_activations;
ALTER TABLE rp_turn_listener_activations_080 RENAME TO rp_turn_listener_activations;
CREATE UNIQUE INDEX ux_rp_turn_activation_rank ON rp_turn_listener_activations(turn_run_id,activation_rank) WHERE activation_rank IS NOT NULL;

-- applyMigration checks all FK references before committing this transaction.
INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-action-triggers-080-2026-10-01','2026-10-01T00:00:00Z');
