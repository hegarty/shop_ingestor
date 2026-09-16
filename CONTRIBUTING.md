# Contributing

See [shop_platform/CONTRIBUTING.md](https://github.com/hegarty/shop_platform/blob/main/CONTRIBUTING.md)
for the general workflow (PRs required, signed commits, required checks) — it applies
identically here.

## Local development

```bash
make test         # go test ./... -race -count=1
make lint          # golangci-lint run
make migrate-up    # apply migrations to whatever DATABASE_* env points at — never production
make docker         # build the container image locally
```

See [shop_docs/docs/local-development.md](https://github.com/hegarty/shop_docs/blob/main/docs/local-development.md)
for running Postgres/Redpanda locally and the full environment variable list.

## Testing conventions

- Pure logic (`internal/normalize`, `internal/webhook`'s HMAC check) has no external
  dependency and is always run in `make test`.
- `internal/webhook`'s handler tests use fakes for `StoreResolver`/`SecretResolver`/
  `Publisher`/`Archiver`/`IdempotencyChecker` — no real DB, S3, or Redpanda needed.
- `internal/shopify`'s client tests run against an `httptest.Server`, never the real
  Shopify API.
