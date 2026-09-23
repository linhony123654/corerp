-- M2 finite-store offer and durable daily purchase decisions.
CREATE TABLE m2_store_offers (
  offer_id TEXT PRIMARY KEY,
  actor_id TEXT NOT NULL,
  cohort_id TEXT NOT NULL,
  sku_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  unit_price_minor INTEGER NOT NULL CHECK (unit_price_minor > 0),
  daily_budget_minor INTEGER NOT NULL CHECK (daily_budget_minor >= 0),
  household_location_id TEXT NOT NULL,
  effective_from TEXT NOT NULL,
  effective_until TEXT,
  definition_event_id TEXT NOT NULL,
  CHECK (effective_until IS NULL OR effective_until > effective_from),
  FOREIGN KEY (actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (cohort_id) REFERENCES cohorts(cohort_id),
  FOREIGN KEY (sku_id) REFERENCES product_skus(sku_id),
  FOREIGN KEY (currency_id) REFERENCES currencies(currency_id),
  FOREIGN KEY (household_location_id) REFERENCES stock_locations(location_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_purchase_outcomes (
  scheduler_item_id TEXT PRIMARY KEY,
  offer_id TEXT NOT NULL,
  day INTEGER NOT NULL CHECK (day BETWEEN 1 AND 30),
  status TEXT NOT NULL CHECK (status IN ('purchased', 'rejected')),
  reason_code TEXT NOT NULL,
  quantity_minor INTEGER NOT NULL CHECK (quantity_minor IN (0, 1)),
  spent_minor INTEGER NOT NULL CHECK (spent_minor >= 0),
  event_id TEXT NOT NULL UNIQUE,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  CHECK ((status = 'purchased' AND reason_code = 'none' AND quantity_minor = 1 AND spent_minor > 0)
     OR (status = 'rejected' AND reason_code IN ('budget_exceeded', 'insufficient_funds', 'insufficient_stock') AND quantity_minor = 0 AND spent_minor = 0)),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (offer_id) REFERENCES m2_store_offers(offer_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-finite-store-009-2026-09-23', '2026-09-23T00:00:00Z');
