package routing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

func TestDeliveryID_Golden(t *testing.T) {
	email := deliveryID(1, 2, notificationdelivery.ChannelEmail)
	assert.Equal(t, "2a87af1bda6a4d0ae39fa53988c18d439ec3f995b04b02ba716c1dea92d18e91", email)
	assert.NotEqual(t, email, deliveryID(1, 2, notificationdelivery.ChannelSMS))
}

func keysOf(t *testing.T, m DeliveryMessage) []string {
	t.Helper()
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	return keys
}

func TestDeliveryMessage_JSONKeys(t *testing.T) {
	email := DeliveryMessage{
		EventID: 1, Source: notificationevent.SourceAlarm, Channel: notificationdelivery.ChannelEmail,
		RecipientID: 2, To: "a@acme.test", Subject: "s", Body: "b", AssetID: 3, OccurredAt: time.Now(),
	}
	assert.ElementsMatch(t,
		[]string{"event_id", "source", "channel", "recipient_id", "to", "subject", "body", "asset_id", "occurred_at"},
		keysOf(t, email))

	sms := email
	sms.Channel = notificationdelivery.ChannelSMS
	sms.Subject = ""
	assert.NotContains(t, keysOf(t, sms), "subject")
}
