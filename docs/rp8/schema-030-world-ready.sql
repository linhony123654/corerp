-- Readiness authorizes local player control; its source cannot be rewritten.
CREATE TRIGGER studio_ready_event_no_update BEFORE UPDATE ON events
WHEN OLD.event_type='StudioWorldReady' OR NEW.event_type='StudioWorldReady'
BEGIN SELECT RAISE(ABORT,'STUDIO_READY_IMMUTABLE'); END;
CREATE TRIGGER studio_ready_event_no_delete BEFORE DELETE ON events
WHEN OLD.event_type='StudioWorldReady'
BEGIN SELECT RAISE(ABORT,'STUDIO_READY_IMMUTABLE'); END;
CREATE TRIGGER studio_ready_event_no_replace BEFORE INSERT ON events
WHEN EXISTS (
  SELECT 1 FROM events e
  WHERE (e.event_id=NEW.event_id
    OR (e.batch_id=NEW.batch_id AND e.batch_index=NEW.batch_index)
    OR (e.instance_id=NEW.instance_id AND e.branch_id=NEW.branch_id AND e.event_sequence=NEW.event_sequence))
    AND (e.event_type='StudioWorldReady' OR NEW.event_type='StudioWorldReady')
)
BEGIN SELECT RAISE(ABORT,'STUDIO_READY_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp8-world-ready-030-2026-09-24','2026-09-24T00:00:00Z');
