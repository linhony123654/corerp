-- Application diagnostics only. Provider attempts and presentation do not
-- change Events, branch heads, world time, Knowledge or canonical narration.
CREATE TABLE rp_provider_calls (
  call_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  turn_run_id TEXT REFERENCES rp_turn_runs(turn_run_id),
  subject_id TEXT NOT NULL,
  npc_entity_id TEXT NOT NULL DEFAULT '',
  phase TEXT NOT NULL CHECK (phase IN ('decision','narrative')),
  provider_kind TEXT NOT NULL,
  model_id TEXT NOT NULL DEFAULT '',
  attempted INTEGER NOT NULL CHECK (attempted IN (0,1)),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0 AND attempt_count <= 100),
  result TEXT NOT NULL CHECK (result IN ('pending','success','failed','timeout','not_used')),
  fallback_kind TEXT NOT NULL DEFAULT '',
  render_source TEXT NOT NULL DEFAULT '',
  started_at_utc TEXT NOT NULL,
  finished_at_utc TEXT,
  CHECK ((result='pending') = (finished_at_utc IS NULL)),
  CHECK (attempted = CASE WHEN attempt_count > 0 THEN 1 ELSE 0 END)
) STRICT;
CREATE INDEX ix_rp_provider_calls_turn ON rp_provider_calls(session_id,turn_run_id,phase,started_at_utc);
CREATE INDEX ix_rp_provider_calls_subject ON rp_provider_calls(session_id,subject_id,phase,started_at_utc);
CREATE TRIGGER rp_provider_calls_no_delete BEFORE DELETE ON rp_provider_calls
BEGIN SELECT RAISE(ABORT,'RP_PROVIDER_RECEIPT_IMMUTABLE'); END;
CREATE TRIGGER rp_provider_calls_no_update_finished BEFORE UPDATE ON rp_provider_calls WHEN OLD.result<>'pending'
BEGIN SELECT RAISE(ABORT,'RP_PROVIDER_RECEIPT_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-provider-receipts-065-2026-09-27','2026-09-27T00:00:00Z');
