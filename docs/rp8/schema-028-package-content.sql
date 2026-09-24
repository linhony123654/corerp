-- Installed declarative package content is pinned source data, not a projection.
-- Keep legacy Event update behavior unchanged outside the new package boundary.
CREATE TRIGGER studio_package_event_no_update BEFORE UPDATE ON events
WHEN OLD.event_type='StudioPackageInstalled' OR NEW.event_type='StudioPackageInstalled'
BEGIN SELECT RAISE(ABORT,'STUDIO_PACKAGE_CONTENT_IMMUTABLE'); END;
CREATE TRIGGER studio_package_event_no_delete BEFORE DELETE ON events
WHEN OLD.event_type='StudioPackageInstalled'
BEGIN SELECT RAISE(ABORT,'STUDIO_PACKAGE_CONTENT_IMMUTABLE'); END;

-- SQLite REPLACE need not fire DELETE triggers when recursive_triggers is off.
-- Protect every Event uniqueness key before a replacement can happen.
CREATE TRIGGER studio_package_event_no_replace BEFORE INSERT ON events
WHEN EXISTS (
  SELECT 1 FROM events e
  WHERE (e.event_id=NEW.event_id
    OR (e.batch_id=NEW.batch_id AND e.batch_index=NEW.batch_index)
    OR (e.instance_id=NEW.instance_id AND e.branch_id=NEW.branch_id AND e.event_sequence=NEW.event_sequence))
    AND (e.event_type='StudioPackageInstalled' OR NEW.event_type='StudioPackageInstalled')
)
BEGIN SELECT RAISE(ABORT,'STUDIO_PACKAGE_CONTENT_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp8-package-content-028-2026-09-24','2026-09-24T00:00:00Z');
