-- An enrolled external controller is active only after a sourced assignment.
-- Generation is pinned by new RP sessions; no legacy session is silently adopted.
CREATE TABLE rp_controller_authorities (
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  controller_instance_id TEXT NOT NULL,
  generation INTEGER NOT NULL CHECK (generation > 0),
  source_event_id TEXT NOT NULL UNIQUE REFERENCES events(event_id),
  assigned_world_time TEXT NOT NULL,
  PRIMARY KEY(instance_id,branch_id,entity_id),
  FOREIGN KEY(instance_id,branch_id,entity_id) REFERENCES rp_external_controller_enrollments(instance_id,branch_id,entity_id)
) STRICT;

ALTER TABLE rp_sessions ADD COLUMN control_generation INTEGER NOT NULL DEFAULT 0 CHECK (control_generation >= 0);
ALTER TABLE rp_sessions ADD COLUMN controller_instance_id TEXT NOT NULL DEFAULT '';

-- Application-only disposition for a heard Entity whose decision belongs to
-- an external controller or an active Human session. Speech/hearing stay intact.
CREATE TABLE rp_turn_listener_skips (
  turn_run_id TEXT NOT NULL REFERENCES rp_turn_runs(turn_run_id),
  npc_entity_id TEXT NOT NULL,
  owner_source_event_id TEXT NOT NULL REFERENCES events(event_id),
  PRIMARY KEY(turn_run_id,npc_entity_id)
) STRICT;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f3-controller-authority-037-2026-09-25','2026-09-25T00:00:00Z');
