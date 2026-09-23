-- Immutable accepted speech is linked to the authoritative Event and turn.
-- Listener evidence remains in observation_records/agent_knowledge.
CREATE TABLE rp_utterances (
  utterance_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  session_id TEXT NOT NULL,
  turn_id TEXT NOT NULL UNIQUE,
  speaker_entity_id TEXT NOT NULL,
  place_id TEXT NOT NULL,
  world_time TEXT NOT NULL,
  speech_text TEXT NOT NULL CHECK (length(speech_text) BETWEEN 1 AND 2000),
  speech_act TEXT NOT NULL CHECK (speech_act IN ('statement', 'question', 'request')),
  listener_count INTEGER NOT NULL CHECK (listener_count >= 0),
  FOREIGN KEY (event_id) REFERENCES events(event_id),
  FOREIGN KEY (session_id) REFERENCES rp_sessions(session_id),
  FOREIGN KEY (speaker_entity_id) REFERENCES materialized_entities(entity_id),
  FOREIGN KEY (place_id) REFERENCES agent_places(place_id)
) STRICT;

CREATE TRIGGER rp_utterances_no_update BEFORE UPDATE ON rp_utterances
BEGIN SELECT RAISE(ABORT, 'ACCEPTED_SPEECH_IMMUTABLE'); END;

CREATE TRIGGER rp_utterances_no_delete BEFORE DELETE ON rp_utterances
BEGIN SELECT RAISE(ABORT, 'ACCEPTED_SPEECH_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp1-utterances-023-2026-09-23', '2026-09-23T00:00:00Z');
