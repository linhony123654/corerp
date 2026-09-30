-- Seed declared personas onto agent profiles so decision inputs can carry a
-- character's own background without abusing self-referential knowledge rows
-- (agent_knowledge forbids observer == subject).
ALTER TABLE agent_profiles ADD COLUMN persona_text TEXT NOT NULL DEFAULT '';

-- Allow familiarity rows whose source is a world's declared acquaintance set
-- (RPIdentitiesDeclared). SQLite cannot widen a CHECK in place; rebuild the
-- table and its immutability triggers, preserving rows.
CREATE TABLE rp_identity_familiarity_new (
  observer_agent_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  subject_agent_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  source_event_id TEXT NOT NULL REFERENCES events(event_id),
  learned_world_time TEXT NOT NULL,
  origin_kind TEXT NOT NULL CHECK (origin_kind IN ('legacy','demo','introduction','declared')),
  PRIMARY KEY(observer_agent_id,subject_agent_id),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK (observer_agent_id<>subject_agent_id)
) STRICT;
INSERT INTO rp_identity_familiarity_new SELECT * FROM rp_identity_familiarity;
DROP TABLE rp_identity_familiarity;
ALTER TABLE rp_identity_familiarity_new RENAME TO rp_identity_familiarity;
CREATE TRIGGER rp_identity_familiarity_no_update BEFORE UPDATE ON rp_identity_familiarity
BEGIN SELECT RAISE(ABORT,'IDENTITY_FAMILIARITY_IMMUTABLE'); END;
CREATE TRIGGER rp_identity_familiarity_no_delete BEFORE DELETE ON rp_identity_familiarity
BEGIN SELECT RAISE(ABORT,'IDENTITY_FAMILIARITY_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-studio-life-seeding-057-2026-09-27','2026-09-27T00:00:00Z');
