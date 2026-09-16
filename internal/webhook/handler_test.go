package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hegarty/shop_platform/event"
)

type fakeStores struct {
	domain2tenant map[string]string
}

func (f *fakeStores) ResolveTenant(_ context.Context, shopDomain string) (string, error) {
	t, ok := f.domain2tenant[shopDomain]
	if !ok {
		return "", errNotFound
	}
	return t, nil
}

var errNotFound = errString("shop domain not found")

type errString string

func (e errString) Error() string { return string(e) }

type fakeSecrets struct{ secret string }

func (f *fakeSecrets) WebhookSecret(_ context.Context, _ string) (string, error) {
	return f.secret, nil
}

type fakePublisher struct {
	calls []struct{ topic, tenantID, eventType string }
}

func (f *fakePublisher) Publish(_ context.Context, topic, tenantID, eventType string, _ any) error {
	f.calls = append(f.calls, struct{ topic, tenantID, eventType string }{topic, tenantID, eventType})
	return nil
}

type fakeArchiver struct{ puts []event.RawEnvelope }

func (f *fakeArchiver) Put(_ context.Context, env event.RawEnvelope) error {
	f.puts = append(f.puts, env)
	return nil
}

type fakeIdempotency struct{ seen map[string]bool }

func (f *fakeIdempotency) MarkProcessed(_ context.Context, tenantID, webhookID string) (bool, error) {
	key := tenantID + ":" + webhookID
	if f.seen[key] {
		return false, nil
	}
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	f.seen[key] = true
	return true, nil
}

func newTestHandler(secret string) (*Handler, *fakePublisher, *fakeArchiver) {
	pub := &fakePublisher{}
	arc := &fakeArchiver{}
	h := &Handler{
		Stores:      &fakeStores{domain2tenant: map[string]string{"devmoto.myshopify.com": "devmoto"}},
		Secrets:     &fakeSecrets{secret: secret},
		Publisher:   pub,
		Archiver:    arc,
		Idempotency: &fakeIdempotency{},
	}
	return h, pub, arc
}

func request(body, shopDomain, webhookID, topic, signature string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/orders/create", strings.NewReader(body))
	req.Header.Set("X-Shopify-Shop-Domain", shopDomain)
	req.Header.Set("X-Shopify-Webhook-Id", webhookID)
	req.Header.Set("X-Shopify-Topic", topic)
	req.Header.Set("X-Shopify-Hmac-Sha256", signature)
	return req
}

func TestHandler_ValidWebhook_PublishesAndArchives(t *testing.T) {
	secret := "test-secret"
	body := `{"id":123,"tags":"vip"}`
	h, pub, arc := newTestHandler(secret)

	req := request(body, "devmoto.myshopify.com", "wh-1", "orders/create", sign([]byte(body), secret))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].tenantID != "devmoto" || pub.calls[0].topic != RawOrdersTopic {
		t.Errorf("unexpected publish call: %+v", pub.calls[0])
	}
	if len(arc.puts) != 1 {
		t.Fatalf("expected 1 archive put, got %d", len(arc.puts))
	}
}

func TestHandler_InvalidSignature_Rejected(t *testing.T) {
	body := `{"id":123}`
	h, pub, _ := newTestHandler("real-secret")

	req := request(body, "devmoto.myshopify.com", "wh-1", "orders/create", sign([]byte(body), "wrong-secret"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if len(pub.calls) != 0 {
		t.Error("expected no publish on invalid signature")
	}
}

func TestHandler_UnknownShopDomain_Rejected(t *testing.T) {
	secret := "test-secret"
	body := `{"id":123}`
	h, _, _ := newTestHandler(secret)

	req := request(body, "unknown.myshopify.com", "wh-1", "orders/create", sign([]byte(body), secret))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandler_DuplicateDelivery_AcknowledgedNotRepublished(t *testing.T) {
	secret := "test-secret"
	body := `{"id":123}`
	h, pub, _ := newTestHandler(secret)

	req1 := request(body, "devmoto.myshopify.com", "wh-dup", "orders/create", sign([]byte(body), secret))
	h.ServeHTTP(httptest.NewRecorder(), req1)

	req2 := request(body, "devmoto.myshopify.com", "wh-dup", "orders/create", sign([]byte(body), secret))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("duplicate delivery status = %d, want 200 (ack, don't retry-storm Shopify)", rec2.Code)
	}
	if len(pub.calls) != 1 {
		t.Errorf("expected exactly 1 publish across both deliveries, got %d", len(pub.calls))
	}
}

func TestHandler_MissingHeaders_Rejected(t *testing.T) {
	h, _, _ := newTestHandler("secret")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/orders/create", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
