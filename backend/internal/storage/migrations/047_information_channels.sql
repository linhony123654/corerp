-- Preserve the populated observation -> knowledge FK chain while permitting
-- sourced remote delivery. A remote message is not co-location evidence.
PRAGMA defer_foreign_keys=ON;

CREATE TABLE observation_records_f7 (
  observation_id TEXT PRIMARY KEY,
  source_event_id TEXT NOT NULL,
  observer_agent_id TEXT NOT NULL,
  subject_agent_id TEXT NOT NULL,
  place_id TEXT NOT NULL,
  channel TEXT NOT NULL CHECK (channel IN ('co_location','direct_message','rumor','organization_announcement','public_notice')),
  observed_world_time TEXT NOT NULL,
  claim_key TEXT NOT NULL,
  claim_payload TEXT NOT NULL CHECK (json_valid(claim_payload)),
  UNIQUE (source_event_id, observer_agent_id, subject_agent_id, claim_key),
  FOREIGN KEY (source_event_id) REFERENCES events(event_id),
  FOREIGN KEY (observer_agent_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (subject_agent_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (place_id) REFERENCES agent_places(place_id),
  CHECK (observer_agent_id <> subject_agent_id)
) STRICT;

INSERT INTO observation_records_f7
  (observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload)
SELECT observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload
FROM observation_records;

CREATE TABLE agent_knowledge_f7 (
  observer_agent_id TEXT NOT NULL,
  claim_key TEXT NOT NULL,
  subject_agent_id TEXT NOT NULL,
  place_id TEXT NOT NULL,
  source_event_id TEXT NOT NULL,
  observation_id TEXT NOT NULL,
  learned_world_time TEXT NOT NULL,
  claim_payload TEXT NOT NULL CHECK (json_valid(claim_payload)),
  projection_version INTEGER NOT NULL CHECK (projection_version >= 0),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  PRIMARY KEY (observer_agent_id, claim_key),
  FOREIGN KEY (observer_agent_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (subject_agent_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (place_id) REFERENCES agent_places(place_id),
  FOREIGN KEY (source_event_id) REFERENCES events(event_id),
  FOREIGN KEY (observation_id) REFERENCES observation_records_f7(observation_id),
  CHECK (observer_agent_id <> subject_agent_id)
) STRICT;

INSERT INTO agent_knowledge_f7
  (observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence)
SELECT observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence
FROM agent_knowledge;

DROP TABLE agent_knowledge;
DROP TABLE observation_records;
ALTER TABLE observation_records_f7 RENAME TO observation_records;
ALTER TABLE agent_knowledge_f7 RENAME TO agent_knowledge;

CREATE INDEX ix_observation_records_observer_time
ON observation_records(observer_agent_id, observed_world_time, observation_id);
CREATE INDEX ix_agent_knowledge_subject
ON agent_knowledge(observer_agent_id, subject_agent_id, claim_key);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f7-information-channels-047-2026-09-26','2026-09-26T00:00:00Z');
