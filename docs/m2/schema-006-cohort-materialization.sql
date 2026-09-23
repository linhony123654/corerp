-- CoreRP M2 Cohort materialization
-- Adds aggregate/named population projections and immutable materialization lineage.

CREATE TABLE cohorts (
  cohort_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  population_count INTEGER NOT NULL CHECK (population_count >= 0),
  asset_account_id TEXT NOT NULL,
  receivable_account_id TEXT NOT NULL,
  liability_account_id TEXT NOT NULL,
  inventory_location_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  sku_id TEXT NOT NULL,
  allocation_algorithm_version TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'depleted')),
  projection_version INTEGER NOT NULL CHECK (projection_version >= 0),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id),
  FOREIGN KEY (asset_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (receivable_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (liability_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (inventory_location_id) REFERENCES stock_locations(location_id),
  FOREIGN KEY (sku_id) REFERENCES product_skus(sku_id),
  CHECK (asset_account_id <> receivable_account_id),
  CHECK (asset_account_id <> liability_account_id),
  CHECK (receivable_account_id <> liability_account_id)
) STRICT;

CREATE TABLE materialized_entities (
  entity_id TEXT PRIMARY KEY,
  source_cohort_id TEXT NOT NULL,
  materialization_id TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  population_count INTEGER NOT NULL CHECK (population_count >= 0),
  asset_account_id TEXT NOT NULL,
  receivable_account_id TEXT NOT NULL,
  liability_account_id TEXT NOT NULL,
  inventory_location_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  sku_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'dematerialized')),
  projection_version INTEGER NOT NULL CHECK (projection_version >= 0),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  FOREIGN KEY (source_cohort_id) REFERENCES cohorts(cohort_id),
  FOREIGN KEY (asset_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (receivable_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (liability_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (inventory_location_id) REFERENCES stock_locations(location_id),
  FOREIGN KEY (sku_id) REFERENCES product_skus(sku_id),
  CHECK (asset_account_id <> receivable_account_id),
  CHECK (asset_account_id <> liability_account_id),
  CHECK (receivable_account_id <> liability_account_id)
) STRICT;

CREATE TABLE cohort_materializations (
  materialization_id TEXT PRIMARY KEY,
  source_cohort_id TEXT NOT NULL,
  entity_id TEXT NOT NULL UNIQUE,
  allocation_algorithm_version TEXT NOT NULL,
  materialize_command_id TEXT NOT NULL UNIQUE,
  materialize_event_id TEXT NOT NULL UNIQUE,
  materialize_sequence INTEGER NOT NULL CHECK (materialize_sequence >= 1),
  dematerialize_command_id TEXT UNIQUE,
  dematerialize_event_id TEXT UNIQUE,
  dematerialize_sequence INTEGER CHECK (dematerialize_sequence IS NULL OR dematerialize_sequence > materialize_sequence),
  population_count INTEGER NOT NULL CHECK (population_count > 0),
  asset_minor INTEGER NOT NULL CHECK (asset_minor >= 0),
  inventory_minor INTEGER NOT NULL CHECK (inventory_minor >= 0),
  receivable_minor INTEGER NOT NULL CHECK (receivable_minor >= 0),
  liability_minor INTEGER NOT NULL CHECK (liability_minor >= 0),
  status TEXT NOT NULL CHECK (status IN ('active', 'dematerialized')),
  FOREIGN KEY (source_cohort_id) REFERENCES cohorts(cohort_id),
  FOREIGN KEY (entity_id) REFERENCES materialized_entities(entity_id),
  FOREIGN KEY (materialize_command_id) REFERENCES commands(command_id),
  FOREIGN KEY (materialize_event_id) REFERENCES events(event_id),
  FOREIGN KEY (dematerialize_command_id) REFERENCES commands(command_id),
  FOREIGN KEY (dematerialize_event_id) REFERENCES events(event_id),
  CHECK (
    (status = 'active' AND dematerialize_command_id IS NULL AND dematerialize_event_id IS NULL AND dematerialize_sequence IS NULL)
    OR
    (status = 'dematerialized' AND dematerialize_command_id IS NOT NULL AND dematerialize_event_id IS NOT NULL AND dematerialize_sequence IS NOT NULL)
  )
) STRICT;

CREATE TABLE population_movements (
  movement_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  materialization_id TEXT,
  from_owner_kind TEXT CHECK (from_owner_kind IN ('cohort', 'entity')),
  from_owner_id TEXT,
  to_owner_kind TEXT CHECK (to_owner_kind IN ('cohort', 'entity')),
  to_owner_id TEXT,
  population_count INTEGER NOT NULL CHECK (population_count > 0),
  movement_kind TEXT NOT NULL CHECK (movement_kind IN ('create', 'materialize', 'dematerialize', 'destroy')),
  reason_code TEXT NOT NULL,
  FOREIGN KEY (event_id) REFERENCES events(event_id),
  FOREIGN KEY (materialization_id) REFERENCES cohort_materializations(materialization_id),
  CHECK ((from_owner_kind IS NULL) = (from_owner_id IS NULL)),
  CHECK ((to_owner_kind IS NULL) = (to_owner_id IS NULL)),
  CHECK (from_owner_id IS NOT to_owner_id OR from_owner_kind IS NOT to_owner_kind),
  CHECK (
    (movement_kind = 'create' AND from_owner_id IS NULL AND to_owner_kind = 'cohort')
    OR
    (movement_kind = 'materialize' AND from_owner_kind = 'cohort' AND to_owner_kind = 'entity' AND materialization_id IS NOT NULL)
    OR
    (movement_kind = 'dematerialize' AND from_owner_kind = 'entity' AND to_owner_kind = 'cohort' AND materialization_id IS NOT NULL)
    OR
    (movement_kind = 'destroy' AND from_owner_id IS NOT NULL AND to_owner_id IS NULL)
  )
) STRICT;

CREATE INDEX ix_cohort_materializations_source_status
ON cohort_materializations(source_cohort_id, status, materialization_id);

CREATE INDEX ix_population_movements_materialization
ON population_movements(materialization_id, movement_id);

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-cohort-materialization-006-2026-09-22', '2026-09-22T00:00:00Z');

