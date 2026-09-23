-- RP-1B reachability definitions; positions remain in agent_movements/agent_positions.
CREATE TABLE rp_place_links (
  link_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  from_place_id TEXT NOT NULL,
  to_place_id TEXT NOT NULL,
  definition_event_id TEXT NOT NULL,
  UNIQUE (instance_id, branch_id, from_place_id, to_place_id),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id),
  FOREIGN KEY (from_place_id) REFERENCES agent_places(place_id),
  FOREIGN KEY (to_place_id) REFERENCES agent_places(place_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id),
  CHECK (from_place_id <> to_place_id)
) STRICT;

CREATE INDEX ix_rp_place_links_from
ON rp_place_links(instance_id, branch_id, from_place_id, to_place_id);

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp1-routes-021-2026-09-23', '2026-09-23T00:00:00Z');
