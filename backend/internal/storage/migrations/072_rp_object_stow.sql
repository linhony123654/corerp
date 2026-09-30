-- Stowing is a terminal, Event-backed exit from physical object escrow. Preserve
-- populated child rows while rebuilding the 050 CHECK under enforced foreign keys.
CREATE TEMP TABLE rp_object_escrows_052 AS SELECT * FROM rp_object_escrows;
CREATE TEMP TABLE rp_object_offers_052 AS SELECT * FROM rp_object_offers;
CREATE TEMP TABLE rp_objects_052 AS SELECT * FROM rp_objects;
DROP TABLE rp_object_offers;
DROP TABLE rp_object_escrows;
DROP TABLE rp_objects;

CREATE TABLE rp_objects (
  object_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  source_id TEXT NOT NULL REFERENCES rp_object_sources(source_id),
  sku_id TEXT NOT NULL REFERENCES product_skus(sku_id),
  unit_minor INTEGER NOT NULL CHECK (unit_minor > 0),
  display_name TEXT NOT NULL,
  owner_actor_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  escrow_location_id TEXT NOT NULL REFERENCES stock_locations(location_id),
  physical_state TEXT NOT NULL CHECK (physical_state IN ('held','placed','stowed')),
  holder_actor_id TEXT REFERENCES agent_profiles(agent_id),
  anchor_id TEXT REFERENCES rp_object_anchors(anchor_id),
  place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  zone_key TEXT NOT NULL,
  stage_event_id TEXT NOT NULL REFERENCES events(event_id),
  last_event_id TEXT NOT NULL REFERENCES events(event_id),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((physical_state='held' AND holder_actor_id=owner_actor_id AND anchor_id IS NULL)
    OR (physical_state='placed' AND holder_actor_id IS NULL AND anchor_id IS NOT NULL)
    OR (physical_state='stowed' AND holder_actor_id IS NULL AND anchor_id IS NULL))
) STRICT;
INSERT INTO rp_objects SELECT * FROM rp_objects_052;
DROP TABLE rp_objects_052;
CREATE INDEX ix_rp_objects_local ON rp_objects(instance_id,branch_id,place_id,zone_key);
CREATE INDEX ix_rp_objects_owner ON rp_objects(instance_id,branch_id,owner_actor_id);

CREATE TABLE rp_object_escrows (
  object_id TEXT NOT NULL REFERENCES rp_objects(object_id),
  owner_actor_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  location_id TEXT NOT NULL UNIQUE REFERENCES stock_locations(location_id),
  PRIMARY KEY (object_id,owner_actor_id)
) STRICT;
INSERT INTO rp_object_escrows SELECT * FROM rp_object_escrows_052;
DROP TABLE rp_object_escrows_052;

CREATE TABLE rp_object_offers (
  offer_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  object_id TEXT NOT NULL REFERENCES rp_objects(object_id),
  from_actor_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  target_actor_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  status TEXT NOT NULL CHECK (status IN ('offered','accepted_pending_transfer','refused','cancelled','transferred')),
  offer_event_id TEXT NOT NULL REFERENCES events(event_id),
  response_event_id TEXT REFERENCES events(event_id),
  terminal_event_id TEXT REFERENCES events(event_id),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK (from_actor_id<>target_actor_id)
) STRICT;
INSERT INTO rp_object_offers SELECT * FROM rp_object_offers_052;
DROP TABLE rp_object_offers_052;
CREATE UNIQUE INDEX ux_rp_object_active_offer ON rp_object_offers(object_id)
 WHERE status IN ('offered','accepted_pending_transfer');
CREATE INDEX ix_rp_object_offers_target ON rp_object_offers(instance_id,branch_id,target_actor_id,status);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-object-stow-072-2026-09-28','2026-09-28T00:00:00Z');
