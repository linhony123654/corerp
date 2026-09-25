-- Durable application orchestration only. Child commands and Events remain the
-- sole owners of movement, speech, time and Knowledge.
CREATE TABLE rp_interactions (
  interaction_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
  request_hash TEXT NOT NULL,
  request_json TEXT NOT NULL CHECK (json_valid(request_json)),
  plan_json TEXT NOT NULL CHECK (json_valid(plan_json)),
  status TEXT NOT NULL CHECK (status IN ('open','paused','stopped','clarification','settled')),
  pause_reason TEXT NOT NULL DEFAULT '',
  next_step INTEGER NOT NULL DEFAULT 0 CHECK (next_step >= 0 AND next_step <= 3),
  pending_kind TEXT NOT NULL DEFAULT '' CHECK (pending_kind IN ('','move','speech','wait')),
  pending_request_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(pending_request_json)),
  outcomes_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(outcomes_json)),
  created_at_utc TEXT NOT NULL,
  updated_at_utc TEXT NOT NULL,
  UNIQUE (session_id,idempotency_key),
  CHECK ((pending_kind='') = (pending_request_json='{}'))
) STRICT;
CREATE UNIQUE INDEX ux_rp_interaction_one_active_per_session
ON rp_interactions(session_id) WHERE status IN ('open','paused');

-- Interaction preference is application configuration, not a world Event.
-- Immutable revisions retain exact retry identity without changing old turns.
CREATE TABLE rp_interaction_mode_revisions (
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  revision INTEGER NOT NULL CHECK (revision >= 1),
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
  request_hash TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('AUTO','DIALOGUE','SCENE')),
  created_at_utc TEXT NOT NULL,
  PRIMARY KEY (session_id,revision),
  UNIQUE (session_id,idempotency_key)
) STRICT;
CREATE TRIGGER rp_interaction_mode_revision_no_update BEFORE UPDATE ON rp_interaction_mode_revisions
BEGIN SELECT RAISE(ABORT,'RP_INTERACTION_MODE_IMMUTABLE'); END;
CREATE TRIGGER rp_interaction_mode_revision_no_delete BEFORE DELETE ON rp_interaction_mode_revisions
BEGIN SELECT RAISE(ABORT,'RP_INTERACTION_MODE_IMMUTABLE'); END;

-- An unaccepted interaction key can be permanently fenced without changing
-- schema027's frozen operation CHECK or touching an accepted plan.
CREATE TABLE rp_interaction_retirements (
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
  retired_at_utc TEXT NOT NULL,
  PRIMARY KEY (principal_id,session_id,idempotency_key)
) STRICT;
CREATE TRIGGER rp_interaction_retirement_no_update BEFORE UPDATE ON rp_interaction_retirements
BEGIN SELECT RAISE(ABORT,'RP_INTERACTION_RETIREMENT_IMMUTABLE'); END;
CREATE TRIGGER rp_interaction_retirement_no_delete BEFORE DELETE ON rp_interaction_retirements
BEGIN SELECT RAISE(ABORT,'RP_INTERACTION_RETIREMENT_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f1-interactions-031-2026-09-25','2026-09-25T00:00:00Z');
