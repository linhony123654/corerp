-- Application narration boundary only. Existing sessions keep their historical
-- zero floor; events, observations, character memory and chapter history stay intact.
ALTER TABLE rp_sessions ADD COLUMN opened_sequence INTEGER NOT NULL DEFAULT 0 CHECK (opened_sequence >= 0);

CREATE TRIGGER rp_session_opened_sequence_immutable
BEFORE UPDATE OF opened_sequence ON rp_sessions
WHEN NEW.opened_sequence <> OLD.opened_sequence
BEGIN SELECT RAISE(ABORT, 'RP_SESSION_OPENED_SEQUENCE_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-session-opened-window-079-2026-10-01','2026-10-01T00:00:00Z');
