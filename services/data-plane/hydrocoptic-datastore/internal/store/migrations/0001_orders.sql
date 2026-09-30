-- The order ledger. One row per order, one row per timeline entry, and one row
-- per event we have already applied.
CREATE TABLE IF NOT EXISTS orders (
    id              TEXT        PRIMARY KEY,
    customer        TEXT        NOT NULL,
    model           TEXT        NOT NULL,
    quantity        INTEGER     NOT NULL CHECK (quantity > 0),
    options         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status          TEXT        NOT NULL,
    stage           TEXT        NOT NULL DEFAULT '',
    attempt         INTEGER     NOT NULL DEFAULT 1,
    quote           JSONB,
    reservation_id  TEXT        NOT NULL DEFAULT '',
    serials         TEXT[]      NOT NULL DEFAULT '{}',
    carrier         TEXT        NOT NULL DEFAULT '',
    tracking_number TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Listing is always newest first, optionally narrowed by customer or status.
CREATE INDEX IF NOT EXISTS orders_created_at_idx ON orders (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS orders_customer_idx ON orders (customer, created_at DESC);
CREATE INDEX IF NOT EXISTS orders_status_idx ON orders (status, created_at DESC);

CREATE TABLE IF NOT EXISTS order_timeline (
    id       BIGSERIAL   PRIMARY KEY,
    order_id TEXT        NOT NULL,
    time     TIMESTAMPTZ NOT NULL,
    type     TEXT        NOT NULL,
    detail   TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS order_timeline_order_idx ON order_timeline (order_id, time, id);

-- Event deduplication. JetStream delivers at least once, so every event is
-- applied exactly once by recording its id here inside the same transaction.
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     TEXT        PRIMARY KEY,
    subject      TEXT        NOT NULL,
    order_id     TEXT        NOT NULL DEFAULT '',
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The throughput half of /v1/stats reads the recent timeline; keep it cheap.
CREATE INDEX IF NOT EXISTS order_timeline_type_time_idx ON order_timeline (type, time DESC);
