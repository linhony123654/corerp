-- A materialized worker keeps the next unaccrued wage share; the contract's
-- aggregate employer liability is unchanged. Settlement facts are append-only.
CREATE TABLE m2_wage_participation_splits (
  materialization_id TEXT PRIMARY KEY REFERENCES cohort_materializations(materialization_id),
  contract_id TEXT NOT NULL REFERENCES m2_cohort_contracts(contract_id),
  cohort_id TEXT NOT NULL REFERENCES cohorts(cohort_id),
  entity_id TEXT NOT NULL UNIQUE REFERENCES materialized_entities(entity_id),
  worker_count INTEGER NOT NULL CHECK (worker_count = 1),
  effective_from TEXT NOT NULL,
  income_account_id TEXT NOT NULL UNIQUE REFERENCES accounts(account_id),
  split_event_id TEXT NOT NULL REFERENCES events(event_id),
  split_event_sequence INTEGER NOT NULL CHECK (split_event_sequence > 0)
) STRICT;

CREATE TABLE m2_wage_split_obligations (
  obligation_id TEXT NOT NULL REFERENCES m2_economic_obligations(obligation_id),
  claimant_kind TEXT NOT NULL CHECK (claimant_kind IN ('cohort', 'entity')),
  claimant_id TEXT NOT NULL,
  due_minor INTEGER NOT NULL CHECK (due_minor > 0),
  asset_account_id TEXT NOT NULL REFERENCES accounts(account_id),
  receivable_account_id TEXT NOT NULL REFERENCES accounts(account_id),
  income_account_id TEXT NOT NULL REFERENCES accounts(account_id),
  accrual_event_id TEXT NOT NULL REFERENCES events(event_id),
  accrual_event_sequence INTEGER NOT NULL CHECK (accrual_event_sequence > 0),
  PRIMARY KEY (obligation_id, claimant_kind, claimant_id)
) STRICT;

CREATE TABLE m2_wage_split_receipts (
  scheduler_item_id TEXT NOT NULL REFERENCES scheduler_items(scheduler_item_id),
  obligation_id TEXT NOT NULL,
  claimant_kind TEXT NOT NULL,
  claimant_id TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  event_id TEXT NOT NULL REFERENCES events(event_id),
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  PRIMARY KEY (scheduler_item_id, claimant_kind, claimant_id),
  FOREIGN KEY (obligation_id, claimant_kind, claimant_id) REFERENCES m2_wage_split_obligations(obligation_id, claimant_kind, claimant_id)
) STRICT;

CREATE TRIGGER m2_wage_participation_splits_no_update BEFORE UPDATE ON m2_wage_participation_splits BEGIN SELECT RAISE(ABORT, 'M2_WAGE_SPLIT_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_participation_splits_no_delete BEFORE DELETE ON m2_wage_participation_splits BEGIN SELECT RAISE(ABORT, 'M2_WAGE_SPLIT_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_split_obligations_no_update BEFORE UPDATE ON m2_wage_split_obligations BEGIN SELECT RAISE(ABORT, 'M2_WAGE_SLICE_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_split_obligations_no_delete BEFORE DELETE ON m2_wage_split_obligations BEGIN SELECT RAISE(ABORT, 'M2_WAGE_SLICE_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_split_receipts_no_update BEFORE UPDATE ON m2_wage_split_receipts BEGIN SELECT RAISE(ABORT, 'M2_WAGE_RECEIPT_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_split_receipts_no_delete BEFORE DELETE ON m2_wage_split_receipts BEGIN SELECT RAISE(ABORT, 'M2_WAGE_RECEIPT_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version, applied_at_utc) VALUES ('corerp-m2-wage-participation-016-2026-09-23', '2026-09-23T00:00:00Z');
