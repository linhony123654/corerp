-- Add selected law-sourced publication while preserving the populated F3 FK chain.
PRAGMA defer_foreign_keys=ON;

CREATE TABLE rp_shared_rounds_f7_public_publish (
  round_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  operator_principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  human_session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  baseline_world_time TEXT NOT NULL,
  baseline_head INTEGER NOT NULL CHECK (baseline_head >= 1),
  status TEXT NOT NULL CHECK (status IN ('open','advancing','settled','stale')),
  advance_target TEXT NOT NULL DEFAULT '',
  wait_key TEXT NOT NULL DEFAULT '',
  completion_event_id TEXT REFERENCES events(event_id),
  settled_sequence INTEGER CHECK (settled_sequence >= 1),
  created_at_utc TEXT NOT NULL,
  settled_at_utc TEXT,
  settlement_kind TEXT NOT NULL DEFAULT '' CHECK (settlement_kind IN ('','wait','speech','health','information')),
  selected_session_id TEXT NOT NULL DEFAULT '',
  selected_action_kind TEXT NOT NULL DEFAULT 'speech' CHECK (selected_action_kind IN ('speech','move','sleep_start','sleep_end','work_task','information_send','information_stance','information_relay','information_public_access','information_organization_access','information_public_publish')),
  UNIQUE(instance_id,branch_id,operator_principal_id,idempotency_key),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((status='settled')=(completion_event_id IS NOT NULL AND settled_sequence IS NOT NULL AND settled_at_utc IS NOT NULL))
) STRICT;

INSERT INTO rp_shared_rounds_f7_public_publish (
  round_id,instance_id,branch_id,operator_principal_id,idempotency_key,request_hash,
  human_session_id,baseline_world_time,baseline_head,status,advance_target,wait_key,
  completion_event_id,settled_sequence,created_at_utc,settled_at_utc,settlement_kind,
  selected_session_id,selected_action_kind
)
SELECT round_id,instance_id,branch_id,operator_principal_id,idempotency_key,request_hash,
       human_session_id,baseline_world_time,baseline_head,status,advance_target,wait_key,
       completion_event_id,settled_sequence,created_at_utc,settled_at_utc,settlement_kind,
       selected_session_id,selected_action_kind
FROM rp_shared_rounds;

CREATE TABLE rp_shared_round_participants_f7_public_publish (
  round_id TEXT NOT NULL REFERENCES rp_shared_rounds_f7_public_publish(round_id),
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  entity_id TEXT NOT NULL REFERENCES materialized_entities(entity_id),
  role TEXT NOT NULL CHECK (role IN ('human','external')),
  control_generation INTEGER NOT NULL CHECK (control_generation >= 0),
  controller_instance_id TEXT NOT NULL,
  horizon_world_time TEXT NOT NULL DEFAULT '',
  submission_key TEXT NOT NULL DEFAULT '',
  request_hash TEXT NOT NULL DEFAULT '',
  submitted_at_utc TEXT,
  PRIMARY KEY(round_id,session_id),
  UNIQUE(round_id,entity_id),
  UNIQUE(round_id,principal_id),
  CHECK ((horizon_world_time='')=(submission_key='' AND request_hash='' AND submitted_at_utc IS NULL))
) STRICT;

INSERT INTO rp_shared_round_participants_f7_public_publish
  (round_id,session_id,principal_id,entity_id,role,control_generation,
   controller_instance_id,horizon_world_time,submission_key,request_hash,submitted_at_utc)
SELECT round_id,session_id,principal_id,entity_id,role,control_generation,
       controller_instance_id,horizon_world_time,submission_key,request_hash,submitted_at_utc
FROM rp_shared_round_participants;

CREATE TABLE rp_shared_round_actions_f7_public_publish (
  round_id TEXT NOT NULL REFERENCES rp_shared_rounds_f7_public_publish(round_id),
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  submission_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  request_json TEXT NOT NULL,
  submitted_at_utc TEXT NOT NULL,
  action_kind TEXT NOT NULL DEFAULT 'speech' CHECK (action_kind IN ('speech','move','sleep_start','sleep_end','work_task','information_send','information_stance','information_relay','information_public_access','information_organization_access','information_public_publish')),
  PRIMARY KEY(round_id,session_id),
  FOREIGN KEY(round_id,session_id) REFERENCES rp_shared_round_participants_f7_public_publish(round_id,session_id)
) STRICT;

INSERT INTO rp_shared_round_actions_f7_public_publish
  (round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc,action_kind)
SELECT round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc,action_kind
FROM rp_shared_round_actions;

DROP TABLE rp_shared_round_actions;
DROP TABLE rp_shared_round_participants;
DROP TABLE rp_shared_rounds;
ALTER TABLE rp_shared_rounds_f7_public_publish RENAME TO rp_shared_rounds;
ALTER TABLE rp_shared_round_participants_f7_public_publish RENAME TO rp_shared_round_participants;
ALTER TABLE rp_shared_round_actions_f7_public_publish RENAME TO rp_shared_round_actions;

CREATE UNIQUE INDEX ux_rp_shared_round_one_active
ON rp_shared_rounds(instance_id,branch_id) WHERE status IN ('open','advancing');

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f7-shared-public-notice-publish-053-2026-09-26','2026-09-26T00:00:00Z');
