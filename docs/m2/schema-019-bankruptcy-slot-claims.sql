-- Immutable per-worker creditor snapshot at the fictional employer proceeding.
-- This records existing unpaid wage debt; it does not authorize liquidation.
CREATE TABLE m2_bankruptcy_slot_claims (
  proceeding_id TEXT NOT NULL REFERENCES m2_bankruptcy_proceedings(proceeding_id),
  obligation_id TEXT NOT NULL REFERENCES m2_economic_obligations(obligation_id),
  slot_index INTEGER NOT NULL CHECK (slot_index >= 0),
  origin_kind TEXT NOT NULL CHECK (origin_kind IN ('cohort', 'entity')),
  origin_id TEXT NOT NULL,
  claimant_kind TEXT NOT NULL CHECK (claimant_kind IN ('cohort', 'entity')),
  claimant_id TEXT NOT NULL,
  due_minor INTEGER NOT NULL CHECK (due_minor > 0),
  paid_at_open_minor INTEGER NOT NULL CHECK (paid_at_open_minor >= 0),
  outstanding_at_open_minor INTEGER NOT NULL CHECK (outstanding_at_open_minor > 0),
  opening_event_id TEXT NOT NULL REFERENCES events(event_id),
  opening_event_sequence INTEGER NOT NULL CHECK (opening_event_sequence > 0),
  PRIMARY KEY (proceeding_id, obligation_id, slot_index),
  CHECK (due_minor = paid_at_open_minor + outstanding_at_open_minor)
) STRICT;

CREATE TRIGGER m2_bankruptcy_slot_claims_no_update BEFORE UPDATE ON m2_bankruptcy_slot_claims BEGIN SELECT RAISE(ABORT, 'M2_BANKRUPTCY_SLOT_CLAIM_IMMUTABLE'); END;
CREATE TRIGGER m2_bankruptcy_slot_claims_no_delete BEFORE DELETE ON m2_bankruptcy_slot_claims BEGIN SELECT RAISE(ABORT, 'M2_BANKRUPTCY_SLOT_CLAIM_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-bankruptcy-slot-claims-019-2026-09-23', '2026-09-23T00:00:00Z');
