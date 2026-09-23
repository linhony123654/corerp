-- CoreRP M0 SQLite schema baseline
-- Status: frozen M0 contract; embedded by the M1 kernel as migration 001.
-- SQLite target: a version with STRICT tables, generated/partial indexes and JSON functions.

PRAGMA foreign_keys = ON;

CREATE TABLE schema_meta (
  schema_version TEXT PRIMARY KEY,
  applied_at_utc TEXT NOT NULL
) STRICT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m0-draft-2026-09-22', '2026-09-22T00:00:00Z');

CREATE TABLE world_instances (
  instance_id TEXT PRIMARY KEY,
  world_definition_id TEXT NOT NULL,
  world_definition_version TEXT NOT NULL,
  created_at_utc TEXT NOT NULL,
  lifecycle_state TEXT NOT NULL CHECK (lifecycle_state IN ('active', 'paused', 'archived'))
) STRICT;

CREATE TABLE branches (
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  label TEXT NOT NULL,
  head_sequence INTEGER NOT NULL DEFAULT 0 CHECK (head_sequence >= 0),
  forked_from_branch_id TEXT,
  forked_at_sequence INTEGER CHECK (forked_at_sequence IS NULL OR forked_at_sequence >= 0),
  created_at_utc TEXT NOT NULL,
  PRIMARY KEY (instance_id, branch_id),
  FOREIGN KEY (instance_id) REFERENCES world_instances(instance_id),
  FOREIGN KEY (instance_id, forked_from_branch_id)
    REFERENCES branches(instance_id, branch_id)
    DEFERRABLE INITIALLY DEFERRED,
  CHECK (
    (forked_from_branch_id IS NULL AND forked_at_sequence IS NULL)
    OR
    (forked_from_branch_id IS NOT NULL AND forked_at_sequence IS NOT NULL)
  )
) STRICT;

CREATE TABLE rule_epochs (
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  epoch_id TEXT NOT NULL,
  start_sequence INTEGER NOT NULL CHECK (start_sequence >= 1),
  end_sequence INTEGER CHECK (end_sequence IS NULL OR end_sequence > start_sequence),
  ruleset_hash TEXT NOT NULL
    CHECK (length(ruleset_hash) = 71 AND ruleset_hash GLOB 'sha256:[0-9a-f]*' AND lower(ruleset_hash) = ruleset_hash),
  lock_document TEXT NOT NULL CHECK (json_valid(lock_document)),
  activation_event_id TEXT,
  migration_contract_hash TEXT,
  PRIMARY KEY (instance_id, branch_id, epoch_id),
  UNIQUE (instance_id, branch_id, start_sequence),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id)
) STRICT;

CREATE UNIQUE INDEX ux_rule_epochs_one_open
ON rule_epochs(instance_id, branch_id)
WHERE end_sequence IS NULL;

CREATE TRIGGER rule_epochs_no_overlap_insert
BEFORE INSERT ON rule_epochs
WHEN EXISTS (
  SELECT 1
  FROM rule_epochs e
  WHERE e.instance_id = NEW.instance_id
    AND e.branch_id = NEW.branch_id
    AND e.start_sequence < COALESCE(NEW.end_sequence, 9223372036854775807)
    AND NEW.start_sequence < COALESCE(e.end_sequence, 9223372036854775807)
)
BEGIN
  SELECT RAISE(ABORT, 'RULE_EPOCH_OVERLAP');
END;

CREATE TABLE currencies (
  currency_id TEXT PRIMARY KEY,
  scale INTEGER NOT NULL CHECK (scale BETWEEN 0 AND 18),
  symbol TEXT NOT NULL,
  definition_event_id TEXT NOT NULL
) STRICT;

CREATE TABLE accounts (
  account_id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  account_type TEXT NOT NULL,
  overdraft_limit_minor INTEGER NOT NULL DEFAULT 0 CHECK (overdraft_limit_minor >= 0),
  overdraft_policy_id TEXT,
  opened_by_event_id TEXT NOT NULL,
  closed_by_event_id TEXT,
  UNIQUE (account_id, currency_id),
  FOREIGN KEY (currency_id) REFERENCES currencies(currency_id),
  CHECK (overdraft_limit_minor = 0 OR overdraft_policy_id IS NOT NULL)
) STRICT;

