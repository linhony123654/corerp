-- Familiarity is not implied by physical co-location, sight or hearing.
-- Presentation-only HMAC key: never returned to clients or replayed as a
-- world fact. A copied DB keeps its handles; an Event-only reconstruction may
-- assign new presentation handles without changing any authoritative ID.
CREATE TABLE rp_identity_alias_secret (
  singleton INTEGER PRIMARY KEY CHECK (singleton=1),
  secret BLOB NOT NULL CHECK (length(secret)=32)
) STRICT;
INSERT INTO rp_identity_alias_secret(singleton,secret) VALUES (1,randomblob(32));
CREATE TRIGGER rp_identity_alias_secret_no_update BEFORE UPDATE ON rp_identity_alias_secret
BEGIN SELECT RAISE(ABORT,'IDENTITY_ALIAS_SECRET_IMMUTABLE'); END;
CREATE TRIGGER rp_identity_alias_secret_no_delete BEFORE DELETE ON rp_identity_alias_secret
BEGIN SELECT RAISE(ABORT,'IDENTITY_ALIAS_SECRET_IMMUTABLE'); END;

CREATE TABLE rp_identity_familiarity (
  observer_agent_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  subject_agent_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  source_event_id TEXT NOT NULL REFERENCES events(event_id),
  learned_world_time TEXT NOT NULL,
  origin_kind TEXT NOT NULL CHECK (origin_kind IN ('legacy','demo','introduction')),
  PRIMARY KEY(observer_agent_id,subject_agent_id),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK (observer_agent_id<>subject_agent_id)
) STRICT;

CREATE TABLE rp_identity_cutovers (
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  through_sequence INTEGER NOT NULL CHECK (through_sequence>=0),
  PRIMARY KEY(instance_id,branch_id),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id)
) STRICT;

INSERT INTO rp_identity_cutovers(instance_id,branch_id,through_sequence)
SELECT instance_id,branch_id,head_sequence FROM branches;

-- Preserve old-world identity assumptions only where a prior committed
-- presence claim already identified the individual. New sight does not add
-- familiarity; introductions below require their own accepted speech Event.
INSERT INTO rp_identity_familiarity(observer_agent_id,subject_agent_id,instance_id,branch_id,source_event_id,learned_world_time,origin_kind)
SELECT k.observer_agent_id,k.subject_agent_id,a.instance_id,a.branch_id,k.source_event_id,k.learned_world_time,'legacy'
FROM agent_knowledge k JOIN agent_profiles a ON a.agent_id=k.observer_agent_id
JOIN agent_profiles b ON b.agent_id=k.subject_agent_id AND b.instance_id=a.instance_id AND b.branch_id=a.branch_id
JOIN events e ON e.event_id=k.source_event_id AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id
WHERE substr(k.claim_key,1,9)='presence:';

-- Freeze the migration bridge independently of mutable knowledge projections.
CREATE TABLE rp_identity_legacy_sources (
  observer_agent_id TEXT NOT NULL,
  subject_agent_id TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  source_event_id TEXT NOT NULL REFERENCES events(event_id),
  learned_world_time TEXT NOT NULL,
  PRIMARY KEY(observer_agent_id,subject_agent_id)
) STRICT;
INSERT INTO rp_identity_legacy_sources
SELECT observer_agent_id,subject_agent_id,instance_id,branch_id,source_event_id,learned_world_time
FROM rp_identity_familiarity WHERE origin_kind='legacy';
CREATE TRIGGER rp_identity_legacy_sources_no_update BEFORE UPDATE ON rp_identity_legacy_sources
BEGIN SELECT RAISE(ABORT,'IDENTITY_LEGACY_SOURCE_IMMUTABLE'); END;
CREATE TRIGGER rp_identity_legacy_sources_no_delete BEFORE DELETE ON rp_identity_legacy_sources
BEGIN SELECT RAISE(ABORT,'IDENTITY_LEGACY_SOURCE_IMMUTABLE'); END;

-- Older demo worlds were initialized before this projection existed. Their
-- setup Event already declared the two named participants, so retain that
-- acquaintance without treating later co-location as an introduction.
INSERT OR IGNORE INTO rp_identity_familiarity(observer_agent_id,subject_agent_id,instance_id,branch_id,source_event_id,learned_world_time,origin_kind)
SELECT 'entity_m2_rp_lin','entity_m2_rp_cai',e.instance_id,e.branch_id,e.event_id,e.world_time,'demo'
FROM events e WHERE e.event_type='RPParticipantsInitialized'
AND json_extract(e.payload,'$.player_entity_id')='entity_m2_rp_lin';
INSERT OR IGNORE INTO rp_identity_familiarity(observer_agent_id,subject_agent_id,instance_id,branch_id,source_event_id,learned_world_time,origin_kind)
SELECT 'entity_m2_rp_cai','entity_m2_rp_lin',e.instance_id,e.branch_id,e.event_id,e.world_time,'demo'
FROM events e WHERE e.event_type='RPParticipantsInitialized'
AND json_extract(e.payload,'$.player_entity_id')='entity_m2_rp_lin';

CREATE TRIGGER rp_identity_familiarity_no_update BEFORE UPDATE ON rp_identity_familiarity
BEGIN SELECT RAISE(ABORT,'IDENTITY_FAMILIARITY_IMMUTABLE'); END;
CREATE TRIGGER rp_identity_familiarity_no_delete BEFORE DELETE ON rp_identity_familiarity
BEGIN SELECT RAISE(ABORT,'IDENTITY_FAMILIARITY_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f2-spatial-identity-035-2026-09-25','2026-09-25T00:00:00Z');
