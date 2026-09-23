-- M2 branch-scoped background economy definitions; all balances remain projections of facts.
CREATE TABLE m2_economic_actors (
  actor_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('employer', 'landlord', 'store', 'supplier')),
  cash_account_id TEXT NOT NULL UNIQUE,
  stock_location_id TEXT UNIQUE,
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id),
  FOREIGN KEY (cash_account_id) REFERENCES accounts(account_id),
  FOREIGN KEY (stock_location_id) REFERENCES stock_locations(location_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_cohort_contracts (
  contract_id TEXT PRIMARY KEY,
  cohort_id TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('wage', 'rent', 'food')),
  participant_count INTEGER NOT NULL CHECK (participant_count > 0),
  unit_rate_minor INTEGER NOT NULL CHECK (unit_rate_minor > 0),
  currency_id TEXT NOT NULL,
  effective_from TEXT NOT NULL,
  effective_until TEXT,
  definition_event_id TEXT NOT NULL,
  UNIQUE (cohort_id, actor_id, kind, effective_from),
  CHECK (effective_until IS NULL OR effective_until > effective_from),
  FOREIGN KEY (cohort_id) REFERENCES cohorts(cohort_id),
  FOREIGN KEY (actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (currency_id) REFERENCES currencies(currency_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_economic_obligations (
  obligation_id TEXT PRIMARY KEY,
  contract_id TEXT NOT NULL,
  period_start TEXT NOT NULL,
  period_end TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('wage', 'rent')),
  amount_due_minor INTEGER NOT NULL CHECK (amount_due_minor > 0),
  amount_paid_minor INTEGER NOT NULL DEFAULT 0 CHECK (amount_paid_minor >= 0 AND amount_paid_minor <= amount_due_minor),
  status TEXT NOT NULL CHECK (status IN ('accrued', 'partial', 'paid', 'overdue')),
  defining_event_id TEXT NOT NULL UNIQUE,
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence > 0),
  UNIQUE (contract_id, period_start, period_end, kind),
  CHECK (period_end > period_start),
  FOREIGN KEY (contract_id) REFERENCES m2_cohort_contracts(contract_id),
  FOREIGN KEY (defining_event_id) REFERENCES events(event_id)
) STRICT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-background-economy-008-2026-09-23', '2026-09-23T00:00:00Z');
