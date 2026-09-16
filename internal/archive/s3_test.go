package archive

import (
	"testing"
	"time"

	"github.com/hegarty/shop_platform/event"
)

func TestObjectKey(t *testing.T) {
	env := event.RawEnvelope{
		EventID:    "evt-123",
		TenantID:   "devmoto",
		Source:     "shopify",
		EventType:  "orders/create",
		ReceivedAt: time.Date(2026, 9, 11, 19, 0, 0, 0, time.UTC),
	}

	want := "source=shopify/tenant=devmoto/event_type=orders_create/year=2026/month=09/day=11/evt-123.json"
	if got := ObjectKey(env); got != want {
		t.Errorf("ObjectKey = %q, want %q", got, want)
	}
}

func TestObjectKey_NormalizesToUTC(t *testing.T) {
	loc := time.FixedZone("UTC-8", -8*60*60)
	// 11pm Sep 10 in UTC-8 is already Sep 11 in UTC — the key must use UTC.
	env := event.RawEnvelope{
		EventID:    "evt-456",
		TenantID:   "devmoto",
		Source:     "shopify",
		EventType:  "orders/updated",
		ReceivedAt: time.Date(2026, 9, 10, 23, 0, 0, 0, loc),
	}

	want := "source=shopify/tenant=devmoto/event_type=orders_updated/year=2026/month=09/day=11/evt-456.json"
	if got := ObjectKey(env); got != want {
		t.Errorf("ObjectKey = %q, want %q", got, want)
	}
}
