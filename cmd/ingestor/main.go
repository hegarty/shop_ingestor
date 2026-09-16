// Command ingestor runs the Shopify webhook receiver and the normalization
// consumer in one process — see shop_docs/docs/architecture.md for why
// these two responsibilities share a deployable service.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/hegarty/shop_platform/config"
	"github.com/hegarty/shop_platform/db"
	"github.com/hegarty/shop_platform/event"
	"github.com/hegarty/shop_platform/logging"
	"github.com/hegarty/shop_platform/otelx"
	"github.com/hegarty/shop_platform/redpanda"

	"github.com/hegarty/shop_ingestor/internal/archive"
	"github.com/hegarty/shop_ingestor/internal/idempotency"
	"github.com/hegarty/shop_ingestor/internal/normalize"
	"github.com/hegarty/shop_ingestor/internal/secrets"
	"github.com/hegarty/shop_ingestor/internal/shopify"
	"github.com/hegarty/shop_ingestor/internal/store"
	"github.com/hegarty/shop_ingestor/internal/webhook"
)

func main() {
	l := config.NewLoader()
	httpPort := l.StringDefault("HTTP_PORT", "8080")
	dbHost := l.String("DATABASE_HOST")
	dbPort := l.IntDefault("DATABASE_PORT", 5432)
	dbName := l.String("DATABASE_NAME")
	dbUser := l.String("DATABASE_USER")
	dbPassword := l.String("DATABASE_PASSWORD")
	redpandaBrokers := l.String("REDPANDA_BROKERS")
	rawArchiveBucket := l.String("RAW_ARCHIVE_BUCKET")
	awsRegion := l.StringDefault("AWS_REGION", "us-east-1")
	otelEndpoint := l.StringDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	logLevel := l.StringDefault("LOG_LEVEL", "info")
	// Single-tenant MVP wiring — see ADR-0007. Multi-tenant deployments need
	// a per-message tenant -> {shop domain, access token} resolution instead
	// of one Shopify client built once at startup.
	tenantID := l.String("TENANT_ID")
	shopDomain := l.String("SHOPIFY_SHOP_DOMAIN")
	if err := l.Err(); err != nil {
		slog.Error("configuration error", slog.Any("error", err))
		os.Exit(1)
	}

	logger := logging.New("shop-ingestor", parseLevel(logLevel))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if otelEndpoint != "" {
		shutdown, err := otelx.Bootstrap(ctx, otelx.Config{
			ServiceName: "shop-ingestor", ServiceVersion: "dev",
			Endpoint: otelEndpoint, Insecure: true,
		})
		if err != nil {
			logger.Error("otel bootstrap failed", slog.Any("error", err))
			os.Exit(1)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdown(shutdownCtx)
		}()
	}

	pool, err := db.Connect(ctx, db.Config{Host: dbHost, Port: dbPort, Database: dbName, User: dbUser}, dbPassword)
	if err != nil {
		logger.Error("db connect failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer pool.Close()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(awsRegion))
	if err != nil {
		logger.Error("aws config load failed", slog.Any("error", err))
		os.Exit(1)
	}
	secretsResolver := secrets.NewResolver(secretsmanager.NewFromConfig(awsCfg))
	s3Client := s3.NewFromConfig(awsCfg)
	archiver := archive.NewWriter(s3Client, rawArchiveBucket)

	producer, err := redpanda.NewProducer([]string{redpandaBrokers})
	if err != nil {
		logger.Error("redpanda producer init failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer producer.Close()

	accessToken, err := secretsResolver.AdminAPIToken(ctx, tenantID)
	if err != nil {
		logger.Error("failed to load Shopify access token", slog.Any("error", err))
		os.Exit(1)
	}
	shopifyClient := shopify.NewClient(nil, shopDomain, accessToken)

	handler := &webhook.Handler{
		Stores:      store.NewStoreResolver(pool),
		Secrets:     secretsResolver,
		Publisher:   producer,
		Archiver:    archiver,
		Idempotency: idempotency.NewStore(pool),
		Logger:      logger,
	}

	mux := http.NewServeMux()
	mux.Handle("/webhooks/orders/create", handler)
	mux.Handle("/webhooks/orders/updated", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	server := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	normalizer := &normalize.Consumer{
		Fetcher:   shopifyClient,
		Store:     store.NewOrderStore(pool),
		Publisher: producer,
	}
	go runNormalizationConsumer(ctx, logger, redpandaBrokers, normalizer)

	go func() {
		logger.Info("shop-ingestor listening", slog.String("port", httpPort))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

// runNormalizationConsumer consumes shopify.orders.raw and normalizes each
// message via normalizer. Handler errors are logged, not retried here —
// see shop_docs/docs/event-model.md's error-handling section for the
// shopify.orders.failed dead-letter path (not yet implemented).
func runNormalizationConsumer(ctx context.Context, logger *slog.Logger, brokers string, normalizer *normalize.Consumer) {
	consumer, err := redpanda.NewConsumer([]string{brokers}, "shop-ingestor-normalizer", []string{webhook.RawOrdersTopic})
	if err != nil {
		logger.Error("redpanda consumer init failed", slog.Any("error", err))
		return
	}
	defer consumer.Close()

	err = consumer.Run(ctx, func(ctx context.Context, record *kgo.Record) error {
		var env event.RawEnvelope
		if unmarshalErr := json.Unmarshal(record.Value, &env); unmarshalErr != nil {
			logger.Error("failed to decode raw envelope", slog.Any("error", unmarshalErr))
			return unmarshalErr
		}
		if handleErr := normalizer.HandleRawEnvelope(ctx, env); handleErr != nil {
			logger.Error("normalization failed",
				slog.String("event_id", env.EventID), slog.String("tenant_id", env.TenantID),
				slog.Any("error", handleErr))
			return handleErr
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		logger.Error("normalization consumer stopped", slog.Any("error", err))
	}
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
