-- CoreRP M1 obligation accounting
-- Adds explicit two-sided accrual accounts and immutable payment-attempt evidence.

CREATE TABLE obligation_ledger_accounts (
  obligation_kind TEXT NOT NULL CHECK (obligation_kind IN ('wage', 'rent')),
  contract_id TEXT NOT NULL,
  expense_account_id TEXT NOT NULL,
  payable_account_id TEXT NOT NULL,
  receivable_account_id TEXT NOT NULL,
  income_account_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  definition_event_id TEXT NOT NULL,
  PRIMARY KEY (obligation_kind, contract_id),
  FOREIGN KEY (expense_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (payable_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (receivable_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  FOREIGN KEY (income_account_id, currency_id) REFERENCES accounts(account_id, currency_id),
  CHECK (expense_account_id <> payable_account_id),
  CHECK (expense_account_id <> receivable_account_id),
  CHECK (expense_account_id <> income_account_id),
  CHECK (payable_account_id <> receivable_account_id),
  CHECK (payable_account_id <> income_account_id),
  CHECK (receivable_account_id <> income_account_id)
) STRICT;

CREATE TABLE obligation_settlements (
  settlement_id TEXT PRIMARY KEY,
  obligation_kind TEXT NOT NULL CHECK (obligation_kind IN ('wage', 'rent')),
  obligation_id TEXT NOT NULL,
  attempted_minor INTEGER NOT NULL CHECK (attempted_minor > 0),
  paid_minor INTEGER NOT NULL CHECK (paid_minor >= 0 AND paid_minor <= attempted_minor),
  remaining_minor INTEGER NOT NULL CHECK (remaining_minor >= 0),
  status TEXT NOT NULL CHECK (status IN ('paid', 'partial', 'failed')),
  reason_code TEXT NOT NULL,
  event_id TEXT NOT NULL UNIQUE,
  event_sequence INTEGER NOT NULL CHECK (event_sequence >= 1),
  FOREIGN KEY (event_id) REFERENCES events(event_id),
  UNIQUE (obligation_kind, obligation_id, settlement_id)
) STRICT;

CREATE INDEX ix_obligation_settlements_lookup
ON obligation_settlements(obligation_kind, obligation_id, event_sequence);

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m1-obligation-accounting-004-2026-09-22', '2026-09-22T00:00:00Z');
