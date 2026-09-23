-- Claim ownership follows T09 receivable allocation; the original wage
-- obligation and its bankruptcy-opening snapshot remain unchanged.
CREATE TABLE m2_bankruptcy_claim_allocations (
  materialization_id TEXT NOT NULL,
  obligation_id TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  allocation_event_id TEXT NOT NULL,
  allocation_event_sequence INTEGER NOT NULL CHECK (allocation_event_sequence > 0),
  PRIMARY KEY (materialization_id, obligation_id),
  UNIQUE (materialization_id, obligation_id, amount_minor),
  FOREIGN KEY (materialization_id) REFERENCES cohort_materializations(materialization_id),
  FOREIGN KEY (obligation_id) REFERENCES m2_bankruptcy_claims(obligation_id),
  FOREIGN KEY (entity_id) REFERENCES materialized_entities(entity_id),
  FOREIGN KEY (allocation_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_bankruptcy_claim_returns (
  materialization_id TEXT NOT NULL,
  obligation_id TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  return_event_id TEXT NOT NULL,
  return_event_sequence INTEGER NOT NULL CHECK (return_event_sequence > 0),
  PRIMARY KEY (materialization_id, obligation_id),
  FOREIGN KEY (materialization_id, obligation_id, amount_minor) REFERENCES m2_bankruptcy_claim_allocations(materialization_id, obligation_id, amount_minor),
  FOREIGN KEY (return_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TRIGGER m2_bankruptcy_claim_allocations_immutable_update BEFORE UPDATE ON m2_bankruptcy_claim_allocations BEGIN SELECT RAISE(ABORT, 'M2_CLAIM_ALLOCATION_IMMUTABLE'); END;
CREATE TRIGGER m2_bankruptcy_claim_allocations_immutable_delete BEFORE DELETE ON m2_bankruptcy_claim_allocations BEGIN SELECT RAISE(ABORT, 'M2_CLAIM_ALLOCATION_IMMUTABLE'); END;
CREATE TRIGGER m2_bankruptcy_claim_returns_immutable_update BEFORE UPDATE ON m2_bankruptcy_claim_returns BEGIN SELECT RAISE(ABORT, 'M2_CLAIM_RETURN_IMMUTABLE'); END;
CREATE TRIGGER m2_bankruptcy_claim_returns_immutable_delete BEFORE DELETE ON m2_bankruptcy_claim_returns BEGIN SELECT RAISE(ABORT, 'M2_CLAIM_RETURN_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-claim-allocations-014-2026-09-23', '2026-09-23T00:00:00Z');
