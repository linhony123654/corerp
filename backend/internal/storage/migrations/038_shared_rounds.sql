-- Application coordination only. The one RPWaitCompleted Event remains the
-- authority for world-time advancement and scheduler effects.
CREATE TABLE rp_shared_rounds (
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
  wait_event_id TEXT REFERENCES events(event_id),
  settled_sequence INTEGER CHECK (settled_sequence >= 1),
  created_at_utc TEXT NOT NULL,
  settled_at_utc TEXT,
  UNIQUE(instance_id,branch_id,operator_principal_id,idempotency_key),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((status='settled')=(wait_event_id IS NOT NULL AND settled_sequence IS NOT NULL AND settled_at_utc IS NOT NULL))
) STRICT;

CREATE UNIQUE INDEX ux_rp_shared_round_one_active
ON rp_shared_rounds(instance_id,branch_id) WHERE status IN ('open','advancing');

CREATE TABLE rp_shared_round_participants (
  round_id TEXT NOT NULL REFERENCES rp_shared_rounds(round_id),
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

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f3-shared-rounds-038-2026-09-25','2026-09-25T00:00:00Z');
