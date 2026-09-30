-- Conversation presentation boundary only. Canonical Events, projections,
-- character knowledge and world time remain unchanged.
ALTER TABLE rp_sessions ADD COLUMN chapter_start_sequence INTEGER NOT NULL DEFAULT 0 CHECK (chapter_start_sequence >= 0);

CREATE TABLE rp_session_chapter_resets (
  session_id TEXT NOT NULL REFERENCES rp_sessions(session_id),
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
  request_hash TEXT NOT NULL,
  chapter_start_sequence INTEGER NOT NULL CHECK (chapter_start_sequence >= 0),
  created_at_utc TEXT NOT NULL,
  PRIMARY KEY (session_id,idempotency_key),
  UNIQUE (principal_id,idempotency_key)
) STRICT;

CREATE TRIGGER rp_session_chapter_reset_no_update BEFORE UPDATE ON rp_session_chapter_resets
BEGIN SELECT RAISE(ABORT,'RP_SESSION_CHAPTER_RESET_IMMUTABLE'); END;
CREATE TRIGGER rp_session_chapter_reset_no_delete BEFORE DELETE ON rp_session_chapter_resets
BEGIN SELECT RAISE(ABORT,'RP_SESSION_CHAPTER_RESET_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-session-chapters-063-2026-09-27','2026-09-27T00:00:00Z');
