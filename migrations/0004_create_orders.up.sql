CREATE TABLE orders (
    id                      BIGSERIAL PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    shopify_order_id        TEXT NOT NULL,
    order_number            TEXT NOT NULL,

    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL,
    processed_at            TIMESTAMPTZ,

    currency                TEXT NOT NULL,

    gross_sales             NUMERIC(12,2) NOT NULL,
    discounts               NUMERIC(12,2) NOT NULL DEFAULT 0,
    returns                 NUMERIC(12,2) NOT NULL DEFAULT 0,
    net_sales               NUMERIC(12,2) NOT NULL,
    shipping                NUMERIC(12,2) NOT NULL DEFAULT 0,
    tax                     NUMERIC(12,2) NOT NULL DEFAULT 0,
    total_sales             NUMERIC(12,2) NOT NULL,

    channel                 TEXT NOT NULL CHECK (channel IN ('shop', 'collective', 'unknown')),
    collective_partner_id   BIGINT REFERENCES collective_partners(id),

    financial_status        TEXT NOT NULL,
    fulfillment_status      TEXT NOT NULL,

    classification_version  INT NOT NULL DEFAULT 1,
    normalization_version   INT NOT NULL DEFAULT 1,
    source_event_id         TEXT NOT NULL,
    reconciled_at           TIMESTAMPTZ,
    raw_metadata            JSONB NOT NULL,

    UNIQUE (tenant_id, shopify_order_id)
);

CREATE INDEX idx_orders_tenant_created ON orders (tenant_id, created_at);
CREATE INDEX idx_orders_tenant_channel ON orders (tenant_id, channel, created_at);
