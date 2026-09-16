package shopify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOrderByID_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Data orderQueryData `json:"data"`
		}{}
		order := Order{ID: "gid://shopify/Order/42", Name: "#1042"}
		resp.Data.Order = &order
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newClientWithBaseURL(server.Client(), server.URL, "test-token")
	order, err := client.OrderByID(t.Context(), "gid://shopify/Order/42")
	if err != nil {
		t.Fatalf("OrderByID: %v", err)
	}
	if order.Name != "#1042" {
		t.Errorf("Name = %q, want #1042", order.Name)
	}
}

func TestOrderByID_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"order":null}}`))
	}))
	defer server.Close()

	client := newClientWithBaseURL(server.Client(), server.URL, "test-token")
	if _, err := client.OrderByID(t.Context(), "gid://shopify/Order/999"); err == nil {
		t.Fatal("expected error for missing order")
	}
}
