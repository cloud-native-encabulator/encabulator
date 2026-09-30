-- The differential girdlespring's one table. Flag values are stored as JSON so
-- that adding a flag type later is a code change, not a schema change.
CREATE TABLE IF NOT EXISTS flags (
    key             TEXT PRIMARY KEY,
    type            TEXT        NOT NULL CHECK (type IN ('boolean', 'number')),
    value           JSONB       NOT NULL,
    rollout_percent INTEGER     CHECK (rollout_percent IS NULL OR (rollout_percent BETWEEN 0 AND 100)),
    description     TEXT        NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      TEXT        NOT NULL DEFAULT 'seed'
);
