//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// seedSuppression inserts an opt-out row directly; there is no write API yet.
func seedSuppression(t *testing.T, db *testutil.TestDB, orgID int, ch notificationdelivery.Channel, address string, cleared bool) {
	t.Helper()
	clearedAt := "NULL"
	if cleared {
		clearedAt = "now()"
	}
	_, err := db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.notification_suppressions (org_id, channel, address, reason, cleared_at)
		 VALUES ($1, $2, $3, 'manual', `+clearedAt+`)`,
		orgID, string(ch), address)
	require.NoError(t, err)
}

func TestIsSuppressed(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	var otherOrg int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Other Co', 'other-co', true) RETURNING id`,
	).Scan(&otherOrg))

	seedSuppression(t, db, orgID, notificationdelivery.ChannelEmail, "Foo@X.test", false)
	seedSuppression(t, db, orgID, notificationdelivery.ChannelEmail, "gone@x.test", true)
	seedSuppression(t, db, otherOrg, notificationdelivery.ChannelEmail, "elsewhere@x.test", false)

	cases := []struct {
		name    string
		ch      notificationdelivery.Channel
		address string
		want    bool
	}{
		{"uncleared row", notificationdelivery.ChannelEmail, "Foo@X.test", true},
		{"different case", notificationdelivery.ChannelEmail, "foo@x.test", true},
		{"cleared row", notificationdelivery.ChannelEmail, "gone@x.test", false},
		{"same address, other channel", notificationdelivery.ChannelSMS, "Foo@X.test", false},
		{"same address, other org", notificationdelivery.ChannelEmail, "elsewhere@x.test", false},
		{"no row", notificationdelivery.ChannelEmail, "nobody@x.test", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := db.Store.IsSuppressed(ctx, orgID, tc.ch, tc.address)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	t.Run("other org sees its own row", func(t *testing.T) {
		got, err := db.Store.IsSuppressed(ctx, otherOrg, notificationdelivery.ChannelEmail, "elsewhere@x.test")
		require.NoError(t, err)
		require.True(t, got)
	})
}

// A failed lookup must surface as an error, never as "not suppressed", and
// must not carry the address in its text.
func TestIsSuppressed_LookupErrorIsReturned(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := db.Store.IsSuppressed(ctx, orgID, notificationdelivery.ChannelEmail, "secret@x.test")
	require.Error(t, err)
	require.False(t, got)
	require.NotContains(t, err.Error(), "secret@x.test")
	require.ErrorIs(t, err, context.Canceled)
}
