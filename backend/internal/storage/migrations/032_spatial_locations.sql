-- F2 stable concrete location identity. A human-readable path/name is never a key.
-- Existing places become roots; child slots are materialized only on demand.
CREATE TABLE rp_location_nodes (
  location_id TEXT PRIMARY KEY REFERENCES agent_places(place_id),
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  parent_location_id TEXT,
  slot_key TEXT NOT NULL DEFAULT '',
  readable_path TEXT NOT NULL,
  generator_version TEXT NOT NULL,
  definition_event_id TEXT NOT NULL REFERENCES events(event_id),
  UNIQUE (instance_id, branch_id, location_id),
  UNIQUE (instance_id, branch_id, parent_location_id, slot_key),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id),
  FOREIGN KEY (instance_id, branch_id, parent_location_id)
    REFERENCES rp_location_nodes(instance_id, branch_id, location_id),
  CHECK ((parent_location_id IS NULL AND slot_key = '') OR
         (parent_location_id IS NOT NULL AND length(slot_key) BETWEEN 1 AND 64))
) STRICT;

INSERT INTO rp_location_nodes(location_id,instance_id,branch_id,parent_location_id,slot_key,readable_path,generator_version,definition_event_id)
SELECT place_id,instance_id,branch_id,NULL,'','/' || place_id,'legacy',definition_event_id
FROM agent_places;

CREATE INDEX ix_rp_location_children
ON rp_location_nodes(instance_id,branch_id,parent_location_id,slot_key);

-- Half-open occupancy is reconstructed from committed movement facts. At a
-- same-world-time boundary, event sequence determines which move came later.
-- No mutable second position table can accidentally leave an actor at both
-- endpoints. The final interval remains open until another movement Event.
CREATE INDEX ix_rp_agent_movement_time
ON agent_movements(agent_id,world_time,event_id);

CREATE VIEW rp_occupancy_intervals AS
SELECT e.instance_id,e.branch_id,m.agent_id,m.to_place_id AS location_id,
       m.world_time AS entered_at,e.event_sequence AS entered_sequence,
       LEAD(m.world_time) OVER (
         PARTITION BY e.instance_id,e.branch_id,m.agent_id
         ORDER BY m.world_time,e.event_sequence
       ) AS exited_at,
       LEAD(e.event_sequence) OVER (
         PARTITION BY e.instance_id,e.branch_id,m.agent_id
         ORDER BY m.world_time,e.event_sequence
       ) AS exited_sequence,
       m.event_id AS source_event_id
FROM agent_movements m JOIN events e ON e.event_id=m.event_id
WHERE m.world_time=e.world_time;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f2-spatial-locations-032-2026-09-25','2026-09-25T00:00:00Z');
