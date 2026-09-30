-- Application worker coordination only. Canonical time and scheduler effects
-- remain Events plus the existing world_clocks/scheduler projections.
CREATE TABLE rp_background_leases (
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  lease_owner TEXT NOT NULL,
  generation INTEGER NOT NULL CHECK (generation > 0),
  lease_until_utc TEXT NOT NULL,
  updated_at_utc TEXT NOT NULL,
  PRIMARY KEY(instance_id,branch_id),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id)
) STRICT;

CREATE TABLE rp_background_runs (
  run_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  lease_generation INTEGER NOT NULL CHECK (lease_generation > 0),
  from_world_time TEXT NOT NULL,
  target_world_time TEXT NOT NULL,
  scheduler_budget INTEGER NOT NULL CHECK (scheduler_budget BETWEEN 1 AND 10000),
  processed_items INTEGER NOT NULL DEFAULT 0 CHECK (processed_items >= 0),
  status TEXT NOT NULL CHECK (status IN ('running','completed','budget_exhausted','controlled','failed')),
  error_code TEXT,
  started_at_utc TEXT NOT NULL,
  finished_at_utc TEXT,
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((status='running')=(finished_at_utc IS NULL))
) STRICT;

CREATE INDEX ix_rp_background_runs_scope_started
ON rp_background_runs(instance_id,branch_id,started_at_utc DESC);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-background-progression-062-2026-09-27','2026-09-27T00:00:00Z');
