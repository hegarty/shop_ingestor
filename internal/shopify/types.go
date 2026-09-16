// Package shopify holds the subset of Shopify's Admin GraphQL API shapes
// this platform actually uses, and the client for reconciliation queries.
// Field names and semantics are taken from Shopify's current schema
// (https://shopify.dev/docs/api/admin-graphql/latest/objects/Order), not
// assumed — see shop_docs/docs/event-model.md for the citations.
package shopify

// Money mirrors Shopify's MoneyV2 shape (the leaf of a MoneyBag).
type Money struct {
	Amount       string `json:"amount"`
	CurrencyCode string `json:"currencyCode"`
}

// MoneyBag mirrors Shopify's MoneyBag: shop-currency and presentment-
// currency views of the same amount. This platform only ever uses
// ShopMoney — the store's own currency — never the customer-facing
// presentment currency, since sales reporting is in the merchant's terms.
type MoneyBag struct {
	ShopMoney Money `json:"shopMoney"`
}

// LineItem is the subset of Shopify's LineItem needed to derive gross
// sales and discounts (see the derivation table in
// shop_docs/docs/event-model.md — Shopify has no order-level "pre-discount
// total" field, so this has to be summed from line items).
type LineItem struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Quantity           int      `json:"quantity"`
	OriginalTotalSet   MoneyBag `json:"originalTotalSet"`   // before any discount
	DiscountedTotalSet MoneyBag `json:"discountedTotalSet"` // after line-level discounts
}

// Order is the subset of Shopify's Order object this platform normalizes.
type Order struct {
	ID                       string             `json:"id"` // GID, e.g. "gid://shopify/Order/123"
	Name                     string             `json:"name"`
	CreatedAt                string             `json:"createdAt"`
	UpdatedAt                string             `json:"updatedAt"`
	ProcessedAt              *string            `json:"processedAt"`
	Tags                     []string           `json:"tags"`
	DisplayFinancialStatus   string             `json:"displayFinancialStatus"`
	DisplayFulfillmentStatus string             `json:"displayFulfillmentStatus"`
	CurrentShippingPriceSet  MoneyBag           `json:"currentShippingPriceSet"`
	CurrentTotalTaxSet       MoneyBag           `json:"currentTotalTaxSet"`
	CurrentTotalPriceSet     MoneyBag           `json:"currentTotalPriceSet"`
	TotalRefundedSet         MoneyBag           `json:"totalRefundedSet"`
	TotalRefundedShippingSet MoneyBag           `json:"totalRefundedShippingSet"`
	CartDiscountAmountSet    *MoneyBag          `json:"cartDiscountAmountSet"`
	LineItems                LineItemConnection `json:"lineItems"`
}

// LineItemConnection mirrors Shopify's connection/edges/node pagination
// shape for an order's line items.
type LineItemConnection struct {
	Edges []struct {
		Node LineItem `json:"node"`
	} `json:"edges"`
}

// Lines returns the order's line items unwrapped from the connection shape.
func (o Order) Lines() []LineItem {
	lines := make([]LineItem, len(o.LineItems.Edges))
	for i, e := range o.LineItems.Edges {
		lines[i] = e.Node
	}
	return lines
}
