package notificationalert

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

func TestFormatID_ReadsAsSentTimeSourceAndPublicID(t *testing.T) {
	sent := time.Date(2026, 11, 16, 19, 44, 22, 999_000_000, time.FixedZone("EST", -5*60*60))
	require.Equal(t, "alert-20261117T004422Z-location_change-2814749767106561",
		FormatID(sent, notificationevent.SourceLocationChange, 2814749767106561),
		"the time is UTC and truncated to the second")
}
