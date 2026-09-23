-- M2 declared consumption and finite, paid supplier transfers.
CREATE TABLE m2_supplier_quotes (
  quote_id TEXT PRIMARY KEY,
  supplier_actor_id TEXT NOT NULL,
  store_actor_id TEXT NOT NULL,
  sku_id TEXT NOT NULL,
  currency_id TEXT NOT NULL,
  unit_price_minor INTEGER NOT NULL CHECK (unit_price_minor > 0),
  max_quantity_minor INTEGER NOT NULL CHECK (max_quantity_minor > 0),
  effective_from TEXT NOT NULL,
  effective_until TEXT,
  definition_event_id TEXT NOT NULL,
  CHECK (effective_until IS NULL OR effective_until > effective_from),
  FOREIGN KEY (supplier_actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (store_actor_id) REFERENCES m2_economic_actors(actor_id),
  FOREIGN KEY (sku_id) REFERENCES product_skus(sku_id),
  FOREIGN KEY (currency_id) REFERENCES currencies(currency_id),
  FOREIGN KEY (definition_event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_consumption_outcomes (
  scheduler_item_id TEXT PRIMARY KEY,
  purchase_item_id TEXT NOT NULL UNIQUE,
  day INTEGER NOT NULL CHECK (day BETWEEN 1 AND 30),
  status TEXT NOT NULL CHECK (status IN ('consumed', 'skipped')),
  reason_code TEXT NOT NULL,
  quantity_minor INTEGER NOT NULL CHECK (quantity_minor IN (0, 1)),
  event_id TEXT NOT NULL UNIQUE,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  CHECK ((status = 'consumed' AND reason_code = 'none' AND quantity_minor = 1)
     OR (status = 'skipped' AND reason_code = 'purchase_rejected' AND quantity_minor = 0)),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (purchase_item_id) REFERENCES m2_purchase_outcomes(scheduler_item_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

CREATE TABLE m2_restock_outcomes (
  scheduler_item_id TEXT PRIMARY KEY,
  quote_id TEXT NOT NULL,
  day INTEGER NOT NULL CHECK (day BETWEEN 1 AND 30),
  status TEXT NOT NULL CHECK (status IN ('restocked', 'rejected')),
  reason_code TEXT NOT NULL,
  quantity_minor INTEGER NOT NULL CHECK (quantity_minor >= 0),
  spent_minor INTEGER NOT NULL CHECK (spent_minor >= 0),
  event_id TEXT NOT NULL UNIQUE,
  event_sequence INTEGER NOT NULL CHECK (event_sequence > 0),
  CHECK ((status = 'restocked' AND reason_code = 'none' AND quantity_minor > 0 AND spent_minor > 0)
     OR (status = 'rejected' AND reason_code IN ('insufficient_store_funds', 'insufficient_supplier_stock') AND quantity_minor = 0 AND spent_minor = 0)),
  FOREIGN KEY (scheduler_item_id) REFERENCES scheduler_items(scheduler_item_id),
  FOREIGN KEY (quote_id) REFERENCES m2_supplier_quotes(quote_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-m2-consumption-supply-010-2026-09-23', '2026-09-23T00:00:00Z');
