package normalize

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hegarty/shop_platform/event"

	"github.com/hegarty/shop_ingestor/internal/shopify"
)

// NormalizedOrdersTopic is where canonical CommerceEvents are published
// after normalization.
const NormalizedOrdersTopic = "shopify.orders.normalized"

// OrderFetcher fetches an order's current state by GID. Satisfied by
// *shopify.Client.
type OrderFetcher interface {
	OrderByID(ctx context.Context, gid string) (shopify.Order, error)
}

// OrderStore persists a normalized order.
type OrderStore interface {
	UpsertOrder(ctx context.Context, tenantID string, attrs event.OrderAttributes, sourceEventID string) error
}

// Publisher publishes a value to a topic, keyed by tenant.
type Publisher interface {
	Publish(ctx context.Context, topic, tenantID, eventType string, value any) error
}

// webhookOrderRef is the minimal shape needed to identify which order a
// webhook is about — present regardless of API version, in both the
// classic REST payload (admin_graphql_api_id) and any newer format.
type webhookOrderRef struct {
	AdminGraphQLAPIID string `json:"admin_graphql_api_id"`
}

// Consumer normalizes shopify.orders.raw messages: re-fetches the order's
// current state via GraphQL (see shopify.Client.OrderByID's doc comment for
// why, rather than parsing the REST webhook body directly), classifies and
// derives money fields, upserts to Postgres, and publishes the canonical
// CommerceEvent.
type Consumer struct {
	Fetcher   OrderFetcher
	Store     OrderStore
	Publisher Publisher
}

// HandleRawEnvelope processes one shopify.orders.raw message.
func (c *Consumer) HandleRawEnvelope(ctx context.Context, env event.RawEnvelope) error {
	var ref webhookOrderRef
	if err := json.Unmarshal(env.Payload, &ref); err != nil {
		return fmt.Errorf("normalize: parse webhook payload for order ref: %w", err)
	}
	if ref.AdminGraphQLAPIID == "" {
		return fmt.Errorf("normalize: webhook payload missing admin_graphql_api_id")
	}

	order, err := c.Fetcher.OrderByID(ctx, ref.AdminGraphQLAPIID)
	if err != nil {
		return fmt.Errorf("normalize: fetch order %s: %w", ref.AdminGraphQLAPIID, err)
	}

	attrs, err := FromShopifyOrder(order)
	if err != nil {
		return fmt.Errorf("normalize: derive attributes for order %s: %w", ref.AdminGraphQLAPIID, err)
	}

	if err := c.Store.UpsertOrder(ctx, env.TenantID, attrs, env.EventID); err != nil {
		return fmt.Errorf("normalize: persist order %s: %w", ref.AdminGraphQLAPIID, err)
	}

	commerceEvent, err := event.NewOrderEvent(env.EventID, env.TenantID, commerceEventType(env.EventType), env.Source, attrs.UpdatedAt, attrs)
	if err != nil {
		return fmt.Errorf("normalize: build commerce event: %w", err)
	}

	if err := c.Publisher.Publish(ctx, NormalizedOrdersTopic, env.TenantID, string(commerceEvent.Type), commerceEvent); err != nil {
		return fmt.Errorf("normalize: publish commerce event: %w", err)
	}
	return nil
}

// commerceEventType maps a Shopify webhook topic to a canonical event.Type.
// Defaults to TypeOrderUpdated for anything not explicitly a creation —
// re-fetching current state makes "created vs. updated" mostly a labeling
// distinction for consumers, not a behavioral one.
func commerceEventType(shopifyTopic string) event.Type {
	switch shopifyTopic {
	case "orders/create":
		return event.TypeOrderCreated
	default:
		return event.TypeOrderUpdated
	}
}
