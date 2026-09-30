-- Long RP runs repeatedly ask for the latest sourced financial event for
-- three owned accounts. Keep that lookup bounded by account instead of
-- rescanning the world's accumulated postings for every NPC decision.
CREATE INDEX ix_postings_account_entry ON postings(account_id,entry_id);

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-life-postings-index-074-2026-09-28','2026-09-28T00:00:00Z');
