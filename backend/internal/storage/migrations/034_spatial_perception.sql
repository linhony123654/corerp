-- F2 same-place perception is a separate graph over bounded zones.
CREATE TABLE rp_perception_links (
  link_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  zone_a TEXT NOT NULL,
  zone_b TEXT NOT NULL,
  barrier_kind TEXT NOT NULL CHECK (barrier_kind IN ('open','door','wall')),
  barrier_state TEXT NOT NULL CHECK (barrier_state IN ('open','closed')),
  distance_m INTEGER NOT NULL CHECK (distance_m BETWEEN 0 AND 1000),
  visual_range_m INTEGER NOT NULL CHECK (visual_range_m BETWEEN 0 AND 1000),
  audio_range_m INTEGER NOT NULL CHECK (audio_range_m BETWEEN 0 AND 1000),
  definition_event_id TEXT NOT NULL REFERENCES events(event_id),
  UNIQUE(instance_id,branch_id,place_id,zone_a,zone_b),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK (zone_a<zone_b)
) STRICT;

-- A zone choice is tied to a specific entry movement Event. On leaving and
-- returning, the old choice cannot reactivate even if place_id is identical.
CREATE TABLE rp_actor_zones (
  agent_id TEXT PRIMARY KEY REFERENCES agent_profiles(agent_id),
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  entry_event_id TEXT NOT NULL REFERENCES events(event_id),
  zone_key TEXT NOT NULL,
  source_event_id TEXT NOT NULL REFERENCES events(event_id),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id)
) STRICT;

ALTER TABLE rp_utterances ADD COLUMN delivery_channel TEXT NOT NULL DEFAULT 'voice'
  CHECK (delivery_channel IN ('voice','whisper','shout'));

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f2-spatial-perception-034-2026-09-25','2026-09-25T00:00:00Z');