CREATE TABLE account_balances (
  account_id TEXT PRIMARY KEY,
  balance_minor INTEGER NOT NULL,
  projection_version INTEGER NOT NULL CHECK (projection_version >= 0),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 0),
  FOREIGN KEY (account_id) REFERENCES accounts(account_id)
) STRICT;

CREATE TABLE product_skus (
  sku_id TEXT PRIMARY KEY,
  base_unit TEXT NOT NULL,
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 18),
  definition_event_id TEXT NOT NULL
) STRICT;

CREATE TABLE stock_locations (
  location_id TEXT PRIMARY KEY,
  owner_id TEXT,
  location_kind TEXT NOT NULL CHECK (location_kind IN ('holder', 'source', 'sink')),
  capability_id TEXT,
  CHECK (location_kind = 'holder' OR capability_id IS NOT NULL)
) STRICT;

CREATE TABLE inventory_balances (
  location_id TEXT NOT NULL,
  sku_id TEXT NOT NULL,
  quantity_minor INTEGER NOT NULL,
  projection_version INTEGER NOT NULL CHECK (projection_version >= 0),
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 0),
  PRIMARY KEY (location_id, sku_id),
  FOREIGN KEY (location_id) REFERENCES stock_locations(location_id),
  FOREIGN KEY (sku_id) REFERENCES product_skus(sku_id)
) STRICT;

CREATE TABLE commands (
  command_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  command_type TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL
    CHECK (length(request_hash) = 71 AND request_hash GLOB 'sha256:[0-9a-f]*' AND lower(request_hash) = request_hash),
  expected_head INTEGER NOT NULL CHECK (expected_head >= 0),
  principal_id TEXT NOT NULL,
  command_policy TEXT NOT NULL CHECK (json_valid(command_policy)),
  status TEXT NOT NULL CHECK (status IN ('pending', 'committed', 'rejected')),
  created_at_utc TEXT NOT NULL,
  UNIQUE (instance_id, branch_id, command_type, idempotency_key),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id)
) STRICT;

CREATE TABLE command_attempts (
  command_id TEXT NOT NULL,
  attempt_no INTEGER NOT NULL CHECK (attempt_no >= 1),
  attempt_id TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK (status IN ('claimed', 'proposing', 'ready', 'committed', 'rejected', 'expired')),
  lease_owner TEXT,
  lease_until_utc TEXT,
  proposal_hash TEXT,
  created_at_utc TEXT NOT NULL,
  finished_at_utc TEXT,
  PRIMARY KEY (command_id, attempt_no),
  FOREIGN KEY (command_id) REFERENCES commands(command_id),
  CHECK (
    status IN ('committed', 'rejected', 'expired')
    OR (lease_owner IS NOT NULL AND lease_until_utc IS NOT NULL)
  )
) STRICT;

CREATE UNIQUE INDEX ux_command_attempts_one_active
ON command_attempts(command_id)
WHERE status IN ('claimed', 'proposing', 'ready');

CREATE TABLE event_batches (
  batch_id TEXT PRIMARY KEY,
  command_id TEXT NOT NULL,
  attempt_no INTEGER NOT NULL,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  epoch_id TEXT NOT NULL,
  expected_head INTEGER NOT NULL CHECK (expected_head >= 0),
  first_sequence INTEGER NOT NULL CHECK (first_sequence >= 1),
  last_sequence INTEGER NOT NULL CHECK (last_sequence >= first_sequence),
  event_count INTEGER NOT NULL CHECK (event_count > 0),
  world_time TEXT NOT NULL,
  batch_hash TEXT NOT NULL
    CHECK (length(batch_hash) = 71 AND batch_hash GLOB 'sha256:[0-9a-f]*' AND lower(batch_hash) = batch_hash),
  committed_at_utc TEXT NOT NULL,
  UNIQUE (command_id),
  UNIQUE (instance_id, branch_id, first_sequence),
  UNIQUE (instance_id, branch_id, last_sequence),
  FOREIGN KEY (command_id, attempt_no) REFERENCES command_attempts(command_id, attempt_no),
  FOREIGN KEY (instance_id, branch_id, epoch_id) REFERENCES rule_epochs(instance_id, branch_id, epoch_id),
  CHECK (first_sequence = expected_head + 1),
  CHECK (last_sequence = first_sequence + event_count - 1)
) STRICT;

