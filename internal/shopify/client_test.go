package shopify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOrdersUpdatedSince_PaginatesAndSendsAccessToken(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Shopify-Access-Token"); got != "test-token" {
			t.Errorf("access token header = %q, want test-token", got)
		}

		var req graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		pageCount++
		hasNext := pageCount == 1

		resp := struct {
			Data ordersQueryData `json:"data"`
		}{}
		resp.Data.Orders.Edges = []struct {
			Cursor string `json:"cursor"`
			Node   Order  `json:"node"`
		}{
			{Cursor: "c1", Node: Order{ID: "gid://shopify/Order/1", Name: "#1001"}},
		}
		resp.Data.Orders.PageInfo.HasNextPage = hasNext
		if hasNext {
			resp.Data.Orders.PageInfo.EndCursor = "c1"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newClientWithBaseURL(server.Client(), server.URL, "test-token")

	orders, err := client.OrdersUpdatedSince(t.Context(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("OrdersUpdatedSince: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("got %d orders across pages, want 2", len(orders))
	}
	if pageCount != 2 {
		t.Errorf("expected 2 pages fetched, got %d", pageCount)
	}
}

func TestOrdersUpdatedSince_GraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"Throttled"}]}`))
	}))
	defer server.Close()

	client := newClientWithBaseURL(server.Client(), server.URL, "test-token")
	if _, err := client.OrdersUpdatedSince(t.Context(), time.Now()); err == nil {
		t.Fatal("expected error from GraphQL error response")
	}
}
