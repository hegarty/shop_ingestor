-- Ingestion-level idempotency: Shopify does not guarantee exactly-once
-- webhook delivery. This is deliberately separate from the orders table's
-- own (tenant_id, shopify_order_id) uniqueness, which handles
-- normalization-level idempotency (safe to reprocess the same order).
CREATE TABLE processed_webhooks (
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    webhook_id   TEXT NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, webhook_id)
);
