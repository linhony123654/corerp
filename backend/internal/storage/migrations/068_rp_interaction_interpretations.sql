-- Interpreter calls are application evidence, not Events or character Knowledge.
-- The partial unique index serializes interpretation for an unaccepted request.
CREATE TABLE rp_interaction_interpretations (
  call_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
  request_hash TEXT NOT NULL,
  baseline_cursor INTEGER NOT NULL CHECK (baseline_cursor > 0),
  interaction_id TEXT UNIQUE REFERENCES rp_interactions(interaction_id),
  source TEXT NOT NULL CHECK (source IN ('model','offline_rules')),
  provider_kind TEXT NOT NULL,
  model_id TEXT NOT NULL DEFAULT '',
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 100),
  result TEXT NOT NULL CHECK (result IN ('pending','success','failed','timeout')),
  reason TEXT NOT NULL DEFAULT '',
  started_unix INTEGER NOT NULL,
  finished_at_utc TEXT,
  CHECK ((result='pending') = (finished_at_utc IS NULL)),
  CHECK ((result='success') = (interaction_id IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX ux_rp_interpretation_pending
ON rp_interaction_interpretations(session_id,idempotency_key) WHERE result='pending';
CREATE INDEX ix_rp_interpretation_request
ON rp_interaction_interpretations(session_id,idempotency_key,started_unix);
CREATE TRIGGER rp_interpretation_no_delete BEFORE DELETE ON rp_interaction_interpretations
BEGIN SELECT RAISE(ABORT,'RP_INTERPRETATION_IMMUTABLE'); END;
CREATE TRIGGER rp_interpretation_no_update_finished BEFORE UPDATE ON rp_interaction_interpretations WHEN OLD.result<>'pending'
BEGIN SELECT RAISE(ABORT,'RP_INTERPRETATION_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-interaction-interpretations-068-2026-09-27','2026-09-27T00:00:00Z');
