package routing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
)

func sp(s string) *string { return &s }

func noneSuppressed(context.Context, notificationdelivery.Channel, string) (bool, error) {
	return false, nil
}

func sub(channel string, email, phone *string) notificationrecipient.Subscriber {
	return notificationrecipient.Subscriber{
		Channel: channel, Email: email, Phone: phone, RecipientActive: true,
	}
}

func TestFilterEligible_Rules(t *testing.T) {
	now := time.Now()
	deleted := sub("email", sp("a@acme.test"), nil)
	deleted.RecipientDeletedAt = &now
	paused := sub("email", sp("a@acme.test"), nil)
	paused.RecipientActive = false

	tests := []struct {
		name       string
		sub        notificationrecipient.Subscriber
		suppressed bool
		want       Outcome
	}{
		{"deleted", deleted, false, OutcomeSkippedInactive},
		{"paused", paused, false, OutcomeSkippedInactive},
		{"email nil", sub("email", nil, nil), false, OutcomeSkippedNoAddress},
		{"email blank", sub("email", sp("   "), nil), false, OutcomeSkippedNoAddress},
		{"fax", sub("fax", sp("a@acme.test"), sp("+15551234567")), false, OutcomeSkippedNoAddress},
		{"sms unparseable", sub("sms", nil, sp("call me")), false, OutcomeSkippedNoAddress},
		{"sms nil", sub("sms", nil, nil), false, OutcomeSkippedNoAddress},
		{"suppressed", sub("email", sp("a@acme.test"), nil), true, OutcomeSkippedOptedOut},
		{"plain", sub("email", sp("a@acme.test"), nil), false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterEligible(context.Background(), []notificationrecipient.Subscriber{tt.sub}, "US",
				func(context.Context, notificationdelivery.Channel, string) (bool, error) {
					return tt.suppressed, nil
				})
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, tt.want, got[0].Outcome)
		})
	}
}

func TestFilterEligible_SuppressionSeesNormalizedPhone(t *testing.T) {
	var seen string
	got, err := filterEligible(context.Background(),
		[]notificationrecipient.Subscriber{sub("sms", nil, sp("(555) 123-4567"))}, "US",
		func(_ context.Context, ch notificationdelivery.Channel, addr string) (bool, error) {
			assert.Equal(t, notificationdelivery.ChannelSMS, ch)
			seen = addr
			return false, nil
		})
	require.NoError(t, err)
	assert.Equal(t, "+15551234567", seen)
	assert.Equal(t, "+15551234567", got[0].Address)
}

func TestFilterEligible_EmailNormalizedBeforeSuppression(t *testing.T) {
	var seen string
	got, err := filterEligible(context.Background(),
		[]notificationrecipient.Subscriber{sub("email", sp("  Ops@Acme.TEST "), nil)}, "US",
		func(_ context.Context, _ notificationdelivery.Channel, addr string) (bool, error) {
			seen = addr
			return false, nil
		})
	require.NoError(t, err)
	assert.Equal(t, "ops@acme.test", seen)
	assert.Equal(t, "ops@acme.test", got[0].Address)
}

func TestFilterEligible_BadPhoneDoesNotBlockEmail(t *testing.T) {
	subs := []notificationrecipient.Subscriber{
		sub("sms", sp("a@acme.test"), sp("call me")),
		sub("email", sp("a@acme.test"), sp("call me")),
	}
	got, err := filterEligible(context.Background(), subs, "US", noneSuppressed)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, OutcomeSkippedNoAddress, got[0].Outcome)
	assert.Equal(t, Outcome(""), got[1].Outcome)
}

func TestFilterEligible_SuppressionNotCalledForSkipped(t *testing.T) {
	s := sub("email", sp("a@acme.test"), nil)
	s.RecipientActive = false
	calls := 0
	_, err := filterEligible(context.Background(), []notificationrecipient.Subscriber{s}, "US",
		func(context.Context, notificationdelivery.Channel, string) (bool, error) {
			calls++
			return false, nil
		})
	require.NoError(t, err)
	assert.Equal(t, 0, calls)
}

func TestFilterEligible_LookupErrorFails(t *testing.T) {
	_, err := filterEligible(context.Background(),
		[]notificationrecipient.Subscriber{sub("email", sp("secret@acme.test"), nil)}, "US",
		func(context.Context, notificationdelivery.Channel, string) (bool, error) {
			return false, errors.New("db down")
		})
	require.Error(t, err)
	assert.False(t, strings.Contains(err.Error(), "secret@acme.test"))
}
