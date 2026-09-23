-- Application retry state only; world_clocks and events remain time authority.
CREATE TABLE rp_wait_intents (
  intent_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  target_world_time TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'completed')),
  command_id TEXT,
  created_at_utc TEXT NOT NULL,
  completed_at_utc TEXT,
  UNIQUE (session_id, idempotency_key),
  FOREIGN KEY (session_id) REFERENCES rp_sessions(session_id),
  FOREIGN KEY (command_id) REFERENCES commands(command_id),
  CHECK ((status = 'pending' AND command_id IS NULL AND completed_at_utc IS NULL)
      OR (status = 'completed' AND command_id IS NOT NULL AND completed_at_utc IS NOT NULL))
) STRICT;

CREATE UNIQUE INDEX ux_rp_wait_one_pending_per_session
ON rp_wait_intents(session_id) WHERE status = 'pending';

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp1-wait-intents-022-2026-09-23', '2026-09-23T00:00:00Z');
