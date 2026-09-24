-- Activation receipts pin the existing Rule Epoch and immutable installed content.
CREATE TRIGGER studio_activation_event_no_update BEFORE UPDATE ON events
WHEN OLD.event_type='StudioPackagesActivated' OR NEW.event_type='StudioPackagesActivated'
BEGIN SELECT RAISE(ABORT,'STUDIO_ACTIVATION_IMMUTABLE'); END;
CREATE TRIGGER studio_activation_event_no_delete BEFORE DELETE ON events
WHEN OLD.event_type='StudioPackagesActivated'
BEGIN SELECT RAISE(ABORT,'STUDIO_ACTIVATION_IMMUTABLE'); END;
CREATE TRIGGER studio_activation_event_no_replace BEFORE INSERT ON events
WHEN EXISTS (
  SELECT 1 FROM events e
  WHERE (e.event_id=NEW.event_id
    OR (e.batch_id=NEW.batch_id AND e.batch_index=NEW.batch_index)
    OR (e.instance_id=NEW.instance_id AND e.branch_id=NEW.branch_id AND e.event_sequence=NEW.event_sequence))
    AND (e.event_type='StudioPackagesActivated' OR NEW.event_type='StudioPackagesActivated')
)
BEGIN SELECT RAISE(ABORT,'STUDIO_ACTIVATION_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp8-package-activation-029-2026-09-24','2026-09-24T00:00:00Z');
