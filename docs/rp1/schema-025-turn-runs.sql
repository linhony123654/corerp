-- Durable application orchestration; Events/positions/knowledge remain world authority.
CREATE TABLE rp_turn_runs (
  turn_run_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  player_speech_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  request_json TEXT NOT NULL CHECK (json_valid(request_json)),
  status TEXT NOT NULL CHECK (status IN ('open', 'player_committed', 'npc_deciding', 'npc_effects_committed', 'narrative_ready', 'settled')),
  player_turn_id TEXT,
  player_event_id TEXT,
  listener_ids_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(listener_ids_json)),
  narrative_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(narrative_json)),
  settled_sequence INTEGER CHECK (settled_sequence >= 0),
  created_at_utc TEXT NOT NULL,
  updated_at_utc TEXT NOT NULL,
  settled_at_utc TEXT,
  UNIQUE (session_id, idempotency_key),
  UNIQUE (session_id, player_speech_key),
  FOREIGN KEY (session_id) REFERENCES rp_sessions(session_id),
  FOREIGN KEY (player_turn_id) REFERENCES rp_utterances(turn_id),
  FOREIGN KEY (player_event_id) REFERENCES events(event_id),
  CHECK ((player_turn_id IS NULL AND player_event_id IS NULL) OR (player_turn_id IS NOT NULL AND player_event_id IS NOT NULL)),
  CHECK (status = 'open' OR player_turn_id IS NOT NULL),
  CHECK (status <> 'settled' OR (settled_at_utc IS NOT NULL AND settled_sequence IS NOT NULL))
) STRICT;

CREATE UNIQUE INDEX ux_rp_turn_one_unsettled_per_session
ON rp_turn_runs(session_id) WHERE status <> 'settled';

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp1-turn-runs-025-2026-09-23', '2026-09-23T00:00:00Z');
