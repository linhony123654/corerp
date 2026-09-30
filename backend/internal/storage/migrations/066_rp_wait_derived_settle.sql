-- Non-authoritative recovery marker: a committed wait is not fully returned
-- until the deterministic activity sweep at its own world time has completed.
-- Historical waits lack a marker; their derived-settle outcome is unknown.
CREATE TABLE rp_wait_activity_settlements (
  intent_id TEXT PRIMARY KEY REFERENCES rp_wait_intents(intent_id),
  status TEXT NOT NULL CHECK (status IN ('pending','complete')),
  completed_at_utc TEXT,
  CHECK ((status='pending') = (completed_at_utc IS NULL))
) STRICT;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-wait-derived-settle-066-2026-09-27','2026-09-27T00:00:00Z');