CREATE TRIGGER event_batches_expected_head
BEFORE INSERT ON event_batches
WHEN NOT EXISTS (
  SELECT 1 FROM branches b
  WHERE b.instance_id = NEW.instance_id
    AND b.branch_id = NEW.branch_id
    AND b.head_sequence = NEW.expected_head
)
BEGIN
  SELECT RAISE(ABORT, 'BRANCH_VERSION_CONFLICT');
END;

CREATE TABLE events (
  event_id TEXT PRIMARY KEY,
  batch_id TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  event_sequence INTEGER NOT NULL CHECK (event_sequence >= 1),
  batch_index INTEGER NOT NULL CHECK (batch_index >= 0),
  event_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  world_time TEXT NOT NULL,
  causation_event_id TEXT,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  UNIQUE (instance_id, branch_id, event_sequence),
  UNIQUE (batch_id, batch_index),
  FOREIGN KEY (batch_id) REFERENCES event_batches(batch_id),
  FOREIGN KEY (causation_event_id) REFERENCES events(event_id)
    DEFERRABLE INITIALLY DEFERRED
) STRICT;

CREATE TRIGGER events_match_batch_range
BEFORE INSERT ON events
WHEN NOT EXISTS (
  SELECT 1
  FROM event_batches b
  JOIN rule_epochs e
    ON e.instance_id = b.instance_id
   AND e.branch_id = b.branch_id
   AND e.epoch_id = b.epoch_id
  WHERE b.batch_id = NEW.batch_id
    AND b.instance_id = NEW.instance_id
    AND b.branch_id = NEW.branch_id
    AND NEW.event_sequence BETWEEN b.first_sequence AND b.last_sequence
    AND NEW.event_sequence = b.first_sequence + NEW.batch_index
    AND NEW.event_sequence >= e.start_sequence
    AND (e.end_sequence IS NULL OR NEW.event_sequence < e.end_sequence)
)
BEGIN
  SELECT RAISE(ABORT, 'EVENT_SEQUENCE_OR_EPOCH_MISMATCH');
END;

CREATE TABLE audit_records (
  record_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  record_order INTEGER NOT NULL CHECK (record_order >= 1),
  record_type TEXT NOT NULL CHECK (record_type IN ('agent_decision', 'rule_validation', 'observation', 'runtime_diagnostic', 'intervention')),
  authority TEXT NOT NULL CHECK (authority IN ('audit', 'non-authoritative')),
  related_event_id TEXT,
  command_id TEXT,
  attempt_id TEXT,
  trace_id TEXT NOT NULL,
  world_time TEXT NOT NULL,
  recorded_at_utc TEXT NOT NULL,
  audience_scope TEXT NOT NULL CHECK (json_valid(audience_scope)),
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  UNIQUE (instance_id, branch_id, record_order),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id),
  FOREIGN KEY (related_event_id) REFERENCES events(event_id),
  FOREIGN KEY (command_id) REFERENCES commands(command_id)
) STRICT;

CREATE TABLE journal_entries (
  entry_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('draft', 'posted')),
  purpose TEXT NOT NULL,
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE postings (
  posting_id TEXT PRIMARY KEY,
  entry_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor <> 0),
  memo TEXT,
  FOREIGN KEY (entry_id) REFERENCES journal_entries(entry_id),
  FOREIGN KEY (account_id, currency_id) REFERENCES accounts(account_id, currency_id)
) STRICT;

CREATE TRIGGER postings_insert_only_draft
BEFORE INSERT ON postings
WHEN (SELECT status FROM journal_entries WHERE entry_id = NEW.entry_id) <> 'draft'
BEGIN
  SELECT RAISE(ABORT, 'JOURNAL_ALREADY_POSTED');
END;

CREATE TRIGGER postings_no_update_posted
BEFORE UPDATE ON postings
WHEN (SELECT status FROM journal_entries WHERE entry_id = OLD.entry_id) = 'posted'
BEGIN
  SELECT RAISE(ABORT, 'POSTED_JOURNAL_IMMUTABLE');
END;

CREATE TRIGGER postings_no_delete_posted
BEFORE DELETE ON postings
WHEN (SELECT status FROM journal_entries WHERE entry_id = OLD.entry_id) = 'posted'
BEGIN
  SELECT RAISE(ABORT, 'POSTED_JOURNAL_IMMUTABLE');
