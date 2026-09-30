-- Extend accepted interaction children without rewriting 031 or its referenced
-- 048 interpretation rows. Legacy move/speech/wait children remain in 031.
CREATE TABLE rp_interaction_pending_actions (
  interaction_id TEXT PRIMARY KEY REFERENCES rp_interactions(interaction_id),
  step_index INTEGER NOT NULL CHECK (step_index BETWEEN 0 AND 2),
  kind TEXT NOT NULL CHECK (kind IN ('object','nonverbal')),
  request_json TEXT NOT NULL CHECK (json_valid(request_json) AND request_json <> '{}'),
  created_at_utc TEXT NOT NULL
) STRICT;
CREATE TRIGGER rp_interaction_pending_actions_no_update BEFORE UPDATE ON rp_interaction_pending_actions
BEGIN SELECT RAISE(ABORT,'RP_INTERACTION_CHILD_IMMUTABLE'); END;

-- The frozen 027 retirement CHECK cannot hold the new owner operations.
-- The fence remains application state and has no world authority.
CREATE TABLE rp_typed_action_retirements (
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  operation TEXT NOT NULL CHECK (operation IN ('object','nonverbal')),
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
  retired_at_utc TEXT NOT NULL,
  PRIMARY KEY (principal_id,operation,session_id,idempotency_key)
) STRICT;
CREATE TRIGGER rp_typed_action_retirement_no_update BEFORE UPDATE ON rp_typed_action_retirements
BEGIN SELECT RAISE(ABORT,'RP_TYPED_ACTION_RETIREMENT_IMMUTABLE'); END;
CREATE TRIGGER rp_typed_action_retirement_no_delete BEFORE DELETE ON rp_typed_action_retirements
BEGIN SELECT RAISE(ABORT,'RP_TYPED_ACTION_RETIREMENT_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-typed-action-children-071-2026-09-27','2026-09-27T00:00:00Z');
