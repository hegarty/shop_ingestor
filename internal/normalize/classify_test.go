package normalize

import (
	"testing"

	"github.com/hegarty/shop_platform/event"
)

func TestClassifyChannel(t *testing.T) {
	cases := []struct {
		name        string
		tags        []string
		wantChannel event.Channel
		wantPartner *string
	}{
		{
			name:        "no tags at all",
			tags:        nil,
			wantChannel: event.ChannelShop,
		},
		{
			name:        "ordinary shop tags, no collective",
			tags:        []string{"vip", "wholesale-inquiry"},
			wantChannel: event.ChannelShop,
		},
		{
			name:        "collective order with clean partner tag",
			tags:        []string{"Shopify Collective", "ABC Bikes"},
			wantChannel: event.ChannelCollective,
			wantPartner: strPtr("ABC Bikes"),
		},
		{
			name:        "collective order, partner tag first",
			tags:        []string{"E-Moto Co", "Shopify Collective"},
			wantChannel: event.ChannelCollective,
			wantPartner: strPtr("E-Moto Co"),
		},
		{
			name:        "collective order with internal tag ignored",
			tags:        []string{"Shopify Collective", "ABC Bikes", "_internal_marker"},
			wantChannel: event.ChannelCollective,
			wantPartner: strPtr("ABC Bikes"),
		},
		{
			name:        "collective order, no partner tag left — ambiguous",
			tags:        []string{"Shopify Collective"},
			wantChannel: event.ChannelCollective,
			wantPartner: nil,
		},
		{
			name:        "collective order, multiple candidate tags — ambiguous",
			tags:        []string{"Shopify Collective", "ABC Bikes", "vip"},
			wantChannel: event.ChannelCollective,
			wantPartner: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotChannel, gotPartner := ClassifyChannel(c.tags)
			if gotChannel != c.wantChannel {
				t.Errorf("channel = %q, want %q", gotChannel, c.wantChannel)
			}
			switch {
			case c.wantPartner == nil && gotPartner != nil:
				t.Errorf("partner = %q, want nil", *gotPartner)
			case c.wantPartner != nil && gotPartner == nil:
				t.Errorf("partner = nil, want %q", *c.wantPartner)
			case c.wantPartner != nil && gotPartner != nil && *gotPartner != *c.wantPartner:
				t.Errorf("partner = %q, want %q", *gotPartner, *c.wantPartner)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
