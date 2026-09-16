package normalize

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hegarty/shop_platform/event"
	"github.com/hegarty/shop_platform/money"

	"github.com/hegarty/shop_ingestor/internal/shopify"
)

// FromShopifyOrder derives shop_platform's canonical OrderAttributes from a
// Shopify GraphQL Order. See shop_docs/docs/event-model.md's money-field
// derivation table for exactly which Shopify fields each output field comes
// from and why (Shopify has no single "pre-discount total" field — gross
// sales has to be summed from line items).
func FromShopifyOrder(o shopify.Order) (event.OrderAttributes, error) {
	createdAt, err := time.Parse(time.RFC3339, o.CreatedAt)
	if err != nil {
		return event.OrderAttributes{}, fmt.Errorf("normalize: parse createdAt: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, o.UpdatedAt)
	if err != nil {
		return event.OrderAttributes{}, fmt.Errorf("normalize: parse updatedAt: %w", err)
	}
	var processedAt *time.Time
	if o.ProcessedAt != nil {
		t, err := time.Parse(time.RFC3339, *o.ProcessedAt)
		if err != nil {
			return event.OrderAttributes{}, fmt.Errorf("normalize: parse processedAt: %w", err)
		}
		processedAt = &t
	}

	var grossSales, lineDiscounts money.Amount
	for _, line := range o.Lines() {
		original, err := money.FromDecimalString(line.OriginalTotalSet.ShopMoney.Amount)
		if err != nil {
			return event.OrderAttributes{}, fmt.Errorf("normalize: line item %s originalTotalSet: %w", line.ID, err)
		}
		discounted, err := money.FromDecimalString(line.DiscountedTotalSet.ShopMoney.Amount)
		if err != nil {
			return event.OrderAttributes{}, fmt.Errorf("normalize: line item %s discountedTotalSet: %w", line.ID, err)
		}
		grossSales = grossSales.Add(original)
		lineDiscounts = lineDiscounts.Add(original.Sub(discounted))
	}

	var orderLevelDiscount money.Amount
	if o.CartDiscountAmountSet != nil {
		orderLevelDiscount, err = money.FromDecimalString(o.CartDiscountAmountSet.ShopMoney.Amount)
		if err != nil {
			return event.OrderAttributes{}, fmt.Errorf("normalize: cartDiscountAmountSet: %w", err)
		}
	}
	discounts := lineDiscounts.Add(orderLevelDiscount)

	totalRefunded, err := money.FromDecimalString(o.TotalRefundedSet.ShopMoney.Amount)
	if err != nil {
		return event.OrderAttributes{}, fmt.Errorf("normalize: totalRefundedSet: %w", err)
	}
	refundedShipping, err := money.FromDecimalString(o.TotalRefundedShippingSet.ShopMoney.Amount)
	if err != nil {
		return event.OrderAttributes{}, fmt.Errorf("normalize: totalRefundedShippingSet: %w", err)
	}
	returns := totalRefunded.Sub(refundedShipping)

	shipping, err := money.FromDecimalString(o.CurrentShippingPriceSet.ShopMoney.Amount)
	if err != nil {
		return event.OrderAttributes{}, fmt.Errorf("normalize: currentShippingPriceSet: %w", err)
	}
	tax, err := money.FromDecimalString(o.CurrentTotalTaxSet.ShopMoney.Amount)
	if err != nil {
		return event.OrderAttributes{}, fmt.Errorf("normalize: currentTotalTaxSet: %w", err)
	}
	totalSales, err := money.FromDecimalString(o.CurrentTotalPriceSet.ShopMoney.Amount)
	if err != nil {
		return event.OrderAttributes{}, fmt.Errorf("normalize: currentTotalPriceSet: %w", err)
	}

	netSales := grossSales.Sub(discounts).Sub(returns)

	channel, partnerName := ClassifyChannel(o.Tags)

	rawMetadata, err := json.Marshal(struct {
		Tags                     []string `json:"tags"`
		DisplayFinancialStatus   string   `json:"display_financial_status"`
		DisplayFulfillmentStatus string   `json:"display_fulfillment_status"`
		LineItemCount            int      `json:"line_item_count"`
	}{
		Tags:                     o.Tags,
		DisplayFinancialStatus:   o.DisplayFinancialStatus,
		DisplayFulfillmentStatus: o.DisplayFulfillmentStatus,
		LineItemCount:            len(o.LineItems.Edges),
	})
	if err != nil {
		return event.OrderAttributes{}, fmt.Errorf("normalize: marshal raw_metadata: %w", err)
	}

	return event.OrderAttributes{
		ShopifyOrderID:        o.ID,
		OrderNumber:           o.Name,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		ProcessedAt:           processedAt,
		Currency:              o.CurrentTotalPriceSet.ShopMoney.CurrencyCode,
		GrossSales:            grossSales,
		Discounts:             discounts,
		Returns:               returns,
		NetSales:              netSales,
		Shipping:              shipping,
		Tax:                   tax,
		TotalSales:            totalSales,
		Channel:               channel,
		CollectivePartnerName: partnerName,
		FinancialStatus:       strings.ToLower(o.DisplayFinancialStatus),
		FulfillmentStatus:     strings.ToLower(o.DisplayFulfillmentStatus),
		ClassificationVersion: CurrentClassificationVersion,
		RawMetadata:           rawMetadata,
	}, nil
}

// ReconcileTotal reports the (possibly nonzero) drift between the
// independently-derived net_sales + shipping + tax and Shopify's own
// authoritative currentTotalPriceSet. Small drift can legitimately occur
// (e.g. rounding on multi-line discount allocation) — this is informational
// for data-quality monitoring, not a hard validation error blocking
// normalization.
func ReconcileTotal(attrs event.OrderAttributes) money.Amount {
	derived := attrs.NetSales.Add(attrs.Shipping).Add(attrs.Tax)
	return derived.Sub(attrs.TotalSales)
}
