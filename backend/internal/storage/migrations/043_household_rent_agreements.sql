-- A household's rent contract is sourced in its own RP branch. The existing
-- rent obligation tables remain the accounting authority for future accrual.
CREATE TABLE rp_household_rent_agreements (
  agreement_id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES rp_households(household_id),
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  contract_id TEXT NOT NULL UNIQUE REFERENCES rent_contracts(contract_id),
  landlord_entity_id TEXT NOT NULL UNIQUE REFERENCES economic_entities(entity_id),
  landlord_account_id TEXT NOT NULL UNIQUE REFERENCES accounts(account_id),
  starts_world_time TEXT NOT NULL,
  starts_world_day INTEGER NOT NULL CHECK (starts_world_day >= 0),
  status TEXT NOT NULL CHECK (status IN ('active','ended')),
  source_event_id TEXT NOT NULL UNIQUE REFERENCES events(event_id),
  ended_event_id TEXT REFERENCES events(event_id),
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((status='active' AND ended_event_id IS NULL) OR (status='ended' AND ended_event_id IS NOT NULL))
) STRICT;

CREATE UNIQUE INDEX ux_rp_household_one_active_rent
ON rp_household_rent_agreements(household_id) WHERE status='active';

CREATE TABLE rp_household_rent_shares (
  agreement_id TEXT NOT NULL REFERENCES rp_household_rent_agreements(agreement_id),
  membership_id TEXT NOT NULL REFERENCES rp_household_memberships(membership_id),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  source_event_id TEXT NOT NULL REFERENCES events(event_id),
  PRIMARY KEY(agreement_id,membership_id)
) STRICT;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f4-household-rent-043-2026-09-25','2026-09-25T00:00:00Z');
