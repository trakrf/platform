package outbox_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
)

func TestProviderError_ErrorMessageNeverLeaksPayload(t *testing.T) {
	err := &outbox.ProviderError{Kind: outbox.ErrorTransient}
	require.Equal(t, "notification channel adapter transient failure", err.Error())
}

func TestCommand_ChannelMatchesModelEnum(t *testing.T) {
	cmd := outbox.Command{Channel: notificationdelivery.ChannelEmail}
	require.Equal(t, notificationdelivery.ChannelEmail, cmd.Channel)
}
