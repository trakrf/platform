//go:build integration

package storage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/locationmap"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// insertMapLocation creates a live location (optionally under parent) and
// returns its id.
func insertMapLocation(t *testing.T, pool *pgxpool.Pool, orgID int, key string, parent *int) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, parent_location_id)
		VALUES ($1, $2, $2, $3) RETURNING id`, orgID, key, parent).Scan(&id))
	return id
}

var pngBytes = []byte("\x89PNG\r\n\x1a\nfake-png-body")

func TestLocationMap_ImageRoundTripAndReplaceKeepsPins(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	site := insertMapLocation(t, db.AdminPool, orgID, "SITE", nil)
	room := insertMapLocation(t, db.AdminPool, orgID, "ROOM-A", &site)

	m, err := db.Store.GetLocationMap(ctx, orgID, site)
	require.NoError(t, err)
	require.Nil(t, m, "no map yet")

	require.NoError(t, db.Store.UpsertLocationMapImage(ctx, orgID, site, "image/png", pngBytes))
	require.NoError(t, db.Store.ReplaceLocationMapPins(ctx, orgID, site, []locationmap.Pin{{LocationID: room, XPct: 12.5, YPct: 80}}))

	m, err = db.Store.GetLocationMap(ctx, orgID, site)
	require.NoError(t, err)
	require.NotNil(t, m)
	sum := sha256.Sum256(pngBytes)
	require.Equal(t, hex.EncodeToString(sum[:]), m.SHA256)
	require.Equal(t, len(pngBytes), m.ByteSize)
	require.Equal(t, "image/png", m.ContentType)
	require.Equal(t, "SITE", m.LocationName)
	require.Equal(t, []locationmap.Pin{{LocationID: room, XPct: 12.5, YPct: 80}}, m.Pins)

	// Replacing the image keeps the pins.
	jpeg := []byte("\xff\xd8\xff\xe0fake-jpeg")
	require.NoError(t, db.Store.UpsertLocationMapImage(ctx, orgID, site, "image/jpeg", jpeg))
	ct, img, _, err := db.Store.GetLocationMapImage(ctx, orgID, site)
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", ct)
	require.Equal(t, jpeg, img)
	m, err = db.Store.GetLocationMap(ctx, orgID, site)
	require.NoError(t, err)
	require.Len(t, m.Pins, 1)

	list, err := db.Store.ListLocationMaps(ctx, orgID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, 1, list[0].PinCount)
}

func TestLocationMap_SoftDeletedPinHiddenAndDeleteCascades(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	site := insertMapLocation(t, db.AdminPool, orgID, "SITE", nil)
	a := insertMapLocation(t, db.AdminPool, orgID, "ROOM-A", &site)
	b := insertMapLocation(t, db.AdminPool, orgID, "ROOM-B", &site)

	require.NoError(t, db.Store.UpsertLocationMapImage(ctx, orgID, site, "image/png", pngBytes))
	require.NoError(t, db.Store.ReplaceLocationMapPins(ctx, orgID, site, []locationmap.Pin{
		{LocationID: a, XPct: 10, YPct: 10}, {LocationID: b, XPct: 90, YPct: 90},
	}))
	_, err := db.AdminPool.Exec(ctx, `UPDATE trakrf.locations SET deleted_at = now() WHERE id = $1`, b)
	require.NoError(t, err)

	m, err := db.Store.GetLocationMap(ctx, orgID, site)
	require.NoError(t, err)
	require.Equal(t, []locationmap.Pin{{LocationID: a, XPct: 10, YPct: 10}}, m.Pins)

	existed, err := db.Store.DeleteLocationMap(ctx, orgID, site)
	require.NoError(t, err)
	require.True(t, existed)
	var pins int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.location_map_pins WHERE map_location_id = $1`, site).Scan(&pins))
	require.Zero(t, pins, "pins cascade with the map")
	existed, err = db.Store.DeleteLocationMap(ctx, orgID, site)
	require.NoError(t, err)
	require.False(t, existed)
}

func TestLocationMap_OtherOrgSeesNothing(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	var orgB int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.organizations (name, identifier) VALUES ('Other Org', 'other-org-maps') RETURNING id`).Scan(&orgB))
	site := insertMapLocation(t, db.AdminPool, orgA, "SITE", nil)
	require.NoError(t, db.Store.UpsertLocationMapImage(ctx, orgA, site, "image/png", pngBytes))

	m, err := db.Store.GetLocationMap(ctx, orgB, site)
	require.NoError(t, err)
	require.Nil(t, m)
	_, img, _, err := db.Store.GetLocationMapImage(ctx, orgB, site)
	require.NoError(t, err)
	require.Nil(t, img)
	list, err := db.Store.ListLocationMaps(ctx, orgB)
	require.NoError(t, err)
	require.Empty(t, list)
}
