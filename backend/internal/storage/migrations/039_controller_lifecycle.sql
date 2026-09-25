-- Keep the last sourced generation after release. A missing row means the
-- Entity has never been assigned, not that an old controller may return.
ALTER TABLE rp_controller_authorities ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
  CHECK (status IN ('active','released'));

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f3-controller-lifecycle-039-2026-09-25','2026-09-25T00:00:00Z');
