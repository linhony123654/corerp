-- CoreRP M1 recovery foundation
-- Adds durable snapshot payloads while preserving the frozen M0 baseline.

CREATE TABLE snapshot_payloads (
  snapshot_id TEXT PRIMARY KEY,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  FOREIGN KEY (snapshot_id) REFERENCES snapshots(snapshot_id) ON DELETE CASCADE
) STRICT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m1-recovery-002-2026-09-22', '2026-09-22T00:00:00Z');
