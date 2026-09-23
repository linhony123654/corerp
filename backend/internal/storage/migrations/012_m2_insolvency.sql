-- M2 fictional employer-insolvency policy and immutable proceeding opening.
CREATE TABLE m2_insolvency_policies (
  policy_id TEXT PRIMARY KEY,
  actor_id TEXT NOT NULL UNIQUE,
  decision_at TEXT NOT NULL,
  min_unpaid_wage_minor INTEGER NOT NULL CHECK (min_unpaid_wage_minor > 0),
  require_zero_cash INTEGER NOT NULL CHECK (require_zero_cash = 1),
  definition_event_id TEXT NOT NULL,
  FOREIGN KEY (actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_insolvency_reviews (
  scheduler_item_id TEXT PRIMARY KEY,
  policy_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('proceeding_opened', 'no_action')),
  reason_code TEXT NOT NULL,
  cash_minor INTEGER NOT NULL CHECK (cash_minor >= 0),
  unpaid_wage_minor INTEGER NOT NULL CHECK (unpaid_wage_minor >= 0),
  expired_case_count INTEGER NOT NULL CHECK (expired_case_count >= 0),
  event_id TEXT NOT NULL UNIQUE,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  CHECK ((status = 'proceeding_opened' AND reason_code = 'declared_employer_insolvency' AND cash_minor = 0 AND unpaid_wage_minor > 0 AND expired_case_count > 0)
     OR (status = 'no_action' AND reason_code IN ('available_cash', 'below_debt_threshold', 'no_expired_case'))),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (policy_id) REFERENCES m2_insolvency_policies(policy_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_bankruptcy_proceedings (
  proceeding_id TEXT PRIMARY KEY,
  actor_id TEXT NOT NULL UNIQUE,
  opening_review_item_id TEXT NOT NULL UNIQUE,
  opened_world_time TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status = 'open'),
  cash_at_open_minor INTEGER NOT NULL CHECK (cash_at_open_minor = 0),
  unpaid_wage_at_open_minor INTEGER NOT NULL CHECK (unpaid_wage_at_open_minor > 0),
  opening_event_id TEXT NOT NULL UNIQUE,
  opening_event_sequence INTEGER NOT NULL CHECK (opening_event_sequence > 0),
  FOREIGN KEY (actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (opening_review_item_id) REFERENCES m2_insolvency_reviews(scheduler_item_id),
  FOREIGN KEY (opening_event_id) REFERENCES events(event_id)
) STRICT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-insolvency-012-2026-09-23', '2026-09-23T00:00:00Z');
