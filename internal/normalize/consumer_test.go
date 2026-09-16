package normalize

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hegarty/shop_platform/event"

	"github.com/hegarty/shop_ingestor/internal/shopify"
)

type fakeFetcher struct {
	order shopify.Order
	err   error
}

func (f *fakeFetcher) OrderByID(_ context.Context, _ string) (shopify.Order, error) {
	return f.order, f.err
}

type fakeStore struct {
	upserted []event.OrderAttributes
}

func (f *fakeStore) UpsertOrder(_ context.Context, _ string, attrs event.OrderAttributes, _ string) error {
	f.upserted = append(f.upserted, attrs)
	return nil
}

type fakePublisher struct {
	published []struct {
		topic, tenantID, eventType string
		value                      any
	}
}

func (f *fakePublisher) Publish(_ context.Context, topic, tenantID, eventType string, value any) error {
	f.published = append(f.published, struct {
		topic, tenantID, eventType string
		value                      any
	}{topic, tenantID, eventType, value})
	return nil
}

func TestConsumer_HandleRawEnvelope(t *testing.T) {
	order := shopify.Order{
		ID:                       "gid://shopify/Order/1",
		Name:                     "#1001",
		CreatedAt:                "2026-09-11T15:31:12Z",
		UpdatedAt:                "2026-09-11T15:31:12Z",
		Tags:                     []string{"vip"},
		DisplayFinancialStatus:   "PAID",
		DisplayFulfillmentStatus: "UNFULFILLED",
		CurrentShippingPriceSet:  moneyBag("0.00"),
		CurrentTotalTaxSet:       moneyBag("0.00"),
		CurrentTotalPriceSet:     moneyBag("50.00"),
		TotalRefundedSet:         moneyBag("0.00"),
		TotalRefundedShippingSet: moneyBag("0.00"),
		LineItems: shopify.LineItemConnection{
			Edges: []struct {
				Node shopify.LineItem `json:"node"`
			}{
				{Node: shopify.LineItem{ID: "li-1", OriginalTotalSet: moneyBag("50.00"), DiscountedTotalSet: moneyBag("50.00")}},
			},
		},
	}

	fetcher := &fakeFetcher{order: order}
	store := &fakeStore{}
	pub := &fakePublisher{}
	c := &Consumer{Fetcher: fetcher, Store: store, Publisher: pub}

	payload, _ := json.Marshal(map[string]string{"admin_graphql_api_id": "gid://shopify/Order/1"})
	env := event.RawEnvelope{
		EventID:    "evt-1",
		TenantID:   "devmoto",
		Source:     "shopify",
		EventType:  "orders/create",
		ReceivedAt: time.Now(),
		Payload:    payload,
	}

	if err := c.HandleRawEnvelope(t.Context(), env); err != nil {
		t.Fatalf("HandleRawEnvelope: %v", err)
	}

	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(pub.published))
	}
	if pub.published[0].topic != NormalizedOrdersTopic {
		t.Errorf("topic = %q, want %q", pub.published[0].topic, NormalizedOrdersTopic)
	}
	if pub.published[0].eventType != string(event.TypeOrderCreated) {
		t.Errorf("eventType = %q, want %q", pub.published[0].eventType, event.TypeOrderCreated)
	}
}

func TestConsumer_HandleRawEnvelope_MissingOrderRef(t *testing.T) {
	c := &Consumer{Fetcher: &fakeFetcher{}, Store: &fakeStore{}, Publisher: &fakePublisher{}}
	env := event.RawEnvelope{Payload: json.RawMessage(`{}`)}

	if err := c.HandleRawEnvelope(t.Context(), env); err == nil {
		t.Fatal("expected error for missing admin_graphql_api_id")
	}
}
