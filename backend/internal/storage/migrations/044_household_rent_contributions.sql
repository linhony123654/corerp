-- Each resident's rent funding is an explicit, sourced personal decision.
-- Posted journal entries remain the cash/balance authority.
CREATE TABLE rp_household_rent_contributions (
  contribution_id TEXT PRIMARY KEY,
  agreement_id TEXT NOT NULL REFERENCES rp_household_rent_agreements(agreement_id),
  membership_id TEXT NOT NULL REFERENCES rp_household_memberships(membership_id),
  period_index INTEGER NOT NULL CHECK (period_index >= 0),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  source_account_id TEXT NOT NULL REFERENCES accounts(account_id),
  target_account_id TEXT NOT NULL REFERENCES accounts(account_id),
  event_id TEXT NOT NULL UNIQUE REFERENCES events(event_id),
  journal_entry_id TEXT NOT NULL UNIQUE REFERENCES journal_entries(entry_id)
) STRICT;

CREATE INDEX ix_rp_household_rent_contributions_share
ON rp_household_rent_contributions(agreement_id,membership_id,period_index);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f4-household-rent-contributions-044-2026-09-25','2026-09-25T00:00:00Z');
