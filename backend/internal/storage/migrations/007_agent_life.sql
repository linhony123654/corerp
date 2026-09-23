-- CoreRP M2 bounded Agent life
-- Adds branch-scoped profiles, schedules, spatial facts, observations and limited knowledge.

CREATE TABLE agent_places (
  place_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  place_kind TEXT NOT NULL CHECK (place_kind IN ('home', 'work', 'public')),
  status TEXT NOT NULL CHECK (status IN ('active', 'closed')),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE agent_profiles (
  agent_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  principal_id TEXT NOT NULL UNIQUE,
  agent_level TEXT NOT NULL CHECK (agent_level IN ('L1', 'L2')),
  goal_code TEXT NOT NULL,
  action_budget_per_day INTEGER NOT NULL CHECK (action_budget_per_day > 0),
  status TEXT NOT NULL CHECK (status IN ('active', 'paused')),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (agent_id) REFERENCES materialized_entities(entity_id),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id),
  FOREIGN KEY (principal_id) REFERENCES principals(principal_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE agent_schedule_entries (
  schedule_id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  world_time TEXT NOT NULL,
  place_id TEXT NOT NULL,
  activity_code TEXT NOT NULL,
  declared_priority INTEGER NOT NULL,
  scheduler_item_id TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'cancelled')),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (agent_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (place_id) REFERENCES agent_places(place_id),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE agent_positions (
  agent_id TEXT PRIMARY KEY,
  place_id TEXT NOT NULL,
  activity_code TEXT NOT NULL,
  effective_world_time TEXT NOT NULL,
  projection_version INTEGER NOT NULL CHECK (projection_version >= 0),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  FOREIGN KEY (agent_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (place_id) REFERENCES agent_places(place_id)
) STRICT;

CREATE TABLE agent_movements (
  movement_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  from_place_id TEXT,
  to_place_id TEXT NOT NULL,
  schedule_id TEXT,
  activity_code TEXT NOT NULL,
  world_time TEXT NOT NULL,
  movement_kind TEXT NOT NULL CHECK (movement_kind IN ('initialize', 'scheduled')),
  UNIQUE (event_id, agent_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id),
  FOREIGN KEY (agent_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (from_place_id) REFERENCES agent_places(place_id),
  FOREIGN KEY (to_place_id) REFERENCES agent_places(place_id),
  FOREIGN KEY (schedule_id) REFERENCES agent_schedule_entries(schedule_id),
  CHECK (from_place_id IS NULL OR from_place_id <> to_place_id),
  CHECK (
    (movement_kind = 'initialize' AND from_place_id IS NULL AND schedule_id IS NULL)
    OR
    (movement_kind = 'scheduled' AND from_place_id IS NOT NULL AND schedule_id IS NOT NULL)
  )
) STRICT;

CREATE TABLE observation_records (
  observation_id TEXT PRIMARY KEY,
  source_event_id TEXT NOT NULL,
  observer_agent_id TEXT NOT NULL,
  subject_agent_id TEXT NOT NULL,
  place_id TEXT NOT NULL,
  channel TEXT NOT NULL CHECK (channel IN ('co_location')),
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

CREATE TABLE agent_knowledge (
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
  FOREIGN KEY (observation_id) REFERENCES observation_records(observation_id),
  CHECK (observer_agent_id <> subject_agent_id)
) STRICT;

CREATE TABLE agent_runs (
  run_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  target_world_time TEXT NOT NULL,
  processed_items INTEGER NOT NULL DEFAULT 0 CHECK (processed_items >= 0),
  pending_due INTEGER NOT NULL DEFAULT 0 CHECK (pending_due >= 0),
  status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'budget_exhausted', 'failed')),
  started_at_utc TEXT NOT NULL,
  finished_at_utc TEXT,
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id)
) STRICT;

CREATE INDEX ix_agent_schedule_stable_order
ON agent_schedule_entries(world_time, declared_priority, scheduler_item_id);

CREATE INDEX ix_agent_positions_place
ON agent_positions(place_id, agent_id);

CREATE INDEX ix_observation_records_observer_time
ON observation_records(observer_agent_id, observed_world_time, observation_id);

CREATE INDEX ix_agent_knowledge_subject
ON agent_knowledge(observer_agent_id, subject_agent_id, claim_key);

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-agent-life-007-2026-09-22', '2026-09-22T00:00:00Z');
