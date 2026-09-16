// Package idempotency guards against Shopify's at-least-once webhook
// delivery (Shopify explicitly does not guarantee exactly-once) by
// recording which (tenant, webhook delivery ID) pairs have already been
// processed.
package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store checks and records processed webhook deliveries in Postgres.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// MarkProcessed records (tenantID, webhookID) as processed. Returns
// (true, nil) if this call recorded it for the first time, (false, nil) if
// it was already recorded (a duplicate delivery — the caller should skip
// reprocessing), or a non-nil error for anything else.
func (s *Store) MarkProcessed(ctx context.Context, tenantID, webhookID string) (firstTime bool, err error) {
	_, err = s.pool.Exec(ctx,
		`INSERT INTO processed_webhooks (tenant_id, webhook_id) VALUES ($1, $2)`,
		tenantID, webhookID,
	)
	if err == nil {
		return true, nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return false, nil
	}
	return false, fmt.Errorf("idempotency: mark processed: %w", err)
}
