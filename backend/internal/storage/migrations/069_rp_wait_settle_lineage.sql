-- Non-authoritative provenance for the activity events produced by a wait's
-- derived sweep. Older completed markers remain unknown; never infer ownership.
ALTER TABLE rp_wait_activity_settlements ADD COLUMN first_sequence INTEGER CHECK (first_sequence IS NULL OR first_sequence > 0);
ALTER TABLE rp_wait_activity_settlements ADD COLUMN last_sequence INTEGER CHECK (last_sequence IS NULL OR last_sequence >= first_sequence);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-wait-settle-lineage-069-2026-09-27','2026-09-27T00:00:00Z');
