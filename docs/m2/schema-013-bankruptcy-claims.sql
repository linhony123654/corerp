-- Immutable, unranked creditor claims at the employer proceeding opening.
-- Claims preserve the original Cohort obligation; allocation to individuals
-- requires a later explicit materialization event.
CREATE TABLE m2_bankruptcy_claims (
  proceeding_id TEXT NOT NULL,
  obligation_id TEXT NOT NULL UNIQUE,
  claimant_cohort_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  due_minor INTEGER NOT NULL CHECK (due_minor > 0),
  paid_at_open_minor INTEGER NOT NULL CHECK (paid_at_open_minor >= 0),
  outstanding_at_open_minor INTEGER NOT NULL CHECK (outstanding_at_open_minor > 0),
  opening_event_id TEXT NOT NULL,
  opening_event_sequence INTEGER NOT NULL CHECK (opening_event_sequence > 0),
  PRIMARY KEY (proceeding_id, obligation_id),
  CHECK (paid_at_open_minor < due_minor),
  FOREIGN KEY (proceeding_id) REFERENCES m2_bankruptcy_proceedings(proceeding_id),
  FOREIGN KEY (obligation_id) REFERENCES m2_economic_obligations(obligation_id),
  FOREIGN KEY (claimant_cohort_id) REFERENCES cohorts(cohort_id),
  FOREIGN KEY (currency_id) REFERENCES currencies(currency_id),
  FOREIGN KEY (opening_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TRIGGER m2_bankruptcy_claims_immutable_update
BEFORE UPDATE ON m2_bankruptcy_claims
BEGIN
  SELECT RAISE(ABORT, 'M2_BANKRUPTCY_CLAIM_IMMUTABLE');
END;

CREATE TRIGGER m2_bankruptcy_claims_immutable_delete
BEFORE DELETE ON m2_bankruptcy_claims
BEGIN
  SELECT RAISE(ABORT, 'M2_BANKRUPTCY_CLAIM_IMMUTABLE');
END;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-bankruptcy-claims-013-2026-09-23', '2026-09-23T00:00:00Z');
