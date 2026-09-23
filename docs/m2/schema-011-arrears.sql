-- M2 contract-local grace, linked arrears retries and one funded service order.
CREATE TABLE m2_arrears_terms (
  contract_id TEXT PRIMARY KEY,
  grace_days INTEGER NOT NULL CHECK (grace_days BETWEEN 0 AND 30),
  late_fee_minor INTEGER NOT NULL CHECK (late_fee_minor = 0),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (contract_id) REFERENCES m2_cohort_contracts(contract_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_arrears_cases (
  obligation_id TEXT PRIMARY KEY,
  contract_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('wage', 'rent')),
  grace_expires_at TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('open', 'grace_expired', 'cured')),
  shortage_event_id TEXT NOT NULL UNIQUE,
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence > 0),
  FOREIGN KEY (obligation_id) REFERENCES m2_economic_obligations(obligation_id),
  FOREIGN KEY (contract_id) REFERENCES m2_arrears_terms(contract_id),
  FOREIGN KEY (shortage_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_arrears_attempts (
  scheduler_item_id TEXT PRIMARY KEY,
  contract_id TEXT NOT NULL,
  obligation_id TEXT,
  day INTEGER NOT NULL CHECK (day BETWEEN 1 AND 30),
  status TEXT NOT NULL CHECK (status IN ('no_arrears', 'deferred', 'partial', 'paid')),
  reason_code TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
  event_id TEXT NOT NULL UNIQUE,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  CHECK ((status = 'no_arrears' AND reason_code = 'none_due' AND obligation_id IS NULL AND amount_minor = 0)
     OR (status = 'deferred' AND reason_code = 'insufficient_liquidity' AND obligation_id IS NOT NULL AND amount_minor = 0)
     OR (status IN ('partial', 'paid') AND reason_code = 'none' AND obligation_id IS NOT NULL AND amount_minor > 0)),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (contract_id) REFERENCES m2_arrears_terms(contract_id),
  FOREIGN KEY (obligation_id) REFERENCES m2_economic_obligations(obligation_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_default_reviews (
  scheduler_item_id TEXT PRIMARY KEY,
  day INTEGER NOT NULL UNIQUE CHECK (day BETWEEN 1 AND 30),
  expired_count INTEGER NOT NULL CHECK (expired_count >= 0),
  event_id TEXT NOT NULL UNIQUE,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_default_transitions (
  obligation_id TEXT PRIMARY KEY,
  decision TEXT NOT NULL CHECK (decision IN ('refinement_blocked_contract_split', 'rent_grace_expired_review')),
  reason_code TEXT NOT NULL,
  event_id TEXT NOT NULL,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  FOREIGN KEY (obligation_id) REFERENCES m2_arrears_cases(obligation_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_service_orders (
  order_id TEXT PRIMARY KEY,
  buyer_actor_id TEXT NOT NULL,
  seller_actor_id TEXT NOT NULL,
  service_code TEXT NOT NULL,
  price_minor INTEGER NOT NULL CHECK (price_minor > 0),
  currency_id TEXT NOT NULL,
  due_at TEXT NOT NULL,
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (buyer_actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (seller_actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (currency_id) REFERENCES currencies(currency_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_service_settlements (
  order_id TEXT PRIMARY KEY,
  scheduler_item_id TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK (status IN ('paid', 'rejected')),
  reason_code TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
  event_id TEXT NOT NULL UNIQUE,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  CHECK ((status = 'paid' AND reason_code = 'none' AND amount_minor > 0)
     OR (status = 'rejected' AND reason_code = 'insufficient_landlord_funds' AND amount_minor = 0)),
  FOREIGN KEY (order_id) REFERENCES m2_service_orders(order_id),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-arrears-grace-011-2026-09-23', '2026-09-23T00:00:00Z');
