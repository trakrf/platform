//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func intp(v int) *int       { return &v }
func strp(v string) *string { return &v }

func TestOrgGeofenceDefaults_RoundTrip(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// A fresh org has no overrides — all nil.
	d, err := db.Store.GetOrgGeofenceDefaults(ctx, orgID)
	require.NoError(t, err)
	require.Nil(t, d.RSSIThreshold)
	require.Nil(t, d.AgeOutSeconds)
	require.Nil(t, d.AutoOffSeconds)
	require.Nil(t, d.Mode)

	// Write a partial set (rssi, age_out, mode; leave auto_off unset).
	require.NoError(t, db.Store.UpdateOrgGeofenceDefaults(ctx, orgID, organization.GeofenceDefaults{
		RSSIThreshold: intp(-50), AgeOutSeconds: intp(25), Mode: strp("presence"),
	}))

	d, err = db.Store.GetOrgGeofenceDefaults(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, d.RSSIThreshold)
	require.Equal(t, -50, *d.RSSIThreshold)
	require.NotNil(t, d.AgeOutSeconds)
	require.Equal(t, 25, *d.AgeOutSeconds)
	require.NotNil(t, d.Mode)
	require.Equal(t, "presence", *d.Mode)
	require.Nil(t, d.AutoOffSeconds, "auto_off was not set, must stay unset")

	// Full-replace: a new write with only auto_off clears the previously-set keys.
	require.NoError(t, db.Store.UpdateOrgGeofenceDefaults(ctx, orgID, organization.GeofenceDefaults{
		AutoOffSeconds: intp(9),
	}))
	d, err = db.Store.GetOrgGeofenceDefaults(ctx, orgID)
	require.NoError(t, err)
	require.Nil(t, d.RSSIThreshold, "replace must clear rssi")
	require.Nil(t, d.AgeOutSeconds, "replace must clear age_out")
	require.Nil(t, d.Mode, "replace must clear mode")
	require.NotNil(t, d.AutoOffSeconds)
	require.Equal(t, 9, *d.AutoOffSeconds)
}

func TestOrgGeofenceDefaults_PreservesOtherMetadata(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// Seed an unrelated metadata key directly.
	_, err := db.AdminPool.Exec(ctx,
		`UPDATE trakrf.organizations SET metadata = jsonb_set(COALESCE(metadata,'{}'::jsonb), '{unrelated}', '"keep-me"', true) WHERE id = $1`,
		orgID)
	require.NoError(t, err)

	require.NoError(t, db.Store.UpdateOrgGeofenceDefaults(ctx, orgID, organization.GeofenceDefaults{
		RSSIThreshold: intp(-60),
	}))

	org, err := db.Store.GetOrganizationByID(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, org)
	require.Equal(t, "keep-me", org.Metadata["unrelated"], "geofence write must not clobber other metadata keys")
}
