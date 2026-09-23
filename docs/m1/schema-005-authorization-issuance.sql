-- CoreRP M1 authorization and controlled issuance
-- Adds scoped grants, explicit issuance policy usage, and intervention evidence.

CREATE TABLE principals (
  principal_id TEXT PRIMARY KEY,
  principal_type TEXT NOT NULL CHECK (principal_type IN ('player', 'creator', 'operator', 'service', 'agent')),
  display_name TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'disabled'))
) STRICT;

CREATE TABLE capability_definitions (
  capability_id TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  policy_version TEXT NOT NULL
) STRICT;

CREATE TABLE capability_grants (
  grant_id TEXT PRIMARY KEY,
  principal_id TEXT NOT NULL,
  capability_id TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  subject_id TEXT NOT NULL,
  field_scope TEXT NOT NULL CHECK (json_valid(field_scope) AND json_type(field_scope) = 'array'),
  amount_limit_minor INTEGER CHECK (amount_limit_minor IS NULL OR amount_limit_minor > 0),
  status TEXT NOT NULL CHECK (status IN ('active', 'revoked')),
  definition_event_id TEXT NOT NULL,
  UNIQUE (principal_id, capability_id, instance_id, branch_id, subject_id),
  FOREIGN KEY (principal_id) REFERENCES principals(principal_id),
  FOREIGN KEY (capability_id) REFERENCES capability_definitions(capability_id),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id)
) STRICT;

CREATE TABLE issuance_policies (
  policy_id TEXT PRIMARY KEY,
  currency_id TEXT NOT NULL,
  source_account_id TEXT NOT NULL,
  capability_id TEXT NOT NULL,
  per_command_limit_minor INTEGER NOT NULL CHECK (per_command_limit_minor > 0),
  cumulative_limit_minor INTEGER NOT NULL CHECK (cumulative_limit_minor >= per_command_limit_minor),
  issued_total_minor INTEGER NOT NULL DEFAULT 0 CHECK (issued_total_minor >= 0 AND issued_total_minor <= cumulative_limit_minor),
  status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (currency_id) REFERENCES currencies(currency_id),
  FOREIGN KEY (source_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (capability_id) REFERENCES capability_definitions(capability_id)
) STRICT;

CREATE TABLE intervention_records (
  intervention_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  command_id TEXT NOT NULL UNIQUE,
  principal_id TEXT NOT NULL,
  grant_id TEXT NOT NULL,
  policy_id TEXT NOT NULL,
  target_account_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  reason_code TEXT NOT NULL,
  scope TEXT NOT NULL CHECK (json_valid(scope)),
  event_sequence INTEGER NOT NULL CHECK (event_sequence >= 1),
  FOREIGN KEY (event_id) REFERENCES events(event_id),
  FOREIGN KEY (command_id) REFERENCES commands(command_id),
  FOREIGN KEY (principal_id) REFERENCES principals(principal_id),
  FOREIGN KEY (grant_id) REFERENCES capability_grants(grant_id),
  FOREIGN KEY (policy_id) REFERENCES issuance_policies(policy_id),
  FOREIGN KEY (target_account_id, currency_id) REFERENCES accounts(account_id, currency_id)
) STRICT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m1-authorization-issuance-005-2026-09-22', '2026-09-22T00:00:00Z');
