//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// A reader move names the scan point that saw it; a handheld save or a
// same-minute override has none. All three origins are recorded.
func TestLocationChange_ScanPointMatchesOrigin(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "lc-origin")

	cases := []struct {
		name      string
		origin    string
		scanPoint *int
		sqlState  string // "" means the insert succeeds
	}{
		{"reader with its scan point", "reader", &f.scanPointID, ""},
		{"reader without a scan point", "reader", nil, sqlStateCheck},
		{"manual_save without a scan point", "manual_save", nil, ""},
		{"manual_save with a scan point", "manual_save", &f.scanPointID, sqlStateCheck},
		{"manual_override without a scan point", "manual_override", nil, ""},
		{"manual_override with a scan point", "manual_override", &f.scanPointID, sqlStateCheck},
		{"simulated is not an origin", "simulated", &f.scanPointID, sqlStateCheck},
		{"unknown origin", "gps", nil, sqlStateCheck},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Distinct instants so the replay guard never interferes.
			at := testInstant.Add(time.Duration(i) * time.Minute)
			_, err := f.insertLocationChange(nil, &f.locationID, tc.origin, tc.scanPoint, at)
			if tc.sqlState == "" {
				require.NoError(t, err, tc.name)
				return
			}
			requireSQLState(t, err, tc.sqlState, tc.name)
		})
	}
}

func TestLocationChange_OriginAndDestinationAreRequired(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "lc-required")

	_, err := f.insertLocationChange(nil, nil, "manual_save", nil, testInstant)
	requireSQLState(t, err, sqlStateNotNull, "a move with no destination")

	_, err = db.AdminPool.Exec(context.Background(), `
		INSERT INTO trakrf.location_changes (org_id, asset_id, to_location_id, detected_at)
		VALUES ($1, $2, $3, $4)`, orgID, f.assetID, f.locationID, testInstant)
	requireSQLState(t, err, sqlStateNotNull, "a move with no origin")
}

// An asset's first-ever sighting, or one whose last scan had no location, has
// no origin location; it is still a move.
func TestLocationChange_FromLocationIsOptional(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "lc-from")
	var yard int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'yard', 'Yard') RETURNING id`,
		orgID,
	).Scan(&yard))

	first, err := f.insertLocationChange(nil, &yard, "reader", &f.scanPointID, testInstant)
	require.NoError(t, err, "first sighting")
	second, err := f.insertLocationChange(&yard, &f.locationID, "manual_override", nil, testInstant.Add(time.Minute))
	require.NoError(t, err, "a move with an origin")

	var from *int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT from_location_id FROM trakrf.location_changes WHERE id = $1`, first).Scan(&from))
	require.Nil(t, from, "first sighting keeps a NULL origin")
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT from_location_id FROM trakrf.location_changes WHERE id = $1`, second).Scan(&from))
	require.NotNil(t, from)
	require.Equal(t, yard, *from)
}

// The same move recorded twice is rejected; a different asset or instant is a
// different move.
func TestLocationChange_ReplayIsRejected(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "lc-replay")
	g := f
	g.assetID = testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-LC-REPLAY-2").ID

	_, err := f.insertLocationChange(nil, &f.locationID, "reader", &f.scanPointID, testInstant)
	require.NoError(t, err)

	_, err = f.insertLocationChange(nil, &f.locationID, "reader", &f.scanPointID, testInstant)
	requireSQLState(t, err, sqlStateUnique, "same asset at the same instant")
	_, err = f.insertLocationChange(nil, &f.locationID, "manual_save", nil, testInstant)
	requireSQLState(t, err, sqlStateUnique, "same asset and instant from another origin")

	_, err = f.insertLocationChange(nil, &f.locationID, "reader", &f.scanPointID, testInstant.Add(time.Microsecond))
	require.NoError(t, err, "same asset, a different instant")
	_, err = g.insertLocationChange(nil, &g.locationID, "reader", &g.scanPointID, testInstant)
	require.NoError(t, err, "another asset at the same instant")
}

func TestLocationChange_ReferencesMustExist(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "lc-fk")
	missing := 2147483000

	_, err := f.insertLocationChange(nil, &missing, "manual_save", nil, testInstant)
	requireSQLState(t, err, sqlStateForeignKey, "unknown destination location")
	_, err = f.insertLocationChange(&missing, &f.locationID, "manual_save", nil, testInstant)
	requireSQLState(t, err, sqlStateForeignKey, "unknown origin location")
	_, err = f.insertLocationChange(nil, &f.locationID, "reader", &missing, testInstant)
	requireSQLState(t, err, sqlStateForeignKey, "unknown scan point")
	g := f
	g.assetID = missing
	_, err = g.insertLocationChange(nil, &f.locationID, "manual_save", nil, testInstant)
	requireSQLState(t, err, sqlStateForeignKey, "unknown asset")
}

// Ids come from the shared obfuscated-id sequence (TRA-886), not a per-table
// IDENTITY, and no location_changes sequence exists.
func TestLocationChange_IDsAreObfuscated(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "lc-id")

	id, err := f.insertLocationChange(nil, &f.locationID, "manual_save", nil, testInstant)
	require.NoError(t, err)
	require.Greater(t, id, 0)

	var triggers, seqs int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		SELECT count(*) FROM pg_trigger t JOIN pg_proc p ON p.oid = t.tgfoid
		WHERE t.tgrelid = 'trakrf.location_changes'::regclass AND p.proname = 'generate_obfuscated_id'`,
	).Scan(&triggers))
	require.Equal(t, 1, triggers, "one generate_obfuscated_id trigger")
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM pg_sequences WHERE schemaname = 'trakrf' AND sequencename LIKE 'location_changes%'`,
	).Scan(&seqs))
	require.Zero(t, seqs, "no per-table sequence")
}

// Row-level security: an org sees its own moves and never another org's.
func TestLocationChange_IsInvisibleToAnotherOrg(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	otherOrgID := createOrg(t, db.AdminPool, "Location Change Org B", "location-change-org-b")
	f := newEventFixture(t, db, orgID, "lc-rls")
	_, err := f.insertLocationChange(nil, &f.locationID, "manual_save", nil, testInstant)
	require.NoError(t, err)

	count := func(org int) int {
		var n int
		require.NoError(t, db.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM trakrf.location_changes`).Scan(&n)
		}))
		return n
	}
	require.Equal(t, 1, count(orgID), "own org sees its move")
	require.Zero(t, count(otherOrgID), "another org sees nothing")
}
