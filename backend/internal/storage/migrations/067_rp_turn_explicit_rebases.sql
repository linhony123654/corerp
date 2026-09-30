-- Preserve the original application intent when a player explicitly retries
-- an uncommitted speech turn after observing a newer world cursor. No world
-- effect or semantic plan may be rebound by this recovery path.
CREATE TABLE rp_turn_explicit_rebases (
  turn_run_id TEXT NOT NULL REFERENCES rp_turn_runs(turn_run_id),
  revision INTEGER NOT NULL CHECK (revision >= 1),
  old_request_hash TEXT NOT NULL,
  new_request_hash TEXT NOT NULL,
  old_cursor INTEGER NOT NULL CHECK (old_cursor >= 1),
  new_cursor INTEGER NOT NULL CHECK (new_cursor > old_cursor),
  reason TEXT NOT NULL CHECK (reason = 'explicit_current_observation'),
  created_at_utc TEXT NOT NULL,
  PRIMARY KEY (turn_run_id,revision),
  CHECK (old_request_hash <> new_request_hash)
) STRICT;
CREATE TRIGGER rp_turn_explicit_rebases_no_update BEFORE UPDATE ON rp_turn_explicit_rebases
BEGIN SELECT RAISE(ABORT,'RP_TURN_REBASE_IMMUTABLE'); END;
CREATE TRIGGER rp_turn_explicit_rebases_no_delete BEFORE DELETE ON rp_turn_explicit_rebases
BEGIN SELECT RAISE(ABORT,'RP_TURN_REBASE_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-turn-explicit-rebases-067-2026-09-27','2026-09-27T00:00:00Z');
