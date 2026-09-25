-- F2 timed directed edge overlay on existing RP reachability.
CREATE TABLE rp_timed_edges (
  edge_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  from_place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  to_place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  segment_place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  duration_minutes INTEGER NOT NULL CHECK (duration_minutes BETWEEN 1 AND 360),
  definition_event_id TEXT NOT NULL REFERENCES events(event_id),
  UNIQUE (instance_id,branch_id,from_place_id,to_place_id),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK (from_place_id<>to_place_id AND from_place_id<>segment_place_id AND to_place_id<>segment_place_id)
) STRICT;

CREATE TABLE rp_journeys (
  journey_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  agent_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  edge_id TEXT NOT NULL REFERENCES rp_timed_edges(edge_id),
  from_place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  to_place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  segment_place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  started_at TEXT NOT NULL,
  scheduled_arrival_at TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active','arrived','cancelled')),
  start_event_id TEXT NOT NULL REFERENCES events(event_id),
  arrival_schedule_id TEXT NOT NULL UNIQUE REFERENCES agent_schedule_entries(schedule_id),
  arrival_item_id TEXT NOT NULL UNIQUE REFERENCES scheduler_items(scheduler_item_id),
  resolved_event_id TEXT REFERENCES events(event_id),
  UNIQUE (instance_id,branch_id,journey_id),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK (from_place_id<>to_place_id AND from_place_id<>segment_place_id AND to_place_id<>segment_place_id),
  CHECK ((status='active' AND resolved_event_id IS NULL) OR (status<>'active' AND resolved_event_id IS NOT NULL))
) STRICT;

CREATE UNIQUE INDEX ux_rp_journey_one_active_agent
ON rp_journeys(instance_id,branch_id,agent_id) WHERE status='active';

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f2-spatial-journeys-033-2026-09-25','2026-09-25T00:00:00Z');
