-- Fixture-local monetary-unit allocation for split wage obligations.
-- Each round visits one slot per original worker: remaining Cohort slots
-- first, then named entities by stable ID. This is not creditor priority law.
CREATE TABLE m2_wage_allocation_policies (
  policy_id TEXT PRIMARY KEY,
  contract_id TEXT NOT NULL REFERENCES m2_cohort_contracts(contract_id),
  effective_from TEXT NOT NULL,
  policy_version TEXT NOT NULL CHECK (policy_version = 'worker_round_robin_v1'),
  definition_event_id TEXT NOT NULL REFERENCES events(event_id),
  UNIQUE (contract_id, effective_from)
) STRICT;

CREATE TRIGGER m2_wage_allocation_policies_no_update BEFORE UPDATE ON m2_wage_allocation_policies BEGIN SELECT RAISE(ABORT, 'M2_WAGE_POLICY_IMMUTABLE'); END;
CREATE TRIGGER m2_wage_allocation_policies_no_delete BEFORE DELETE ON m2_wage_allocation_policies BEGIN SELECT RAISE(ABORT, 'M2_WAGE_POLICY_IMMUTABLE'); END;

-- Existing schema-016 databases can have the contract already defined. The
-- policy affects future split settlement only; completed events are unchanged.
INSERT INTO m2_wage_allocation_policies(policy_id, contract_id, effective_from, policy_version, definition_event_id)
SELECT 'policy_m2_wage_round_robin_v1', contract_id, effective_from, 'worker_round_robin_v1', definition_event_id
FROM m2_cohort_contracts WHERE contract_id = 'contract_m2_cohort_wage_18' AND kind = 'wage';

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-wage-allocation-017-2026-09-23', '2026-09-23T00:00:00Z');
