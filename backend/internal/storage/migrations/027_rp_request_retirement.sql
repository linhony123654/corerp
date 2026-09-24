-- Application request acceptance fences, not world Events or projections.
-- Empty scope is the principal-scoped session-open key namespace.
CREATE TABLE rp_request_retirements (
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  operation TEXT NOT NULL CHECK (operation IN ('open','dialogue','wait','move','social')),
  session_scope TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
  retired_at_utc TEXT NOT NULL,
  PRIMARY KEY (principal_id, operation, session_scope, idempotency_key),
  CHECK ((operation = 'open' AND session_scope = '') OR (operation <> 'open' AND session_scope <> ''))
) STRICT;
CREATE TRIGGER rp_request_retirement_no_update BEFORE UPDATE ON rp_request_retirements
BEGIN SELECT RAISE(ABORT,'RP_REQUEST_RETIREMENT_IMMUTABLE'); END;
CREATE TRIGGER rp_request_retirement_no_delete BEFORE DELETE ON rp_request_retirements
BEGIN SELECT RAISE(ABORT,'RP_REQUEST_RETIREMENT_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp7-request-retirement-027-2026-09-24','2026-09-24T00:00:00Z');
