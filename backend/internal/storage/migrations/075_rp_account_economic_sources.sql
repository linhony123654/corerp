-- Read-only acceleration for the latest posted economic source per account.
-- The immutable journal/postings/events remain authoritative; this table is
-- rebuilt from them when projection repair detects a difference.
CREATE TABLE rp_account_economic_sources (
  account_id TEXT NOT NULL,
  entry_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  event_sequence INTEGER NOT NULL CHECK (event_sequence >= 1),
  PRIMARY KEY (account_id, entry_id),
  FOREIGN KEY (account_id) REFERENCES accounts(account_id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(entry_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id),
  FOREIGN KEY (instance_id, branch_id, event_sequence)
    REFERENCES events(instance_id, branch_id, event_sequence)
) STRICT;

CREATE INDEX ix_rp_account_economic_latest
ON rp_account_economic_sources(account_id, instance_id, branch_id, event_sequence DESC, event_id);

INSERT INTO rp_account_economic_sources(account_id, entry_id, event_id, instance_id, branch_id, event_sequence)
SELECT DISTINCT p.account_id, j.entry_id, j.event_id, e.instance_id, e.branch_id, e.event_sequence
FROM postings p
JOIN journal_entries j ON j.entry_id = p.entry_id AND j.status = 'posted'
JOIN events e ON e.event_id = j.event_id;

CREATE TRIGGER rp_account_economic_sources_on_post
AFTER UPDATE OF status ON journal_entries
WHEN OLD.status = 'draft' AND NEW.status = 'posted'
BEGIN
  INSERT INTO rp_account_economic_sources(account_id, entry_id, event_id, instance_id, branch_id, event_sequence)
  SELECT DISTINCT p.account_id, NEW.entry_id, NEW.event_id, e.instance_id, e.branch_id, e.event_sequence
  FROM postings p JOIN events e ON e.event_id = NEW.event_id
  WHERE p.entry_id = NEW.entry_id;
END;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp-account-economic-sources-075-2026-09-28', '2026-09-28T00:00:00Z');
