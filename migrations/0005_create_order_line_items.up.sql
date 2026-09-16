CREATE TABLE order_line_items (
    id                    BIGSERIAL PRIMARY KEY,
    order_id              BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    tenant_id             TEXT NOT NULL REFERENCES tenants(id),
    shopify_line_item_id  TEXT NOT NULL,
    title                 TEXT NOT NULL,
    quantity              INT NOT NULL,
    price                 NUMERIC(12,2) NOT NULL,

    UNIQUE (order_id, shopify_line_item_id)
);
