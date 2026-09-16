// Package store persists normalized orders to PostgreSQL.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hegarty/shop_platform/event"
)

// OrderStore upserts normalized orders, keyed on (tenant_id,
// shopify_order_id) — reprocessing the same order (a duplicate webhook that
// slipped past idempotency, or a deliberate reconciliation re-fetch) is
// always safe.
type OrderStore struct {
	pool *pgxpool.Pool
}

func NewOrderStore(pool *pgxpool.Pool) *OrderStore {
	return &OrderStore{pool: pool}
}

// UpsertOrder resolves the Collective partner (if any) and writes the order
// row. sourceEventID is the RawEnvelope.event_id that produced this write,
// recorded for traceability back to the raw archive.
func (s *OrderStore) UpsertOrder(ctx context.Context, tenantID string, attrs event.OrderAttributes, sourceEventID string) error {
	var partnerID *int64
	if attrs.CollectivePartnerName != nil {
		id, err := s.upsertCollectivePartner(ctx, tenantID, *attrs.CollectivePartnerName)
		if err != nil {
			return err
		}
		partnerID = &id
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO orders (
			tenant_id, shopify_order_id, order_number,
			created_at, updated_at, processed_at,
			currency,
			gross_sales, discounts, returns, net_sales, shipping, tax, total_sales,
			channel, collective_partner_id,
			financial_status, fulfillment_status,
			classification_version, source_event_id, raw_metadata
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7,
			$8, $9, $10, $11, $12, $13, $14,
			$15, $16,
			$17, $18,
			$19, $20, $21
		)
		ON CONFLICT (tenant_id, shopify_order_id) DO UPDATE SET
			order_number           = EXCLUDED.order_number,
			updated_at             = EXCLUDED.updated_at,
			processed_at           = EXCLUDED.processed_at,
			gross_sales            = EXCLUDED.gross_sales,
			discounts              = EXCLUDED.discounts,
			returns                = EXCLUDED.returns,
			net_sales              = EXCLUDED.net_sales,
			shipping               = EXCLUDED.shipping,
			tax                    = EXCLUDED.tax,
			total_sales            = EXCLUDED.total_sales,
			channel                = EXCLUDED.channel,
			collective_partner_id  = EXCLUDED.collective_partner_id,
			financial_status       = EXCLUDED.financial_status,
			fulfillment_status     = EXCLUDED.fulfillment_status,
			classification_version = EXCLUDED.classification_version,
			source_event_id        = EXCLUDED.source_event_id,
			raw_metadata           = EXCLUDED.raw_metadata
	`,
		tenantID, attrs.ShopifyOrderID, attrs.OrderNumber,
		attrs.CreatedAt, attrs.UpdatedAt, attrs.ProcessedAt,
		attrs.Currency,
		attrs.GrossSales.DecimalString(), attrs.Discounts.DecimalString(), attrs.Returns.DecimalString(),
		attrs.NetSales.DecimalString(), attrs.Shipping.DecimalString(), attrs.Tax.DecimalString(), attrs.TotalSales.DecimalString(),
		string(attrs.Channel), partnerID,
		attrs.FinancialStatus, attrs.FulfillmentStatus,
		attrs.ClassificationVersion, sourceEventID, attrs.RawMetadata,
	)
	if err != nil {
		return fmt.Errorf("store: upsert order %s: %w", attrs.ShopifyOrderID, err)
	}
	return nil
}

func (s *OrderStore) upsertCollectivePartner(ctx context.Context, tenantID, name string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO collective_partners (tenant_id, name) VALUES ($1, $2)
		ON CONFLICT (tenant_id, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, tenantID, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: upsert collective partner %q: %w", name, err)
	}
	return id, nil
}
