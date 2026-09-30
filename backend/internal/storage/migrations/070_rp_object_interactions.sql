-- Authored reach anchors and stock-backed individual objects. All mutable rows
-- are projections of committed Events, not a second source of world authority.
CREATE TABLE rp_object_anchors (
  anchor_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  zone_key TEXT NOT NULL,
  anchor_code TEXT NOT NULL,
  display_name TEXT NOT NULL,
  near_entity_id TEXT REFERENCES agent_profiles(agent_id),
  definition_event_id TEXT NOT NULL REFERENCES events(event_id),
  UNIQUE (instance_id,branch_id,place_id,zone_key,anchor_code),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id)
) STRICT;
CREATE INDEX ix_rp_object_anchors_place ON rp_object_anchors(instance_id,branch_id,place_id,zone_key);

-- A declaration identifies an existing actor's holder stock and a genuine SKU.
-- It cannot create stock, declare a held object, or rename a SKU on staging.
CREATE TABLE rp_object_sources (
  source_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  anchor_id TEXT NOT NULL REFERENCES rp_object_anchors(anchor_id),
  owner_actor_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  inventory_location_id TEXT NOT NULL REFERENCES stock_locations(location_id),
  sku_id TEXT NOT NULL REFERENCES product_skus(sku_id),
  unit_minor INTEGER NOT NULL CHECK (unit_minor > 0),
  display_name TEXT NOT NULL,
  definition_event_id TEXT NOT NULL REFERENCES events(event_id),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id)
) STRICT;
CREATE INDEX ix_rp_object_sources_owner ON rp_object_sources(instance_id,branch_id,owner_actor_id);

-- A physical single unit is serialized from ordinary inventory to a dedicated
-- holder escrow. Transfers move exactly this unit between actor-owned escrows;
-- an aggregate inventory balance alone never proves its physical location.
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
  physical_state TEXT NOT NULL CHECK (physical_state IN ('held','placed')),
  holder_actor_id TEXT REFERENCES agent_profiles(agent_id),
  anchor_id TEXT REFERENCES rp_object_anchors(anchor_id),
  place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  zone_key TEXT NOT NULL,
  stage_event_id TEXT NOT NULL REFERENCES events(event_id),
  last_event_id TEXT NOT NULL REFERENCES events(event_id),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((physical_state='held' AND holder_actor_id=owner_actor_id AND anchor_id IS NULL)
    OR (physical_state='placed' AND holder_actor_id IS NULL AND anchor_id IS NOT NULL))
) STRICT;
CREATE INDEX ix_rp_objects_local ON rp_objects(instance_id,branch_id,place_id,zone_key);
CREATE INDEX ix_rp_objects_owner ON rp_objects(instance_id,branch_id,owner_actor_id);

CREATE TABLE rp_object_escrows (
  object_id TEXT NOT NULL REFERENCES rp_objects(object_id),
  owner_actor_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  location_id TEXT NOT NULL UNIQUE REFERENCES stock_locations(location_id),
  PRIMARY KEY (object_id,owner_actor_id)
) STRICT;

-- Consent is a distinct state transition, not a stock movement. One active
-- offer per item; refusal/cancellation are durable facts rather than deletion.
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
CREATE UNIQUE INDEX ux_rp_object_active_offer ON rp_object_offers(object_id)
 WHERE status IN ('offered','accepted_pending_transfer');
CREATE INDEX ix_rp_object_offers_target ON rp_object_offers(instance_id,branch_id,target_actor_id,status);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-object-interactions-070-2026-09-27','2026-09-27T00:00:00Z');