END;

CREATE TRIGGER journal_entries_validate_post
BEFORE UPDATE OF status ON journal_entries
WHEN OLD.status = 'draft' AND NEW.status = 'posted'
BEGIN
  SELECT CASE
    WHEN (SELECT COUNT(*) FROM postings WHERE entry_id = NEW.entry_id) < 2
    THEN RAISE(ABORT, 'JOURNAL_TOO_FEW_POSTINGS')
  END;
  SELECT CASE
    WHEN EXISTS (
      SELECT currency_id
      FROM postings
      WHERE entry_id = NEW.entry_id
      GROUP BY currency_id
      HAVING SUM(amount_minor) <> 0
    )
    THEN RAISE(ABORT, 'JOURNAL_UNBALANCED')
  END;
END;

CREATE TRIGGER journal_entries_no_change_posted
BEFORE UPDATE ON journal_entries
WHEN OLD.status = 'posted'
BEGIN
  SELECT RAISE(ABORT, 'POSTED_JOURNAL_IMMUTABLE');
END;

CREATE TRIGGER journal_entries_no_delete_posted
BEFORE DELETE ON journal_entries
WHEN OLD.status = 'posted'
BEGIN
  SELECT RAISE(ABORT, 'POSTED_JOURNAL_IMMUTABLE');
END;

CREATE TABLE stock_movements (
  movement_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL,
  sku_id TEXT NOT NULL,
  from_location_id TEXT NOT NULL,
  to_location_id TEXT NOT NULL,
  quantity_minor INTEGER NOT NULL CHECK (quantity_minor > 0),
  movement_kind TEXT NOT NULL CHECK (movement_kind IN ('transfer', 'create', 'consume', 'destroy')),
  reason_code TEXT NOT NULL,
  capability_id TEXT,
  FOREIGN KEY (event_id) REFERENCES events(event_id),
  FOREIGN KEY (sku_id) REFERENCES product_skus(sku_id),
  FOREIGN KEY (from_location_id) REFERENCES stock_locations(location_id),
  FOREIGN KEY (to_location_id) REFERENCES stock_locations(location_id),
  CHECK (from_location_id <> to_location_id),
  CHECK (movement_kind = 'transfer' OR capability_id IS NOT NULL)
) STRICT;

CREATE TABLE scheduler_items (
  scheduler_item_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  world_time TEXT NOT NULL,
  phase_id TEXT NOT NULL,
  declared_priority INTEGER NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'deferred', 'cancelled')),
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id)
) STRICT;

CREATE INDEX ix_scheduler_stable_order
ON scheduler_items(instance_id, branch_id, world_time, phase_id, declared_priority, scheduler_item_id);

CREATE TABLE outbox (
  outbox_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL,
  topic TEXT NOT NULL,
  audience_scope TEXT NOT NULL CHECK (json_valid(audience_scope)),
  audience_scope_hash TEXT NOT NULL,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at_utc TEXT,
  published_at_utc TEXT,
  UNIQUE (event_id, topic, audience_scope_hash),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE snapshots (
  snapshot_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  through_sequence INTEGER NOT NULL CHECK (through_sequence >= 0),
  state_hash TEXT NOT NULL
    CHECK (length(state_hash) = 71 AND state_hash GLOB 'sha256:[0-9a-f]*' AND lower(state_hash) = state_hash),
  storage_ref TEXT NOT NULL,
  created_at_utc TEXT NOT NULL,
  UNIQUE (instance_id, branch_id, through_sequence),
  FOREIGN KEY (instance_id, branch_id) REFERENCES branches(instance_id, branch_id)
) STRICT;

-- The application commit gate must additionally prove, in the same write transaction:
-- 1. every event_batches row has exactly event_count events with contiguous batch_index values;
-- 2. all draft journals created by the batch are posted before COMMIT;
-- 3. signed integer arithmetic did not overflow before SQLite receives values;
-- 4. ordinary account/inventory projections respect overdraft and nonnegative-stock policies;
-- 5. source/sink movements and privileged commands have Principal/Capability/Scope authorization;
-- 6. Rule Epoch intervals are continuous (the trigger only rejects overlap);
-- 7. branch.head_sequence is conditionally updated from expected_head to last_sequence exactly once;
-- 8. canonical hashes are recomputed from normative bytes, not trusted from the caller.
