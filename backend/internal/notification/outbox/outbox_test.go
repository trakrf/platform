package outbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
)

type fakeEntitlement struct {
	entitled bool
	err      error
}

func (f fakeEntitlement) OrgIsEntitled(ctx context.Context, orgID int) (bool, error) {
	return f.entitled, f.err
}

func TestEnqueue_UnentitledOrgIsSkippedNotBuffered(t *testing.T) {
	cmd := outbox.Command{
		DeliveryID: "evt-1:contact-1:email",
		OrgID:      42,
		Channel:    notificationdelivery.ChannelEmail,
	}

	skipped, err := outbox.CheckEntitlement(context.Background(), fakeEntitlement{entitled: false}, cmd.OrgID)
	require.NoError(t, err)
	require.True(t, skipped, "unentitled org must be skipped, never buffered for later replay")
}

func TestEnqueue_EntitlementCheckErrorIsTransientNotSkip(t *testing.T) {
	_, err := outbox.CheckEntitlement(context.Background(), fakeEntitlement{err: errors.New("db unreachable")}, 42)
	require.Error(t, err, "an entitlement lookup failure must propagate as an error, never be treated as unentitled")
}
