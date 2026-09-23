-- Immutable current-owner changes for individual wage-claim slots. The
-- original aggregate obligation and accrual slices are never rewritten.
CREATE TABLE m2_wage_claim_owner_transitions (
  transition_id TEXT PRIMARY KEY,
  materialization_id TEXT NOT NULL REFERENCES cohort_materializations(materialization_id),
  obligation_id TEXT NOT NULL REFERENCES m2_economic_obligations(obligation_id),
  slot_index INTEGER NOT NULL CHECK (slot_index >= 0),
  transition_kind TEXT NOT NULL CHECK (transition_kind IN ('materialize', 'dematerialize')),
  from_kind TEXT NOT NULL CHECK (from_kind IN ('cohort', 'entity')),
  from_id TEXT NOT NULL,
  to_kind TEXT NOT NULL CHECK (to_kind IN ('cohort', 'entity')),
  to_id TEXT NOT NULL,
  outstanding_minor INTEGER NOT NULL CHECK (outstanding_minor > 0),
  event_id TEXT NOT NULL REFERENCES events(event_id),
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  UNIQUE (obligation_id, slot_index, event_sequence),
  UNIQUE (materialization_id, obligation_id, slot_index, transition_kind),
  CHECK (from_kind != to_kind AND from_id != to_id)
) STRICT;

CREATE TABLE m2_wage_participation_returns (
  materialization_id TEXT PRIMARY KEY REFERENCES m2_wage_participation_splits(materialization_id),
  effective_from TEXT NOT NULL,
  return_event_id TEXT NOT NULL REFERENCES events(event_id),
  return_event_sequence INTEGER NOT NULL CHECK (return_event_sequence > 0)
) STRICT;

-- New payments after an ownership transition identify the actual recipient
-- per worker slot; old schema-016 aggregate receipts remain historical facts.
CREATE TABLE m2_wage_slot_receipts (
  scheduler_item_id TEXT NOT NULL REFERENCES scheduler_items(scheduler_item_id),
  obligation_id TEXT NOT NULL REFERENCES m2_economic_obligations(obligation_id),
  slot_index INTEGER NOT NULL CHECK (slot_index >= 0),
  claimant_kind TEXT NOT NULL CHECK (claimant_kind IN ('cohort', 'entity')),
  claimant_id TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  event_id TEXT NOT NULL REFERENCES events(event_id),
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  PRIMARY KEY (scheduler_item_id, obligation_id, slot_index)
) STRICT;

CREATE TRIGGER m2_wage_claim_owner_transitions_no_update BEFORE UPDATE ON m2_wage_claim_owner_transitions BEGIN SELECT RAISE(ABORT, 'M2_WAGE_OWNER_TRANSITION_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_claim_owner_transitions_no_delete BEFORE DELETE ON m2_wage_claim_owner_transitions BEGIN SELECT RAISE(ABORT, 'M2_WAGE_OWNER_TRANSITION_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_participation_returns_no_update BEFORE UPDATE ON m2_wage_participation_returns BEGIN SELECT RAISE(ABORT, 'M2_WAGE_PARTICIPATION_RETURN_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_participation_returns_no_delete BEFORE DELETE ON m2_wage_participation_returns BEGIN SELECT RAISE(ABORT, 'M2_WAGE_PARTICIPATION_RETURN_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_slot_receipts_no_update BEFORE UPDATE ON m2_wage_slot_receipts BEGIN SELECT RAISE(ABORT, 'M2_WAGE_SLOT_RECEIPT_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_slot_receipts_no_delete BEFORE DELETE ON m2_wage_slot_receipts BEGIN SELECT RAISE(ABORT, 'M2_WAGE_SLOT_RECEIPT_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-wage-claim-ownership-018-2026-09-23', '2026-09-23T00:00:00Z');
