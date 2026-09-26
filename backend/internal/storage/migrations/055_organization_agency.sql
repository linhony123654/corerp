-- Organization Agency policies and review records.
-- An organization review records evaluated evidence, deterministic policy
-- decisions and operational changes (posting freeze/unfreeze, capacity adjustment).

CREATE TABLE organization_agency_policies (
  policy_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  organization_id TEXT NOT NULL,
  manager_principal_id TEXT NOT NULL,
  review_frequency_hours INTEGER NOT NULL CHECK (review_frequency_hours > 0),
  reserve_target_minor INTEGER NOT NULL CHECK (reserve_target_minor >= 0),
  hiring_threshold_minor INTEGER NOT NULL,
  freeze_threshold_minor INTEGER NOT NULL,
  target_position_id TEXT NOT NULL,
  default_capacity INTEGER NOT NULL CHECK (default_capacity >= 0),
  status TEXT NOT NULL CHECK (status IN ('active', 'suspended', 'revoked')),
  definition_event_id TEXT NOT NULL,
  UNIQUE (instance_id, branch_id, organization_id)
) STRICT;

CREATE TABLE organization_reviews (
  review_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  organization_id TEXT NOT NULL,
  policy_id TEXT NOT NULL,
  reviewed_world_time TEXT NOT NULL,
  evidence_payload TEXT NOT NULL CHECK (json_valid(evidence_payload)),
  decision_kind TEXT NOT NULL CHECK (decision_kind IN ('no_change', 'freeze_recruitment', 'unfreeze_recruitment', 'expand_capacity', 'adjust_schedule')),
  decision_payload TEXT NOT NULL CHECK (json_valid(decision_payload)),
  event_id TEXT NOT NULL,
  FOREIGN KEY (policy_id) REFERENCES organization_agency_policies(policy_id)
) STRICT;

CREATE INDEX idx_org_reviews_scope_time ON organization_reviews(instance_id, branch_id, organization_id, reviewed_world_time);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f8-organization-agency-055-2026-09-26', '2026-09-26T00:00:00Z');
