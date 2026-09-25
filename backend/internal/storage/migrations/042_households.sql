-- F4 household identity and membership are sourced separately from people.
-- The rent account is owned by the household economic Entity; it conveys no
-- authority to spend any member's personal account.
CREATE TABLE rp_households (
  household_id TEXT PRIMARY KEY REFERENCES economic_entities(entity_id),
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  residence_place_id TEXT NOT NULL REFERENCES agent_places(place_id),
  rent_account_id TEXT NOT NULL UNIQUE REFERENCES accounts(account_id),
  currency_id TEXT NOT NULL REFERENCES currencies(currency_id),
  status TEXT NOT NULL CHECK (status IN ('active','ended')),
  source_event_id TEXT NOT NULL UNIQUE REFERENCES events(event_id),
  started_world_time TEXT NOT NULL,
  ended_event_id TEXT REFERENCES events(event_id),
  ended_world_time TEXT,
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((status='active' AND ended_event_id IS NULL AND ended_world_time IS NULL)
    OR (status='ended' AND ended_event_id IS NOT NULL AND ended_world_time IS NOT NULL))
) STRICT;

CREATE TABLE rp_household_memberships (
  membership_id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES rp_households(household_id),
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  member_entity_id TEXT NOT NULL REFERENCES materialized_entities(entity_id),
  member_role TEXT NOT NULL CHECK (member_role IN ('adult','dependent')),
  source_event_id TEXT NOT NULL REFERENCES events(event_id),
  started_world_time TEXT NOT NULL,
  ended_event_id TEXT REFERENCES events(event_id),
  ended_world_time TEXT,
  FOREIGN KEY(instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((ended_event_id IS NULL AND ended_world_time IS NULL)
    OR (ended_event_id IS NOT NULL AND ended_world_time IS NOT NULL))
) STRICT;

CREATE UNIQUE INDEX ux_rp_household_one_active_membership
ON rp_household_memberships(instance_id,branch_id,member_entity_id)
WHERE ended_event_id IS NULL;

CREATE INDEX ix_rp_household_memberships_household
ON rp_household_memberships(household_id,ended_event_id);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f4-households-042-2026-09-25','2026-09-25T00:00:00Z');
