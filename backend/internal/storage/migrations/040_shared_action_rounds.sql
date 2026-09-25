-- Application-only proposals. The chosen typed RP Event remains world authority.
ALTER TABLE rp_shared_rounds RENAME COLUMN wait_event_id TO completion_event_id;
ALTER TABLE rp_shared_rounds ADD COLUMN settlement_kind TEXT NOT NULL DEFAULT '' CHECK (settlement_kind IN ('','wait','speech'));
ALTER TABLE rp_shared_rounds ADD COLUMN selected_session_id TEXT NOT NULL DEFAULT '';
UPDATE rp_shared_rounds SET settlement_kind='wait' WHERE status='settled';
UPDATE rp_shared_rounds SET settlement_kind='wait' WHERE status='advancing';

CREATE TABLE rp_shared_round_actions (
  round_id TEXT NOT NULL REFERENCES rp_shared_rounds(round_id),
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  submission_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  request_json TEXT NOT NULL,
  submitted_at_utc TEXT NOT NULL,
  PRIMARY KEY(round_id,session_id),
  FOREIGN KEY(round_id,session_id) REFERENCES rp_shared_round_participants(round_id,session_id)
) STRICT;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f3-shared-action-rounds-040-2026-09-25','2026-09-25T00:00:00Z');
