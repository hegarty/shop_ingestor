package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StoreResolver looks up which tenant a Shopify shop domain belongs to.
type StoreResolver struct {
	pool *pgxpool.Pool
}

func NewStoreResolver(pool *pgxpool.Pool) *StoreResolver {
	return &StoreResolver{pool: pool}
}

// ResolveTenant implements webhook.StoreResolver.
func (r *StoreResolver) ResolveTenant(ctx context.Context, shopDomain string) (string, error) {
	var tenantID string
	err := r.pool.QueryRow(ctx,
		`SELECT tenant_id FROM shopify_stores WHERE shop_domain = $1`, shopDomain,
	).Scan(&tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("store: no tenant registered for shop domain %q", shopDomain)
		}
		return "", fmt.Errorf("store: resolve tenant for %q: %w", shopDomain, err)
	}
	return tenantID, nil
}
