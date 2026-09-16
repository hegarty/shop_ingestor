# shop_ingestor

Shopify webhook receiver, order normalization, and reconciliation for the commerce-intel
platform. See [shop_docs](https://github.com/hegarty/shop_docs) for the full architecture,
event model, and database model.

## What's here

- **`cmd/ingestor`** — HTTP server receiving Shopify webhooks (`orders/create`,
  `orders/updated`), plus a background Redpanda consumer that normalizes them.
- **`cmd/reconciler`** — polls Shopify's Admin GraphQL API for orders updated since the
  last run, the authoritative repair path for missed webhooks and downtime. Intended to
  run as a Kubernetes CronJob.
- **`cmd/migrate`** — applies/rolls back this repo's schema migrations.
- **`internal/webhook`** — HMAC verification, idempotency, archiving, publishing. No
  normalization happens synchronously in the webhook path.
- **`internal/normalize`** — Shopify Collective channel/partner classification (see the
  package doc comment and [shop_docs/docs/event-model.md](https://github.com/hegarty/shop_docs/blob/main/docs/event-model.md)
  for the citations) and money-field derivation from Shopify's GraphQL Order schema.
- **`internal/shopify`** — the Admin GraphQL client, used by both reconciliation and (to
  re-fetch full order state) the normalization consumer.
- **`internal/store`** — Postgres persistence.
- **`internal/archive`** — S3 raw-payload archival.
- **`internal/secrets`** — Secrets Manager credential resolution.

## Why webhooks re-fetch via GraphQL instead of parsing the webhook body

Shopify webhook payloads are REST-shaped; the reconciliation path (and this repo's
normalization logic) is built and tested against the GraphQL Admin API's schema, which
uses different field names and requires deriving gross sales from line items rather than
reading one order-level field. Rather than maintain two divergent, only-one-of-them-tested
normalization paths, the webhook consumer treats a webhook delivery as a change
notification — verify, dedupe, archive, then re-fetch the order's current state via the
same GraphQL query reconciliation uses. See `internal/normalize/consumer.go`'s doc comment.

## Environment variables

See [shop_docs/docs/local-development.md](https://github.com/hegarty/shop_docs/blob/main/docs/local-development.md)
for the full list. Service-specific: `TENANT_ID`, `SHOPIFY_SHOP_DOMAIN`,
`RAW_ARCHIVE_BUCKET`, `RECONCILE_LOOKBACK_HOURS` (reconciler only).

## Multi-tenancy status

`cmd/ingestor` and `cmd/reconciler` currently build one Shopify API client from
`TENANT_ID`/`SHOPIFY_SHOP_DOMAIN` at startup — correct for the single-tenant MVP, and a
deliberate, documented simplification (see
[ADR-0007](https://github.com/hegarty/shop_docs/blob/main/docs/adr/0007-multi-tenancy-from-day-one.md)).
A second tenant needs per-message tenant resolution to the right Shopify client instead.

## Development

```bash
make test
make lint
make migrate-up   # against a local/test database only
```
