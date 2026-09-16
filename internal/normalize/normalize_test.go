package normalize

import (
	"testing"

	"github.com/hegarty/shop_platform/event"

	"github.com/hegarty/shop_ingestor/internal/shopify"
)

func moneyBag(amount string) shopify.MoneyBag {
	return shopify.MoneyBag{ShopMoney: shopify.Money{Amount: amount, CurrencyCode: "USD"}}
}

func TestFromShopifyOrder_DirectSale(t *testing.T) {
	o := shopify.Order{
		ID:                       "gid://shopify/Order/1",
		Name:                     "#1001",
		CreatedAt:                "2026-09-11T15:31:12Z",
		UpdatedAt:                "2026-09-11T15:31:12Z",
		Tags:                     []string{"vip"},
		DisplayFinancialStatus:   "PAID",
		DisplayFulfillmentStatus: "UNFULFILLED",
		CurrentShippingPriceSet:  moneyBag("10.00"),
		CurrentTotalTaxSet:       moneyBag("5.00"),
		CurrentTotalPriceSet:     moneyBag("115.00"),
		TotalRefundedSet:         moneyBag("0.00"),
		TotalRefundedShippingSet: moneyBag("0.00"),
		LineItems: shopify.LineItemConnection{
			Edges: []struct {
				Node shopify.LineItem `json:"node"`
			}{
				{Node: shopify.LineItem{
					ID:                 "gid://shopify/LineItem/1",
					Title:              "Widget",
					Quantity:           1,
					OriginalTotalSet:   moneyBag("100.00"),
					DiscountedTotalSet: moneyBag("100.00"),
				}},
			},
		},
	}

	attrs, err := FromShopifyOrder(o)
	if err != nil {
		t.Fatalf("FromShopifyOrder: %v", err)
	}

	if attrs.Channel != event.ChannelShop {
		t.Errorf("Channel = %q, want shop", attrs.Channel)
	}
	if attrs.GrossSales.DecimalString() != "100.00" {
		t.Errorf("GrossSales = %s, want 100.00", attrs.GrossSales.DecimalString())
	}
	if attrs.NetSales.DecimalString() != "100.00" {
		t.Errorf("NetSales = %s, want 100.00", attrs.NetSales.DecimalString())
	}
	if attrs.TotalSales.DecimalString() != "115.00" {
		t.Errorf("TotalSales = %s, want 115.00", attrs.TotalSales.DecimalString())
	}
	if drift := ReconcileTotal(attrs); drift != 0 {
		t.Errorf("ReconcileTotal drift = %d, want 0", drift)
	}
}

func TestFromShopifyOrder_CollectiveSaleWithDiscountAndPartialRefund(t *testing.T) {
	o := shopify.Order{
		ID:                       "gid://shopify/Order/2",
		Name:                     "#1002",
		CreatedAt:                "2026-09-11T15:31:12Z",
		UpdatedAt:                "2026-09-12T09:00:00Z",
		Tags:                     []string{"Shopify Collective", "ABC Bikes"},
		DisplayFinancialStatus:   "PARTIALLY_REFUNDED",
		DisplayFulfillmentStatus: "FULFILLED",
		CurrentShippingPriceSet:  moneyBag("0.00"),
		CurrentTotalTaxSet:       moneyBag("0.00"),
		CurrentTotalPriceSet:     moneyBag("599.00"),
		TotalRefundedSet:         moneyBag("100.00"),
		TotalRefundedShippingSet: moneyBag("0.00"),
		CartDiscountAmountSet:    &shopify.MoneyBag{ShopMoney: shopify.Money{Amount: "0.00", CurrencyCode: "USD"}},
		LineItems: shopify.LineItemConnection{
			Edges: []struct {
				Node shopify.LineItem `json:"node"`
			}{
				{Node: shopify.LineItem{
					ID:                 "gid://shopify/LineItem/2",
					Title:              "E-Bike",
					Quantity:           1,
					OriginalTotalSet:   moneyBag("799.00"),
					DiscountedTotalSet: moneyBag("699.00"), // $100 line-level discount
				}},
			},
		},
	}

	attrs, err := FromShopifyOrder(o)
	if err != nil {
		t.Fatalf("FromShopifyOrder: %v", err)
	}

	if attrs.Channel != event.ChannelCollective {
		t.Errorf("Channel = %q, want collective", attrs.Channel)
	}
	if attrs.CollectivePartnerName == nil || *attrs.CollectivePartnerName != "ABC Bikes" {
		t.Errorf("CollectivePartnerName = %v, want ABC Bikes", attrs.CollectivePartnerName)
	}
	if attrs.GrossSales.DecimalString() != "799.00" {
		t.Errorf("GrossSales = %s, want 799.00", attrs.GrossSales.DecimalString())
	}
	if attrs.Discounts.DecimalString() != "100.00" {
		t.Errorf("Discounts = %s, want 100.00", attrs.Discounts.DecimalString())
	}
	if attrs.Returns.DecimalString() != "100.00" {
		t.Errorf("Returns = %s, want 100.00", attrs.Returns.DecimalString())
	}
	// 799 gross - 100 discount - 100 returned = 599 net
	if attrs.NetSales.DecimalString() != "599.00" {
		t.Errorf("NetSales = %s, want 599.00", attrs.NetSales.DecimalString())
	}
	if drift := ReconcileTotal(attrs); drift != 0 {
		t.Errorf("ReconcileTotal drift = %d, want 0", drift)
	}
}

func TestFromShopifyOrder_InvalidTimestamp(t *testing.T) {
	o := shopify.Order{CreatedAt: "not-a-timestamp"}
	if _, err := FromShopifyOrder(o); err == nil {
		t.Fatal("expected error for invalid createdAt")
	}
}
