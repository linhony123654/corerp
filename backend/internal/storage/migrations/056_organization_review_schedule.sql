ALTER TABLE organization_agency_policies ADD COLUMN automatic_review INTEGER NOT NULL DEFAULT 0 CHECK (automatic_review IN (0,1));

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f8-organization-review-schedule-056-2026-09-26','2026-09-26T00:00:00Z');
