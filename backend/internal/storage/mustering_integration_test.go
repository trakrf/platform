//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/muster"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// createPersonAsset inserts an asset with metadata.person=true and returns its id.
func createPersonAsset(t *testing.T, db *testutil.TestDB, orgID int, name string) int {
	t.Helper()
	ctx := context.Background()
	var id int
	err := db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.assets (org_id, external_key, name, valid_from, is_active, metadata)
		VALUES ($1, $2, $3, now(), true, '{"person":true}')
		RETURNING id`,
		orgID, "person-"+name, name,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// createRegularAsset inserts a non-person asset and returns its id.
func createRegularAsset(t *testing.T, db *testutil.TestDB, orgID int, name string) int {
	t.Helper()
	ctx := context.Background()
	var id int
	err := db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.assets (org_id, external_key, name, valid_from, is_active, metadata)
		VALUES ($1, $2, $3, now(), true, '{}')
		RETURNING id`,
		orgID, "asset-"+name, name,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// createZoneLocation inserts a plain zone location and returns its id.
func createZoneLocation(t *testing.T, db *testutil.TestDB, orgID int, name string) int {
	t.Helper()
	ctx := context.Background()
	var id int
	err := db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.locations (org_id, external_key, name, valid_from, is_active, metadata)
		VALUES ($1, $2, $3, now(), true, '{}')
		RETURNING id`,
		orgID, "zone-"+name, name,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// createMusterPointLocation inserts a muster-point location and returns its id.
func createMusterPointLocation(t *testing.T, db *testutil.TestDB, orgID int, name string) int {
	t.Helper()
	ctx := context.Background()
	var id int
	err := db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.locations (org_id, external_key, name, valid_from, is_active, metadata)
		VALUES ($1, $2, $3, now(), true, '{"muster_point":true}')
		RETURNING id`,
		orgID, "muster-"+name, name,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// insertAssetScan inserts an asset_scans row at the given timestamp.
func insertAssetScan(t *testing.T, db *testutil.TestDB, orgID, assetID, locationID int, ts time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := db.AdminPool.Exec(ctx, `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING`,
		ts, orgID, assetID, locationID,
	)
	require.NoError(t, err)
}

// createMusterUser inserts a user with operator role and returns its id.
func createMusterUser(t *testing.T, db *testutil.TestDB, orgID int, email string) int {
	t.Helper()
	ctx := context.Background()
	var userID int
	err := db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $1, 'hash')
		RETURNING id`,
		email,
	).Scan(&userID)
	require.NoError(t, err)
	// Add user to org
	_, err = db.AdminPool.Exec(ctx, `
		INSERT INTO trakrf.org_users (org_id, user_id, role)
		VALUES ($1, $2, 'operator')`,
		orgID, userID,
	)
	require.NoError(t, err)
	return userID
}

// ── ListPersonPresence ────────────────────────────────────────────────────────

func TestMuster_ListPersonPresence_HonorsWindow(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	zoneID := createZoneLocation(t, db, orgID, "Production")
	personA := createPersonAsset(t, db, orgID, "Operator 001")
	personB := createPersonAsset(t, db, orgID, "Operator 002")

	now := time.Now()
	window := 15 * time.Minute

	// Person A: seen recently (within window)
	insertAssetScan(t, db, orgID, personA, zoneID, now.Add(-5*time.Minute))

	// Person B: seen too long ago (outside window)
	insertAssetScan(t, db, orgID, personB, zoneID, now.Add(-30*time.Minute))

	presence, err := db.Store.ListPersonPresence(ctx, orgID, window)
	require.NoError(t, err)

	// Only personA should appear (within window)
	require.Len(t, presence, 1)
	require.Equal(t, personA, presence[0].AssetID)
	require.NotNil(t, presence[0].LocationID)
	require.Equal(t, zoneID, *presence[0].LocationID)
}

func TestMuster_ListPersonPresence_ExcludesNonPersons(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	zoneID := createZoneLocation(t, db, orgID, "Zone1")
	personID := createPersonAsset(t, db, orgID, "Operator 003")
	assetID := createRegularAsset(t, db, orgID, "Forklift")

	now := time.Now()
	insertAssetScan(t, db, orgID, personID, zoneID, now.Add(-2*time.Minute))
	insertAssetScan(t, db, orgID, assetID, zoneID, now.Add(-2*time.Minute))

	presence, err := db.Store.ListPersonPresence(ctx, orgID, 15*time.Minute)
	require.NoError(t, err)

	require.Len(t, presence, 1)
	require.Equal(t, personID, presence[0].AssetID)
}

func TestMuster_ListPersonPresence_MostRecentSighting(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	zone1ID := createZoneLocation(t, db, orgID, "Zone1")
	zone2ID := createZoneLocation(t, db, orgID, "Zone2")
	personID := createPersonAsset(t, db, orgID, "Operator 004")

	now := time.Now()
	// Person seen in zone1 earlier, then zone2 more recently
	insertAssetScan(t, db, orgID, personID, zone1ID, now.Add(-10*time.Minute))
	insertAssetScan(t, db, orgID, personID, zone2ID, now.Add(-3*time.Minute))

	presence, err := db.Store.ListPersonPresence(ctx, orgID, 15*time.Minute)
	require.NoError(t, err)

	require.Len(t, presence, 1)
	require.Equal(t, personID, presence[0].AssetID)
	require.NotNil(t, presence[0].LocationID)
	// Should return the most-recent location (zone2)
	require.Equal(t, zone2ID, *presence[0].LocationID)
}

// ── ListZones ─────────────────────────────────────────────────────────────────

func TestMuster_ListZones(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	zoneID := createZoneLocation(t, db, orgID, "Warehouse")
	mpID := createMusterPointLocation(t, db, orgID, "Muster A")

	zones, err := db.Store.ListZones(ctx, orgID)
	require.NoError(t, err)
	require.Len(t, zones, 2)

	byID := map[int]muster.ZonePresence{}
	for _, z := range zones {
		byID[z.LocationID] = z
	}
	require.Contains(t, byID, zoneID)
	require.False(t, byID[zoneID].MusterPoint)
	require.Equal(t, 0, byID[zoneID].Count) // count filled by engine

	require.Contains(t, byID, mpID)
	require.True(t, byID[mpID].MusterPoint)
}

// ── ListMusterPointIDs ────────────────────────────────────────────────────────

func TestMuster_ListMusterPointIDs(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	createZoneLocation(t, db, orgID, "Zone")
	mpA := createMusterPointLocation(t, db, orgID, "Muster A")
	mpB := createMusterPointLocation(t, db, orgID, "Muster B")

	ids, err := db.Store.ListMusterPointIDs(ctx, orgID)
	require.NoError(t, err)
	require.ElementsMatch(t, []int{mpA, mpB}, ids)
}

// ── CreateMusterEvent ─────────────────────────────────────────────────────────

func TestMuster_CreateMusterEvent_Basic(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "operator@example.com")

	zoneID := createZoneLocation(t, db, orgID, "Zone")
	personID := createPersonAsset(t, db, orgID, "Operator 001")

	now := time.Now()
	insertAssetScan(t, db, orgID, personID, zoneID, now.Add(-5*time.Minute))

	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.NotZero(t, event.ID)
	require.Equal(t, "active", event.Status)
	require.Equal(t, 15, event.WindowMinutes)
	require.NotNil(t, event.StartedBy)
	require.Equal(t, userID, *event.StartedBy)

	// Should have one entry for the person
	require.Len(t, event.Entries, 1)
	require.Equal(t, personID, event.Entries[0].AssetID)
	require.Equal(t, "missing", event.Entries[0].Status)
	require.Equal(t, "Operator 001", event.Entries[0].Label)
	require.NotNil(t, event.Entries[0].ExpectedLocationID)
	require.Equal(t, zoneID, *event.Entries[0].ExpectedLocationID)

	// Counts
	require.Equal(t, 1, event.Counts.Expected)
	require.Equal(t, 1, event.Counts.Missing)
}

func TestMuster_CreateMusterEvent_OneActivePerOrg(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "operator2@example.com")

	// First event: should succeed
	_, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	// Second event on same org: must fail with ErrActiveEventExists
	_, err = db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.Error(t, err)
	var activeErr muster.ErrActiveEventExists
	require.ErrorAs(t, err, &activeErr, "must return ErrActiveEventExists for duplicate active event")
}

func TestMuster_CreateMusterEvent_ExcludesStalePersons(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "operator3@example.com")

	zoneID := createZoneLocation(t, db, orgID, "Zone")
	personFresh := createPersonAsset(t, db, orgID, "Fresh Person")
	personStale := createPersonAsset(t, db, orgID, "Stale Person")

	now := time.Now()
	// Fresh: within 15-min window
	insertAssetScan(t, db, orgID, personFresh, zoneID, now.Add(-5*time.Minute))
	// Stale: outside 15-min window
	insertAssetScan(t, db, orgID, personStale, zoneID, now.Add(-20*time.Minute))

	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	// Only the fresh person should be in the entries
	require.Len(t, event.Entries, 1)
	require.Equal(t, personFresh, event.Entries[0].AssetID)
}

func TestMuster_CreateMusterEvent_ZeroPersons(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "operator-zero@example.com")

	// No person-assets and no asset scans — presence snapshot is empty.
	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.NotZero(t, event.ID)
	require.Equal(t, "active", event.Status)
	require.Len(t, event.Entries, 0)
	require.Equal(t, 0, event.Counts.Expected)
}

// ── GetActiveMusterEvent ──────────────────────────────────────────────────────

func TestMuster_GetActiveMusterEvent(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op4@example.com")

	// No event yet: returns nil, nil
	event, err := db.Store.GetActiveMusterEvent(ctx, orgID)
	require.NoError(t, err)
	require.Nil(t, event)

	// Create an event
	personID := createPersonAsset(t, db, orgID, "Person 1")
	zoneID := createZoneLocation(t, db, orgID, "Zone")
	insertAssetScan(t, db, orgID, personID, zoneID, time.Now().Add(-2*time.Minute))
	created, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	// Now GetActiveMusterEvent should return it
	active, err := db.Store.GetActiveMusterEvent(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, created.ID, active.ID)
	require.Equal(t, "active", active.Status)
	require.Len(t, active.Entries, 1)
}

// ── MarkEntryAtMuster ─────────────────────────────────────────────────────────

func TestMuster_MarkEntryAtMuster_TransitionsMissingToAtMuster(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op5@example.com")

	zoneID := createZoneLocation(t, db, orgID, "Zone")
	mpID := createMusterPointLocation(t, db, orgID, "MP A")
	personID := createPersonAsset(t, db, orgID, "Person 1")

	insertAssetScan(t, db, orgID, personID, zoneID, time.Now().Add(-3*time.Minute))
	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	seenAt := time.Now()
	entry, err := db.Store.MarkEntryAtMuster(ctx, orgID, event.ID, personID, mpID, seenAt)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Equal(t, "at_muster", entry.Status)
	require.NotNil(t, entry.MusterLocationID)
	require.Equal(t, mpID, *entry.MusterLocationID)
	require.NotNil(t, entry.FirstMusterSeenAt)
}

func TestMuster_MarkEntryAtMuster_NoopForVerified(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op6@example.com")

	zoneID := createZoneLocation(t, db, orgID, "Zone")
	mpID := createMusterPointLocation(t, db, orgID, "MP A")
	personID := createPersonAsset(t, db, orgID, "Person 1")

	insertAssetScan(t, db, orgID, personID, zoneID, time.Now().Add(-3*time.Minute))
	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	// Mark at muster first
	_, err = db.Store.MarkEntryAtMuster(ctx, orgID, event.ID, personID, mpID, time.Now())
	require.NoError(t, err)

	// Then verify it
	entry, err := db.Store.UpdateEntryStatus(ctx, orgID, event.ID, event.Entries[0].ID, "verify", userID, "")
	require.NoError(t, err)
	require.Equal(t, "verified", entry.Status)

	// Try to mark at_muster again — should be a no-op (nil return)
	noopEntry, err := db.Store.MarkEntryAtMuster(ctx, orgID, event.ID, personID, mpID, time.Now())
	require.NoError(t, err)
	require.Nil(t, noopEntry, "verified entry must not be downgraded by MarkEntryAtMuster")
}

// ── UpdateEntryStatus ─────────────────────────────────────────────────────────

func TestMuster_UpdateEntryStatus_Verify(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op7@example.com")

	zoneID := createZoneLocation(t, db, orgID, "Zone")
	mpID := createMusterPointLocation(t, db, orgID, "MP A")
	personID := createPersonAsset(t, db, orgID, "Person 1")

	insertAssetScan(t, db, orgID, personID, zoneID, time.Now().Add(-3*time.Minute))
	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	// Cannot verify from missing (must be at_muster first)
	_, err = db.Store.UpdateEntryStatus(ctx, orgID, event.ID, event.Entries[0].ID, "verify", userID, "")
	require.Error(t, err)
	var transErr muster.ErrInvalidTransition
	require.ErrorAs(t, err, &transErr)

	// Move to at_muster
	_, err = db.Store.MarkEntryAtMuster(ctx, orgID, event.ID, personID, mpID, time.Now())
	require.NoError(t, err)

	// Now verify succeeds
	verified, err := db.Store.UpdateEntryStatus(ctx, orgID, event.ID, event.Entries[0].ID, "verify", userID, "")
	require.NoError(t, err)
	require.Equal(t, "verified", verified.Status)
	require.NotNil(t, verified.VerifiedBy)
	require.Equal(t, userID, *verified.VerifiedBy)
	require.NotNil(t, verified.VerifiedAt)
}

func TestMuster_UpdateEntryStatus_MarkSafe(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op8@example.com")

	zoneID := createZoneLocation(t, db, orgID, "Zone")
	personID := createPersonAsset(t, db, orgID, "Person 1")

	insertAssetScan(t, db, orgID, personID, zoneID, time.Now().Add(-3*time.Minute))
	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	// Mark safe from missing (valid)
	entry, err := db.Store.UpdateEntryStatus(ctx, orgID, event.ID, event.Entries[0].ID, "mark_safe", userID, "I can see them")
	require.NoError(t, err)
	require.Equal(t, "safe_manual", entry.Status)
	require.NotNil(t, entry.MarkedSafeBy)
	require.Equal(t, userID, *entry.MarkedSafeBy)
	require.NotNil(t, entry.MarkedSafeAt)
	require.Equal(t, "I can see them", entry.MarkedSafeNote)
}

func TestMuster_UpdateEntryStatus_VerifyFromMissing_Invalid(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op9@example.com")

	zoneID := createZoneLocation(t, db, orgID, "Zone")
	personID := createPersonAsset(t, db, orgID, "Person 1")

	insertAssetScan(t, db, orgID, personID, zoneID, time.Now().Add(-3*time.Minute))
	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	// Verify from missing: invalid
	_, err = db.Store.UpdateEntryStatus(ctx, orgID, event.ID, event.Entries[0].ID, "verify", userID, "")
	require.Error(t, err)
	var transErr muster.ErrInvalidTransition
	require.ErrorAs(t, err, &transErr)
	require.Equal(t, "missing", transErr.Current)
	require.Equal(t, "verify", transErr.Action)
}

// ── Counts ────────────────────────────────────────────────────────────────────

func TestMuster_GetActiveMusterEvent_CountsAreCorrect(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op10@example.com")

	zoneID := createZoneLocation(t, db, orgID, "Zone")
	mpID := createMusterPointLocation(t, db, orgID, "MP A")
	p1 := createPersonAsset(t, db, orgID, "P1")
	p2 := createPersonAsset(t, db, orgID, "P2")
	p3 := createPersonAsset(t, db, orgID, "P3")

	now := time.Now()
	insertAssetScan(t, db, orgID, p1, zoneID, now.Add(-2*time.Minute))
	insertAssetScan(t, db, orgID, p2, zoneID, now.Add(-3*time.Minute))
	insertAssetScan(t, db, orgID, p3, zoneID, now.Add(-4*time.Minute))

	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)
	require.Equal(t, 3, event.Counts.Expected)
	require.Equal(t, 3, event.Counts.Missing)

	// Mark p1 at muster
	_, err = db.Store.MarkEntryAtMuster(ctx, orgID, event.ID, p1, mpID, time.Now())
	require.NoError(t, err)

	// Mark p2 safe
	entryP2 := findEntry(t, event.Entries, p2)
	_, err = db.Store.UpdateEntryStatus(ctx, orgID, event.ID, entryP2.ID, "mark_safe", userID, "")
	require.NoError(t, err)

	// Refresh
	active, err := db.Store.GetActiveMusterEvent(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, 3, active.Counts.Expected)
	require.Equal(t, 1, active.Counts.Missing)    // p3 still missing
	require.Equal(t, 1, active.Counts.AtMuster)   // p1
	require.Equal(t, 0, active.Counts.Verified)   // none verified yet
	require.Equal(t, 1, active.Counts.SafeManual) // p2
}

// ── CompleteMusterEvent ───────────────────────────────────────────────────────

func TestMuster_CompleteMusterEvent(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op11@example.com")

	personID := createPersonAsset(t, db, orgID, "Person 1")
	zoneID := createZoneLocation(t, db, orgID, "Zone")
	insertAssetScan(t, db, orgID, personID, zoneID, time.Now().Add(-2*time.Minute))

	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	report := []byte(`{"total_seconds":120,"counts":{"expected":1,"missing":1,"at_muster":0,"verified":0,"safe_manual":0}}`)
	completed, err := db.Store.CompleteMusterEvent(ctx, orgID, event.ID, "completed", report)
	require.NoError(t, err)
	require.NotNil(t, completed)
	require.Equal(t, "completed", completed.Status)
	require.NotNil(t, completed.EndedAt)
	require.NotEmpty(t, completed.Report)

	// After completion, a new event can be created
	event2, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)
	require.NotNil(t, event2)
}

// ── RLS isolation ─────────────────────────────────────────────────────────────

func TestMuster_RLSOrgIsolation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Org B", "muster-test-org-b")

	userA := createMusterUser(t, db, orgA, "user-a-muster@example.com")
	userB := createMusterUser(t, db, orgB, "user-b-muster@example.com")

	personA := createPersonAsset(t, db, orgA, "Org A Person")
	zoneA := createZoneLocation(t, db, orgA, "Zone A")
	insertAssetScan(t, db, orgA, personA, zoneA, time.Now().Add(-2*time.Minute))

	// Create event in org A
	eventA, err := db.Store.CreateMusterEvent(ctx, orgA, userA, 15)
	require.NoError(t, err)
	require.NotZero(t, eventA.ID)

	// Org B sees no active event
	active, err := db.Store.GetActiveMusterEvent(ctx, orgB)
	require.NoError(t, err)
	require.Nil(t, active)

	// Org B cannot read org A's event by ID
	found, err := db.Store.GetMusterEvent(ctx, orgB, eventA.ID)
	require.NoError(t, err)
	require.Nil(t, found)

	// Org B's list is empty
	list, err := db.Store.ListMusterEvents(ctx, orgB)
	require.NoError(t, err)
	require.Empty(t, list)

	// Org B can create its own event without conflict
	personB := createPersonAsset(t, db, orgB, "Org B Person")
	zoneB := createZoneLocation(t, db, orgB, "Zone B")
	insertAssetScan(t, db, orgB, personB, zoneB, time.Now().Add(-2*time.Minute))
	eventB, err := db.Store.CreateMusterEvent(ctx, orgB, userB, 15)
	require.NoError(t, err)
	require.NotEqual(t, eventA.ID, eventB.ID)

	// Org B cannot mark org A's entry
	entryA := eventA.Entries[0]
	mpA := createMusterPointLocation(t, db, orgA, "MP A")
	result, err := db.Store.MarkEntryAtMuster(ctx, orgB, eventA.ID, entryA.AssetID, mpA, time.Now())
	require.NoError(t, err)
	require.Nil(t, result, "cross-org MarkEntryAtMuster must be a no-op")
}

// ── AppendMusterUnlock ────────────────────────────────────────────────────────

func TestMuster_AppendMusterUnlock(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	userID := createMusterUser(t, db, orgID, "op12@example.com")

	personID := createPersonAsset(t, db, orgID, "Person 1")
	zoneID := createZoneLocation(t, db, orgID, "Zone")
	insertAssetScan(t, db, orgID, personID, zoneID, time.Now().Add(-2*time.Minute))

	event, err := db.Store.CreateMusterEvent(ctx, orgID, userID, 15)
	require.NoError(t, err)

	unlock1 := map[string]any{
		"user_id": userID,
		"email":   "op12@example.com",
		"at":      time.Now().Format(time.RFC3339),
		"seq":     1,
	}
	err = db.Store.AppendMusterUnlock(ctx, orgID, event.ID, unlock1)
	require.NoError(t, err)

	unlock2 := map[string]any{
		"user_id": userID,
		"email":   "op12@example.com",
		"at":      time.Now().Add(time.Second).Format(time.RFC3339),
		"seq":     2,
	}
	err = db.Store.AppendMusterUnlock(ctx, orgID, event.ID, unlock2)
	require.NoError(t, err)

	// Verify both records were accumulated (not overwritten)
	refreshed, err := db.Store.GetMusterEvent(ctx, orgID, event.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed)
	unlocks, ok := refreshed.Metadata["unlocks"]
	require.True(t, ok, "metadata.unlocks must exist after AppendMusterUnlock")
	arr, ok := unlocks.([]any)
	require.True(t, ok, "metadata.unlocks must be a JSON array")
	require.Len(t, arr, 2, "two AppendMusterUnlock calls must accumulate, not overwrite")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func findEntry(t *testing.T, entries []muster.Entry, assetID int) muster.Entry {
	t.Helper()
	for _, e := range entries {
		if e.AssetID == assetID {
			return e
		}
	}
	t.Fatalf("no entry found for assetID=%d", assetID)
	return muster.Entry{}
}
