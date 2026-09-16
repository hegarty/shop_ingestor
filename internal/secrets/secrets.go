// Package secrets reads tenant credentials from AWS Secrets Manager — the
// sole authoritative store for this platform's secrets. See
// shop_docs/docs/security.md.
package secrets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// ShopifyCredentials is the JSON shape stored at
// commerce-intel/shopify/<tenant> (see eks-prod's secrets/ Terragrunt
// units).
type ShopifyCredentials struct {
	AdminAPIToken        string `json:"admin_api_token"`
	WebhookSigningSecret string `json:"webhook_signing_secret"`
}

// Resolver fetches per-tenant Shopify credentials from Secrets Manager.
type Resolver struct {
	client *secretsmanager.Client
}

func NewResolver(client *secretsmanager.Client) *Resolver {
	return &Resolver{client: client}
}

func (r *Resolver) shopifyCredentials(ctx context.Context, tenantID string) (ShopifyCredentials, error) {
	secretID := fmt.Sprintf("commerce-intel/shopify/%s", tenantID)
	out, err := r.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return ShopifyCredentials{}, fmt.Errorf("secrets: get %s: %w", secretID, err)
	}

	var creds ShopifyCredentials
	if err := json.Unmarshal([]byte(aws.ToString(out.SecretString)), &creds); err != nil {
		return ShopifyCredentials{}, fmt.Errorf("secrets: decode %s: %w", secretID, err)
	}
	return creds, nil
}

// WebhookSecret implements webhook.SecretResolver.
func (r *Resolver) WebhookSecret(ctx context.Context, tenantID string) (string, error) {
	creds, err := r.shopifyCredentials(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return creds.WebhookSigningSecret, nil
}

// AdminAPIToken returns a tenant's Shopify Admin API token, for the
// reconciliation client.
func (r *Resolver) AdminAPIToken(ctx context.Context, tenantID string) (string, error) {
	creds, err := r.shopifyCredentials(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return creds.AdminAPIToken, nil
}
