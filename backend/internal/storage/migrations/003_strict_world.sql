-- CoreRP M1 strict-world foundation
-- Adds deterministic world time, economic contracts, obligations and ruleset phases.

CREATE TABLE world_clocks (
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  current_world_time TEXT NOT NULL,
  current_day INTEGER NOT NULL CHECK (current_day >= 0),
  status TEXT NOT NULL CHECK (status IN ('ready', 'running', 'paused', 'completed')),
  projection_version INTEGER NOT NULL CHECK (projection_version >= 0),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 0),
  PRIMARY KEY (instance_id, branch_id),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id)
) STRICT;

CREATE TABLE scheduler_phases (
  phase_id TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  ruleset_hash TEXT NOT NULL
    CHECK (length(ruleset_hash) = 71 AND ruleset_hash GLOB 'sha256:[0-9a-f]*' AND lower(ruleset_hash) = ruleset_hash)
) STRICT;

CREATE TABLE economic_entities (
  entity_id TEXT PRIMARY KEY,
  entity_kind TEXT NOT NULL CHECK (entity_kind IN ('enterprise', 'employee', 'landlord', 'store', 'household', 'authority')),
  display_name TEXT NOT NULL,
  account_id TEXT,
  private_finances INTEGER NOT NULL CHECK (private_finances IN (0, 1)),
  UNIQUE (account_id),
  FOREIGN KEY (account_id) REFERENCES accounts(account_id)
) STRICT;

