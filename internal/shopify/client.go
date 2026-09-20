package shopify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AdminAPIVersion pins the Shopify Admin API version this client targets.
// Bump deliberately, not automatically — Shopify's versioned API means an
// old pin keeps working (for a supported window) rather than breaking
// silently on Shopify's release cadence.
const AdminAPIVersion = "2025-10"

// Client queries Shopify's Admin GraphQL API for reconciliation — the
// authoritative source used to repair missed webhooks, catch up after
// downtime, and correct for Shopify's own eventual consistency. See
// shop_docs/docs/architecture.md's "webhooks = low latency, Shopify API =
// authoritative reconciliation" split.
type Client struct {
	httpClient  *http.Client
	baseURL     string // https://<shop>.myshopify.com/admin/api/<version>/graphql.json — overridable for tests
	accessToken string
}

func NewClient(httpClient *http.Client, shopDomain, accessToken string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		httpClient:  httpClient,
		baseURL:     fmt.Sprintf("https://%s/admin/api/%s/graphql.json", shopDomain, AdminAPIVersion),
		accessToken: accessToken,
	}
}

// newClientWithBaseURL is used by tests to point at an httptest.Server
// instead of a real Shopify domain.
func newClientWithBaseURL(httpClient *http.Client, baseURL, accessToken string) *Client {
	return &Client{httpClient: httpClient, baseURL: baseURL, accessToken: accessToken}
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// ordersQuery fetches orders updated since a given time, one page at a
// time. `query: "updated_at:>..."` and cursor-based pagination match
// Shopify's documented pattern
// (https://shopify.dev/docs/api/admin-graphql/latest/queries/orders).
const ordersQuery = `
query ReconcileOrders($cursor: String, $filter: String!) {
  orders(first: 50, after: $cursor, query: $filter, sortKey: UPDATED_AT) {
    edges {
      cursor
      node {
        id
        name
        createdAt
        updatedAt
        processedAt
        tags
        displayFinancialStatus
        displayFulfillmentStatus
        currentShippingPriceSet { shopMoney { amount currencyCode } }
        currentTotalTaxSet { shopMoney { amount currencyCode } }
        currentTotalPriceSet { shopMoney { amount currencyCode } }
        totalRefundedSet { shopMoney { amount currencyCode } }
        totalRefundedShippingSet { shopMoney { amount currencyCode } }
        cartDiscountAmountSet { shopMoney { amount currencyCode } }
        lineItems(first: 50) {
          edges {
            node {
              id
              title
              quantity
              originalTotalSet { shopMoney { amount currencyCode } }
              discountedTotalSet { shopMoney { amount currencyCode } }
            }
          }
        }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

type ordersQueryData struct {
	Orders struct {
		Edges []struct {
			Cursor string `json:"cursor"`
			Node   Order  `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"orders"`
}

// orderQuery fetches a single order by its GID, with the same field
// selection as ordersQuery. Webhook payloads are REST-shaped (different
// field names/derivation than this GraphQL schema — see
// shop_docs/docs/event-model.md), so rather than maintaining two divergent
// normalization paths, the webhook consumer treats a webhook as a change
// notification and re-fetches current state here, through the one
// normalization path (normalize.FromShopifyOrder) that's actually verified
// against Shopify's GraphQL schema.
const orderQuery = `
query GetOrder($id: ID!) {
  order(id: $id) {
    id
    name
    createdAt
    updatedAt
    processedAt
    tags
    displayFinancialStatus
    displayFulfillmentStatus
    currentShippingPriceSet { shopMoney { amount currencyCode } }
    currentTotalTaxSet { shopMoney { amount currencyCode } }
    currentTotalPriceSet { shopMoney { amount currencyCode } }
    totalRefundedSet { shopMoney { amount currencyCode } }
    totalRefundedShippingSet { shopMoney { amount currencyCode } }
    cartDiscountAmountSet { shopMoney { amount currencyCode } }
    lineItems(first: 50) {
      edges {
        node {
          id
          title
          quantity
          originalTotalSet { shopMoney { amount currencyCode } }
          discountedTotalSet { shopMoney { amount currencyCode } }
        }
      }
    }
  }
}`

type orderQueryData struct {
	Order *Order `json:"order"`
}

// OrderByID fetches one order's current state by its GID
// (gid://shopify/Order/123).
func (c *Client) OrderByID(ctx context.Context, gid string) (Order, error) {
	var data orderQueryData
	if err := c.do(ctx, orderQuery, map[string]any{"id": gid}, &data); err != nil {
		return Order{}, err
	}
	if data.Order == nil {
		return Order{}, fmt.Errorf("shopify: order %s not found", gid)
	}
	return *data.Order, nil
}

// OrdersUpdatedSince fetches every order updated at or after `since`,
// paginating internally. `since` should include an overlap window (not
// exactly the last successful sync time) to absorb clock skew and
// Shopify's own eventual consistency — see
// shop_docs/docs/architecture.md's reconciliation section.
func (c *Client) OrdersUpdatedSince(ctx context.Context, since time.Time) ([]Order, error) {
	filter := fmt.Sprintf("updated_at:>='%s'", since.UTC().Format(time.RFC3339))

	var (
		all    []Order
		cursor *string
	)
	for {
		var data ordersQueryData
		if err := c.do(ctx, ordersQuery, map[string]any{"cursor": cursor, "filter": filter}, &data); err != nil {
			return nil, err
		}
		for _, edge := range data.Orders.Edges {
			all = append(all, edge.Node)
		}
		if !data.Orders.PageInfo.HasNextPage {
			break
		}
		c := data.Orders.PageInfo.EndCursor
		cursor = &c
	}
	return all, nil
}

func (c *Client) do(ctx context.Context, query string, variables map[string]any, out any) error {
	reqBody, err := json.Marshal(graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("shopify: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("shopify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("shopify: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("shopify: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shopify: unexpected status %d: %s", resp.StatusCode, truncate(body, 500))
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return fmt.Errorf("shopify: decode response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("shopify: graphql error: %s", gqlResp.Errors[0].Message)
	}
	if err := json.Unmarshal(gqlResp.Data, out); err != nil {
		return fmt.Errorf("shopify: decode data: %w", err)
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
