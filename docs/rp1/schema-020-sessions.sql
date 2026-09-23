-- RP-1 application session metadata; world identity, location and time remain elsewhere.
CREATE TABLE rp_sessions (
  session_id TEXT PRIMARY KEY,
  principal_id TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  controlled_entity_id TEXT NOT NULL,
  pov TEXT NOT NULL CHECK (pov IN ('first_person', 'second_person')),
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  observation_cursor INTEGER NOT NULL DEFAULT 0 CHECK (observation_cursor >= 0),
  turn_cursor TEXT NOT NULL DEFAULT '',
  turn_state TEXT NOT NULL DEFAULT 'idle',
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
  created_at_utc TEXT NOT NULL,
  resumed_at_utc TEXT NOT NULL,
  UNIQUE (principal_id, idempotency_key),
  FOREIGN KEY (principal_id) REFERENCES principals(principal_id),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id),
  FOREIGN KEY (controlled_entity_id) REFERENCES materialized_entities(entity_id)
) STRICT;

CREATE INDEX ix_rp_sessions_principal_status
ON rp_sessions(principal_id, status, resumed_at_utc);

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp1-sessions-020-2026-09-23', '2026-09-23T00:00:00Z');
