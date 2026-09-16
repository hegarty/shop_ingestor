// Package normalize converts a Shopify order payload into shop_platform's
// canonical CommerceEvent shape, including classifying the order as a
// direct shop sale or a Shopify Collective sale (and, for Collective
// orders, identifying the partner).
package normalize

import (
	"strings"

	"github.com/hegarty/shop_platform/event"
)

// CurrentClassificationVersion is stamped onto every classified order so a
// future improvement to this heuristic can identify which historical rows
// need reprocessing. Bump this whenever the classification logic changes.
const CurrentClassificationVersion = 1

// collectiveTag is the literal tag Shopify Collective applies to every
// order it creates — confirmed against Shopify's current developer docs
// (https://shopify.dev/docs/apps/build/collective/orders), not assumed.
// See shop_docs/docs/event-model.md for the citations.
const collectiveTag = "Shopify Collective"

// ClassifyChannel determines an order's channel and (for Collective orders)
// its partner name from the order's tags.
//
// Classification rules:
//  1. No "Shopify Collective" tag -> ChannelShop, no partner.
//  2. "Shopify Collective" tag present, exactly one other non-internal tag
//     remains -> ChannelCollective, that tag is the partner name.
//  3. "Shopify Collective" tag present, but zero or more than one candidate
//     partner tag remains -> ChannelCollective with no partner. This is a
//     real, expected case (e.g. the merchant's own manual tags collide with
//     the heuristic) — modeled explicitly rather than guessed at, so it
//     shows up as its own queryable bucket instead of misattributing or
//     dropping the sale.
func ClassifyChannel(tags []string) (channel event.Channel, partnerName *string) {
	hasCollectiveTag := false
	var candidates []string

	for _, tag := range tags {
		switch {
		case tag == collectiveTag:
			hasCollectiveTag = true
		case strings.HasPrefix(tag, "_"):
			// Shopify-internal tags (app-generated, underscore-prefixed by
			// convention) are never a partner name.
		default:
			candidates = append(candidates, tag)
		}
	}

	if !hasCollectiveTag {
		return event.ChannelShop, nil
	}

	if len(candidates) == 1 {
		return event.ChannelCollective, &candidates[0]
	}
	return event.ChannelCollective, nil
}
