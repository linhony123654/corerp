-- Legacy prose remains unversioned. Composition groups describe only actual
-- committed facts; authored presentation provenance stays in the source list.
ALTER TABLE rp_narrative_renders ADD COLUMN composition_version TEXT NOT NULL DEFAULT '';
ALTER TABLE rp_narrative_renders ADD COLUMN fact_groups_json TEXT NOT NULL DEFAULT '[]'
  CHECK (json_valid(fact_groups_json) AND json_type(fact_groups_json)='array');
ALTER TABLE rp_narrative_renders ADD COLUMN fact_event_ids_json TEXT NOT NULL DEFAULT '[]'
  CHECK (json_valid(fact_event_ids_json) AND json_type(fact_event_ids_json)='array');

ALTER TABLE rp_turn_runs ADD COLUMN narrative_composition_version TEXT NOT NULL DEFAULT '';
ALTER TABLE rp_turn_runs ADD COLUMN narrative_fact_groups_json TEXT NOT NULL DEFAULT '[]'
  CHECK (json_valid(narrative_fact_groups_json) AND json_type(narrative_fact_groups_json)='array');
ALTER TABLE rp_turn_runs ADD COLUMN narrative_fact_event_ids_json TEXT NOT NULL DEFAULT '[]'
  CHECK (json_valid(narrative_fact_event_ids_json) AND json_type(narrative_fact_event_ids_json)='array');

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-narrative-composition-077-2026-10-01','2026-10-01T00:00:00Z');
