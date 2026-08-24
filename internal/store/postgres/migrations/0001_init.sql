-- Schema for the claim-identification service.
--
-- Identifiers are TEXT rather than UUID: catalogue product IDs are opaque
-- strings owned by an upstream system, and a lookup with an unexpected format
-- should return "not found" rather than a type error.

CREATE TABLE IF NOT EXISTS products (
    id         TEXT PRIMARY KEY,
    mcr_id     TEXT,
    sku        TEXT,
    title      TEXT        NOT NULL,
    brand      TEXT,
    status     TEXT,
    -- Full product document, including the claims array once enriched.
    data       JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS workflows (
    id           TEXT PRIMARY KEY,
    product_id   TEXT        NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    status       TEXT        NOT NULL CHECK (status IN ('IN_PROGRESS', 'COMPLETED', 'FAILED')),
    error        TEXT,
    -- Per-step audit trail of the run.
    steps        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    -- Enriched product snapshot produced by this run.
    result       JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS workflows_product_id_idx ON workflows (product_id);
CREATE INDEX IF NOT EXISTS workflows_status_idx ON workflows (status);

-- Claims are also stored relationally so they can be queried across the
-- catalogue ("every restricted medical claim in the Noodles category").
CREATE TABLE IF NOT EXISTS claims (
    id          TEXT PRIMARY KEY,
    product_id  TEXT        NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    workflow_id TEXT        REFERENCES workflows (id) ON DELETE SET NULL,
    claim_type  TEXT        NOT NULL,
    claim_value TEXT        NOT NULL,
    status      TEXT        NOT NULL,
    source      TEXT,
    evidence    TEXT,
    rule_id     TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS claims_product_id_idx ON claims (product_id);
CREATE INDEX IF NOT EXISTS claims_claim_type_idx ON claims (claim_type);
