-- Fixture-local voluntary no-recourse estate funding and claimant-aware wage distribution.
-- This policy ranks no creditors outside this declared demonstration.
CREATE TABLE m2_estate_policies (
  policy_id TEXT PRIMARY KEY,
  actor_id TEXT NOT NULL,
  donor_actor_id TEXT NOT NULL,
  contribution_minor INTEGER NOT NULL CHECK (contribution_minor > 0),
  contribution_at TEXT NOT NULL,
  distribution_at TEXT NOT NULL,
  per_worker_minor INTEGER NOT NULL CHECK (per_worker_minor > 0),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (donor_actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_estate_contributions (
  scheduler_item_id TEXT PRIMARY KEY,
  policy_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('paid', 'deferred')),
  reason_code TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
  event_id TEXT NOT NULL,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (policy_id) REFERENCES m2_estate_policies(policy_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_estate_distributions (
  scheduler_item_id TEXT PRIMARY KEY,
  policy_id TEXT NOT NULL,
  obligation_id TEXT,
  status TEXT NOT NULL CHECK (status IN ('paid', 'deferred')),
  reason_code TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
  event_id TEXT NOT NULL,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (policy_id) REFERENCES m2_estate_policies(policy_id),
  FOREIGN KEY (obligation_id) REFERENCES m2_bankruptcy_claims(obligation_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_estate_distribution_receipts (
  scheduler_item_id TEXT NOT NULL,
  claimant_kind TEXT NOT NULL CHECK (claimant_kind IN ('cohort', 'entity')),
  claimant_id TEXT NOT NULL,
  asset_account_id TEXT NOT NULL,
  receivable_account_id TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  event_id TEXT NOT NULL,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  PRIMARY KEY (scheduler_item_id, claimant_kind, claimant_id),
  FOREIGN KEY (scheduler_item_id) REFERENCES m2_estate_distributions(scheduler_item_id),
  FOREIGN KEY (asset_account_id) REFERENCES accounts(account_id),
  FOREIGN KEY (receivable_account_id) REFERENCES accounts(account_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TRIGGER m2_estate_policies_immutable_update BEFORE UPDATE ON m2_estate_policies BEGIN SELECT RAISE(ABORT, 'M2_ESTATE_POLICY_IMMUTABLE'); END;
CREATE TRIGGER m2_estate_policies_immutable_delete BEFORE DELETE ON m2_estate_policies BEGIN SELECT RAISE(ABORT, 'M2_ESTATE_POLICY_IMMUTABLE'); END;
CREATE TRIGGER m2_estate_contributions_immutable_update BEFORE UPDATE ON m2_estate_contributions BEGIN SELECT RAISE(ABORT, 'M2_ESTATE_CONTRIBUTION_IMMUTABLE'); END;
CREATE TRIGGER m2_estate_contributions_immutable_delete BEFORE DELETE ON m2_estate_contributions BEGIN SELECT RAISE(ABORT, 'M2_ESTATE_CONTRIBUTION_IMMUTABLE'); END;
CREATE TRIGGER m2_estate_distributions_immutable_update BEFORE UPDATE ON m2_estate_distributions BEGIN SELECT RAISE(ABORT, 'M2_ESTATE_DISTRIBUTION_IMMUTABLE'); END;
CREATE TRIGGER m2_estate_distributions_immutable_delete BEFORE DELETE ON m2_estate_distributions BEGIN SELECT RAISE(ABORT, 'M2_ESTATE_DISTRIBUTION_IMMUTABLE'); END;
CREATE TRIGGER m2_estate_distribution_receipts_immutable_update BEFORE UPDATE ON m2_estate_distribution_receipts BEGIN SELECT RAISE(ABORT, 'M2_ESTATE_RECEIPT_IMMUTABLE'); END;
CREATE TRIGGER m2_estate_distribution_receipts_immutable_delete BEFORE DELETE ON m2_estate_distribution_receipts BEGIN SELECT RAISE(ABORT, 'M2_ESTATE_RECEIPT_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-estate-distribution-015-2026-09-23', '2026-09-23T00:00:00Z');
