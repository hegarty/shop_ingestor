// Package webhook receives and verifies Shopify webhooks, then publishes
// them onto the event bus. It deliberately does no normalization or
// analytics work synchronously — see shop_docs/docs/architecture.md.
package webhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/hegarty/shop_platform/event"
	"github.com/hegarty/shop_platform/logging"
)

const (
	// maxBodyBytes bounds how much of a request body we'll read, so a
	// misbehaving or malicious sender can't exhaust memory. Shopify order
	// payloads are well under this in practice.
	maxBodyBytes = 5 << 20 // 5 MiB

	// RawOrdersTopic is where verified, unnormalized Shopify order webhook
	// payloads are published.
	RawOrdersTopic = "shopify.orders.raw"
)

// StoreResolver maps a Shopify shop domain to this platform's tenant ID.
type StoreResolver interface {
	ResolveTenant(ctx context.Context, shopDomain string) (tenantID string, err error)
}

// SecretResolver fetches a tenant's webhook signing secret. Backed by
// Secrets Manager in production — see shop_docs/docs/security.md.
type SecretResolver interface {
	WebhookSecret(ctx context.Context, tenantID string) (secret string, err error)
}

// Publisher publishes a value to a topic, keyed by tenant.
type Publisher interface {
	Publish(ctx context.Context, topic, tenantID, eventType string, value any) error
}

// Archiver durably archives a raw envelope.
type Archiver interface {
	Put(ctx context.Context, env event.RawEnvelope) error
}

// IdempotencyChecker records whether a webhook delivery has already been
// processed.
type IdempotencyChecker interface {
	MarkProcessed(ctx context.Context, tenantID, webhookID string) (firstTime bool, err error)
}

// Handler is an http.Handler for one Shopify webhook topic (orders/create,
// orders/updated, etc. — the topic itself doesn't change the handling
// logic, only the event_type it stamps on the envelope).
type Handler struct {
	Stores      StoreResolver
	Secrets     SecretResolver
	Publisher   Publisher
	Archiver    Archiver
	Idempotency IdempotencyChecker
	Logger      *slog.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}

	shopDomain := r.Header.Get("X-Shopify-Shop-Domain")
	webhookID := r.Header.Get("X-Shopify-Webhook-Id")
	eventType := r.Header.Get("X-Shopify-Topic")
	signature := r.Header.Get("X-Shopify-Hmac-Sha256")

	if shopDomain == "" || webhookID == "" || eventType == "" {
		logger.WarnContext(ctx, "webhook rejected: missing required headers",
			slog.String("shop_domain", shopDomain))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	tenantID, err := h.Stores.ResolveTenant(ctx, shopDomain)
	if err != nil {
		logger.WarnContext(ctx, "webhook rejected: unknown shop domain",
			slog.String("shop_domain", shopDomain), slog.Any("error", err))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(body) > maxBodyBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}

	secret, err := h.Secrets.WebhookSecret(ctx, tenantID)
	if err != nil {
		logger.ErrorContext(ctx, "webhook: failed to resolve secret", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !VerifyHMAC(body, signature, secret) {
		logger.WarnContext(ctx, "webhook rejected: HMAC verification failed",
			slog.String("tenant_id", tenantID),
			slog.String("x_shopify_hmac_sha256", logging.RedactHeader("X-Shopify-Hmac-Sha256", signature)))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	firstTime, err := h.Idempotency.MarkProcessed(ctx, tenantID, webhookID)
	if err != nil {
		logger.ErrorContext(ctx, "webhook: idempotency check failed", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !firstTime {
		// Duplicate delivery — Shopify doesn't guarantee exactly-once. Not
		// an error: acknowledge quickly so Shopify doesn't keep retrying.
		logger.InfoContext(ctx, "webhook: duplicate delivery, skipping",
			slog.String("tenant_id", tenantID), slog.String("webhook_id", webhookID))
		w.WriteHeader(http.StatusOK)
		return
	}

	if !json.Valid(body) {
		logger.WarnContext(ctx, "webhook rejected: body is not valid JSON", slog.String("tenant_id", tenantID))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	env := event.RawEnvelope{
		EventID:       newEventID(),
		TenantID:      tenantID,
		Source:        "shopify",
		EventType:     eventType,
		ShopDomain:    shopDomain,
		ReceivedAt:    time.Now().UTC(),
		SchemaVersion: event.CurrentRawSchemaVersion,
		Payload:       json.RawMessage(body),
	}

	if err := h.Archiver.Put(ctx, env); err != nil {
		// Archiving failure is logged but does not block publishing — the
		// event bus is the more time-sensitive path, and a missed archive
		// write can be backfilled from Redpanda while it still has the
		// message, unlike a missed publish.
		logger.ErrorContext(ctx, "webhook: raw archive write failed", slog.Any("error", err))
	}

	if err := h.Publisher.Publish(ctx, RawOrdersTopic, tenantID, eventType, env); err != nil {
		logger.ErrorContext(ctx, "webhook: publish failed", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func newEventID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
