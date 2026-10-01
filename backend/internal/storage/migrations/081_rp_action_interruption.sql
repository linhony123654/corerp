-- Application terminal outcome only: no world effects or alternate turn owner.
ALTER TABLE rp_turn_runs ADD COLUMN interruption_event_id TEXT REFERENCES events(event_id);
ALTER TABLE rp_turn_runs ADD COLUMN interrupted_listener_ids_json TEXT NOT NULL DEFAULT '[]'
  CHECK (json_valid(interrupted_listener_ids_json) AND json_type(interrupted_listener_ids_json)='array');
INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-action-interruption-081-2026-10-02','2026-10-02T00:00:00Z');