CREATE TABLE employment_contracts (
  contract_id TEXT PRIMARY KEY,
  employer_entity_id TEXT NOT NULL,
  employee_entity_id TEXT NOT NULL,
  employer_account_id TEXT NOT NULL,
  employee_account_id TEXT NOT NULL,
  position_id TEXT NOT NULL,
  gross_wage_minor INTEGER NOT NULL CHECK (gross_wage_minor > 0),
  currency_id TEXT NOT NULL,
  pay_period_days INTEGER NOT NULL CHECK (pay_period_days > 0),
  starts_on_day INTEGER NOT NULL CHECK (starts_on_day >= 0),
  status TEXT NOT NULL CHECK (status IN ('active', 'ended')),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (employer_entity_id) REFERENCES economic_entities(entity_id),
  FOREIGN KEY (employee_entity_id) REFERENCES economic_entities(entity_id),
  FOREIGN KEY (employer_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (employee_account_id, currency_id) REFERENCES accounts(account_id, currency_id)
) STRICT;

CREATE TABLE wage_obligations (
  obligation_id TEXT PRIMARY KEY,
  contract_id TEXT NOT NULL,
  period_start_day INTEGER NOT NULL CHECK (period_start_day >= 0),
  period_end_day INTEGER NOT NULL CHECK (period_end_day > period_start_day),
  due_world_time TEXT NOT NULL,
  amount_due_minor INTEGER NOT NULL CHECK (amount_due_minor > 0),
  amount_paid_minor INTEGER NOT NULL DEFAULT 0 CHECK (amount_paid_minor >= 0 AND amount_paid_minor <= amount_due_minor),
  status TEXT NOT NULL CHECK (status IN ('accrued', 'partially_paid', 'paid', 'arrears')),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  UNIQUE (contract_id, period_start_day, period_end_day),
  FOREIGN KEY (contract_id) REFERENCES employment_contracts(contract_id)
) STRICT;

CREATE TABLE rent_contracts (
  contract_id TEXT PRIMARY KEY,
  tenant_entity_id TEXT NOT NULL,
  landlord_entity_id TEXT NOT NULL,
  tenant_account_id TEXT NOT NULL,
  landlord_account_id TEXT NOT NULL,
  rent_minor INTEGER NOT NULL CHECK (rent_minor > 0),
  currency_id TEXT NOT NULL,
  period_days INTEGER NOT NULL CHECK (period_days > 0),
  grace_days INTEGER NOT NULL CHECK (grace_days >= 0),
  starts_on_day INTEGER NOT NULL CHECK (starts_on_day >= 0),
  status TEXT NOT NULL CHECK (status IN ('active', 'ended')),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (tenant_entity_id) REFERENCES economic_entities(entity_id),
  FOREIGN KEY (landlord_entity_id) REFERENCES economic_entities(entity_id),
  FOREIGN KEY (tenant_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (landlord_account_id, currency_id) REFERENCES accounts(account_id, currency_id)
) STRICT;

CREATE TABLE rent_obligations (
  obligation_id TEXT PRIMARY KEY,
  contract_id TEXT NOT NULL,
  period_start_day INTEGER NOT NULL CHECK (period_start_day >= 0),
  period_end_day INTEGER NOT NULL CHECK (period_end_day > period_start_day),
  due_world_time TEXT NOT NULL,
  grace_until_world_time TEXT NOT NULL,
  amount_due_minor INTEGER NOT NULL CHECK (amount_due_minor > 0),
  amount_paid_minor INTEGER NOT NULL DEFAULT 0 CHECK (amount_paid_minor >= 0 AND amount_paid_minor <= amount_due_minor),
  status TEXT NOT NULL CHECK (status IN ('due', 'partially_paid', 'paid', 'past_due')),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  UNIQUE (contract_id, period_start_day, period_end_day),
  FOREIGN KEY (contract_id) REFERENCES rent_contracts(contract_id)
) STRICT;

CREATE TABLE market_quotes (
  quote_id TEXT PRIMARY KEY,
  region_id TEXT NOT NULL,
  seller_entity_id TEXT NOT NULL,
  seller_account_id TEXT NOT NULL,
  seller_location_id TEXT NOT NULL,
  sku_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  unit_price_minor INTEGER NOT NULL CHECK (unit_price_minor > 0),
  valid_from_day INTEGER NOT NULL CHECK (valid_from_day >= 0),
  valid_until_day INTEGER NOT NULL CHECK (valid_until_day >= valid_from_day),
  max_quantity_minor INTEGER NOT NULL CHECK (max_quantity_minor > 0),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (seller_entity_id) REFERENCES economic_entities(entity_id),
  FOREIGN KEY (seller_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (seller_location_id) REFERENCES stock_locations(location_id),
  FOREIGN KEY (sku_id) REFERENCES product_skus(sku_id)
) STRICT;

CREATE TABLE household_budgets (
  budget_id TEXT PRIMARY KEY,
  household_entity_id TEXT NOT NULL,
  period_start_day INTEGER NOT NULL CHECK (period_start_day >= 0),
  period_end_day INTEGER NOT NULL CHECK (period_end_day > period_start_day),
  food_limit_minor INTEGER NOT NULL CHECK (food_limit_minor >= 0),
  rent_limit_minor INTEGER NOT NULL CHECK (rent_limit_minor >= 0),
  currency_id TEXT NOT NULL,
  definition_event_id TEXT NOT NULL,
  UNIQUE (household_entity_id, period_start_day, period_end_day),
  FOREIGN KEY (household_entity_id) REFERENCES economic_entities(entity_id),
  FOREIGN KEY (currency_id) REFERENCES currencies(currency_id)
) STRICT;

CREATE TABLE simulation_runs (
  run_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  start_day INTEGER NOT NULL CHECK (start_day >= 0),
  target_day INTEGER NOT NULL CHECK (target_day >= start_day),
  processed_items INTEGER NOT NULL DEFAULT 0 CHECK (processed_items >= 0),
  status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'budget_exhausted', 'failed')),
  started_at_utc TEXT NOT NULL,
  finished_at_utc TEXT,
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id)
) STRICT;

CREATE INDEX ix_wage_obligations_status ON wage_obligations(status, due_world_time, obligation_id);
CREATE INDEX ix_rent_obligations_status ON rent_obligations(status, due_world_time, obligation_id);

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m1-strict-world-003-2026-09-22', '2026-09-22T00:00:00Z');
