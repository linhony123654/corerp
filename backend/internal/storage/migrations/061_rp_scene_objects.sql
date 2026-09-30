-- Typed scene-object projection. Definitions are sourced by the immutable
-- Studio genesis and StudioSpatialPrepared event; later state is derived only
-- from RPSceneObjectStateChanged events.
CREATE TABLE rp_scene_objects (
  object_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  object_key TEXT NOT NULL,
  display_name TEXT NOT NULL,
  object_kind TEXT NOT NULL CHECK (object_kind IN ('door','container','light')),
  place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  state_code TEXT NOT NULL CHECK (state_code IN ('open','closed','on','off')),
  definition_event_id TEXT NOT NULL REFERENCES events(event_id),
  state_event_id TEXT NOT NULL REFERENCES events(event_id),
  projection_version INTEGER NOT NULL CHECK (projection_version >= 0),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  UNIQUE(instance_id,branch_id,object_key),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id)
) STRICT;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-scene-objects-061-2026-09-27','2026-09-27T00:00:00Z');
