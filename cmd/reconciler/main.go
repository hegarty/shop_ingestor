// Command reconciler polls Shopify's Admin GraphQL API for orders updated
// since the last successful run and normalizes them directly — the
// authoritative repair path for missed webhooks, downtime, and Shopify's
// own eventual consistency. Intended to run as a Kubernetes CronJob. See
// shop_docs/docs/architecture.md.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/hegarty/shop_platform/config"
	"github.com/hegarty/shop_platform/db"
	"github.com/hegarty/shop_platform/logging"
	"github.com/hegarty/shop_platform/redpanda"

	"github.com/hegarty/shop_ingestor/internal/normalize"
	"github.com/hegarty/shop_ingestor/internal/secrets"
	"github.com/hegarty/shop_ingestor/internal/shopify"
	"github.com/hegarty/shop_ingestor/internal/store"
)

// overlapWindow protects against clock skew and Shopify's own eventual
// consistency — reconciliation always looks slightly further back than the
// exact last-success timestamp. See shop_docs/docs/architecture.md.
const overlapWindow = 15 * time.Minute

func main() {
	logger := logging.New("shop-reconciler", slog.LevelInfo)
	slog.SetDefault(logger)

	l := config.NewLoader()
	dbHost := l.String("DATABASE_HOST")
	dbPort := l.IntDefault("DATABASE_PORT", 5432)
	dbName := l.String("DATABASE_NAME")
	dbUser := l.String("DATABASE_USER")
	dbPassword := l.String("DATABASE_PASSWORD")
	redpandaBrokers := l.String("REDPANDA_BROKERS")
	awsRegion := l.StringDefault("AWS_REGION", "us-east-1")
	tenantID := l.String("TENANT_ID")
	shopDomain := l.String("SHOPIFY_SHOP_DOMAIN")
	lookbackHours := l.IntDefault("RECONCILE_LOOKBACK_HOURS", 24)
	if err := l.Err(); err != nil {
		logger.Error("configuration error", slog.Any("error", err))
		os.Exit(1)
	}

	ctx := context.Background()

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
	accessToken, err := secrets.NewResolver(secretsmanager.NewFromConfig(awsCfg)).AdminAPIToken(ctx, tenantID)
	if err != nil {
		logger.Error("failed to load Shopify access token", slog.Any("error", err))
		os.Exit(1)
	}
	shopifyClient := shopify.NewClient(nil, shopDomain, accessToken)

	producer, err := redpanda.NewProducer([]string{redpandaBrokers})
	if err != nil {
		logger.Error("redpanda producer init failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer producer.Close()

	orderStore := store.NewOrderStore(pool)

	since := time.Now().Add(-time.Duration(lookbackHours)*time.Hour - overlapWindow)
	logger.Info("reconciliation starting", slog.Time("since", since))

	orders, err := shopifyClient.OrdersUpdatedSince(ctx, since)
	if err != nil {
		logger.Error("failed to fetch orders for reconciliation", slog.Any("error", err))
		os.Exit(1)
	}

	var succeeded, failed int
	for _, o := range orders {
		attrs, err := normalize.FromShopifyOrder(o)
		if err != nil {
			logger.Error("reconciliation: normalize failed", slog.String("order_id", o.ID), slog.Any("error", err))
			failed++
			continue
		}
		if err := orderStore.UpsertOrder(ctx, tenantID, attrs, "reconciliation"); err != nil {
			logger.Error("reconciliation: upsert failed", slog.String("order_id", o.ID), slog.Any("error", err))
			failed++
			continue
		}
		succeeded++
	}

	logger.Info("reconciliation complete",
		slog.Int("orders_seen", len(orders)),
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)
	if failed > 0 {
		os.Exit(1)
	}
}
