# Security Policy

This repository receives and processes real Shopify webhook traffic for a real business,
including customer order data. Please report suspected vulnerabilities privately.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting for this repository (Security tab -> "Report
a vulnerability"), or email me@terencehegarty.com if that isn't available. Include a
description, reproduction steps or a proof of concept, and the commit/version tested.

## Scope

This service verifies Shopify webhook HMAC signatures, deduplicates deliveries, archives
raw payloads to S3, and normalizes orders. Particularly interested in:

- HMAC verification bypasses
- Idempotency logic that could be exploited to process a forged/replayed webhook
- Anything that would let one tenant's data leak into another tenant's results
- Injection via any Shopify-controlled field (tags, order fields) into SQL, logs, or
  downstream systems

## Secrets handling

No Shopify token, webhook signing secret, or database credential should ever be committed
here. All are read from AWS Secrets Manager at runtime — see
[shop_docs/docs/security.md](https://github.com/hegarty/shop_docs/blob/main/docs/security.md).
If you find one committed, report it immediately via the private channel above.
