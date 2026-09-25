-- F3 admission is not decision control. A later sourced handoff assigns
-- authority/generation; this table only binds a dedicated external principal
-- and controller instance to one existing scoped resident.
CREATE TABLE rp_external_controller_enrollments (
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  controller_instance_id TEXT NOT NULL,
  source_event_id TEXT NOT NULL UNIQUE REFERENCES events(event_id),
  enrolled_world_time TEXT NOT NULL,
  PRIMARY KEY(instance_id,branch_id,entity_id),
  UNIQUE(instance_id,branch_id,principal_id),
  UNIQUE(instance_id,branch_id,controller_instance_id),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  FOREIGN KEY(entity_id) REFERENCES materialized_entities(entity_id)
) STRICT;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f3-controller-enrollment-036-2026-09-25','2026-09-25T00:00:00Z');
