-- An existing Person may depend on one active adult in the same household.
-- The reference is a relationship, not a copied Person or wallet authority.
ALTER TABLE rp_household_memberships
ADD COLUMN supporter_membership_id TEXT REFERENCES rp_household_memberships(membership_id);

CREATE INDEX ix_rp_household_memberships_supporter
ON rp_household_memberships(supporter_membership_id,ended_event_id);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f4-household-dependents-045-2026-09-25','2026-09-25T00:00:00Z');
