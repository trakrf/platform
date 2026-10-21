package routing

import (
	"context"
	"fmt"
	"strings"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/notification/phone"
)

// SuppressionLookup reports whether an address has opted out on a channel.
// It is handed the normalized address.
type SuppressionLookup func(ctx context.Context, ch notificationdelivery.Channel, address string) (bool, error)

// candidate is one subscriber's verdict. An empty Outcome means eligible;
// Address is then the normalized address to send to.
type candidate struct {
	Subscriber notificationrecipient.Subscriber
	Channel    notificationdelivery.Channel
	Address    string
	Outcome    Outcome
}

// filterEligible returns one candidate per subscriber, in input order. A
// subscriber is judged on its own channel only, so a bad phone number never
// blocks the same person's email. The suppression lookup runs only for
// subscribers that survive every other rule; if it fails, nothing is sent.
func filterEligible(ctx context.Context, subs []notificationrecipient.Subscriber, region string, suppressed SuppressionLookup) ([]candidate, error) {
	out := make([]candidate, 0, len(subs))
	for _, s := range subs {
		c := candidate{Subscriber: s, Channel: notificationdelivery.Channel(s.Channel)}
		switch {
		case s.RecipientDeletedAt != nil || !s.RecipientActive:
			c.Outcome = OutcomeSkippedInactive
		default:
			c.Address = normalizeAddress(c.Channel, s, region)
			if c.Address == "" {
				c.Outcome = OutcomeSkippedNoAddress
				break
			}
			opted, err := suppressed(ctx, c.Channel, c.Address)
			if err != nil {
				return nil, fmt.Errorf("routing: suppression lookup for subscription %d: %w", s.SubscriptionID, err)
			}
			if opted {
				c.Outcome = OutcomeSkippedOptedOut
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// normalizeAddress returns the address for the subscriber's channel, or ""
// when the channel is unknown or the address is missing or unusable.
func normalizeAddress(ch notificationdelivery.Channel, s notificationrecipient.Subscriber, region string) string {
	switch ch {
	case notificationdelivery.ChannelEmail:
		if s.Email == nil {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(*s.Email))
	case notificationdelivery.ChannelSMS:
		if s.Phone == nil || strings.TrimSpace(*s.Phone) == "" {
			return ""
		}
		e164, err := phone.NormalizeE164(*s.Phone, region)
		if err != nil {
			return ""
		}
		return e164
	}
	return ""
}
