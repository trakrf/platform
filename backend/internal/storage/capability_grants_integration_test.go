//go:build integration
// +build integration

// TRA-1027: superadmin capability grant management — the write half of the
// TRA-1024 schema. Reads (trakrf.org_capability_set) are covered by
// capabilities_integration_test.go; these tests pin the declarative set-write.
//
// The write is a whole-set replace rather than per-name grant/revoke calls
// because that is what the superadmin UI submits: a checkbox list plus Save.
// Two properties fall out of that choice and both are pinned below — a name
// already granted must keep its original granted_at (the replace is a diff, not
// a delete-and-reinsert, so "granted since" survives an unrelated edit), and an
// empty request must mean "revoke everything", never "no-op".

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// grantedAt reads the grant timestamp for one (org, capability) pair.
func grantedAt(t *testing.T, db *testutil.TestDB, orgID int, capability string) time.Time {
	t.Helper()
	var ts time.Time
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`SELECT granted_at FROM trakrf.org_capabilities WHERE org_id = $1 AND capability = $2`,
		orgID, capability).Scan(&ts))
	return ts
}

// newOrg seeds a bare org. CreateTestAccount hardcodes one identifier, so tests
// that need a second org seed it directly.
func newOrg(t *testing.T, db *testutil.TestDB, name, identifier string) int {
	t.Helper()
	var id int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ($1, $2, true) RETURNING id`, name, identifier).Scan(&id))
	return id
}

func TestSetOrgCapabilities_GrantsRequestedNames(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	caps, err := db.Store.SetOrgCapabilities(context.Background(), orgID, []string{"geofence"})
	require.NoError(t, err)

	assert.Equal(t, []string{"geofence"}, caps)
	assert.Equal(t, []string{"geofence"}, orgCapabilitySet(t, db.AdminPool, orgID))
}

// The returned set is sorted, matching org_capability_set — callers compare it
// against the read path and a differing order would read as a changed grant.
func TestSetOrgCapabilities_ReturnsSortedSet(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	caps, err := db.Store.SetOrgCapabilities(context.Background(), orgID,
		[]string{"mustering", "geofence", "inventory"})
	require.NoError(t, err)

	assert.Equal(t, []string{"geofence", "inventory", "mustering"}, caps)
}

func TestSetOrgCapabilities_RevokesOmittedNames(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence", "mustering"})
	require.NoError(t, err)

	caps, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"mustering"})
	require.NoError(t, err)

	assert.Equal(t, []string{"mustering"}, caps)
	assert.Equal(t, []string{"mustering"}, orgCapabilitySet(t, db.AdminPool, orgID))
}

// Revoke is a hard delete (ADR 0002: no soft-delete, no audit table beyond
// granted_at), so the row must be gone, not tombstoned.
func TestSetOrgCapabilities_EmptyRequestRevokesEverything(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence", "mustering"})
	require.NoError(t, err)

	caps, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{})
	require.NoError(t, err)
	assert.Empty(t, caps)
	assert.NotNil(t, caps, "empty set must serialize as [], never null")

	var n int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.org_capabilities WHERE org_id = $1`, orgID).Scan(&n))
	assert.Zero(t, n)
}

// A nil slice is the same request as an empty one — a JSON body that omits the
// field must not silently mean "keep what you had".
func TestSetOrgCapabilities_NilRequestRevokesEverything(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence"})
	require.NoError(t, err)

	caps, err := db.Store.SetOrgCapabilities(ctx, orgID, nil)
	require.NoError(t, err)
	assert.Empty(t, caps)
	assert.Empty(t, orgCapabilitySet(t, db.AdminPool, orgID))
}

// Editing one capability must not reset "granted since" on the others.
func TestSetOrgCapabilities_PreservesGrantedAtForUnchangedGrant(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence"})
	require.NoError(t, err)

	// Backdate so a delete-and-reinsert would be visible rather than landing
	// inside the same statement timestamp.
	_, err = db.AdminPool.Exec(ctx,
		`UPDATE trakrf.org_capabilities SET granted_at = now() - interval '30 days'
		 WHERE org_id = $1 AND capability = 'geofence'`, orgID)
	require.NoError(t, err)
	before := grantedAt(t, db, orgID, "geofence")

	_, err = db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence", "mustering"})
	require.NoError(t, err)

	assert.WithinDuration(t, before, grantedAt(t, db, orgID, "geofence"), time.Millisecond,
		"re-submitting an existing grant must not reset granted_at")
}

// Repeating the same request is a no-op, not an error: the superadmin UI submits
// the whole set on every Save, including Saves that changed nothing.
func TestSetOrgCapabilities_IsIdempotent(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	first, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence"})
	require.NoError(t, err)
	second, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence"})
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// Grants are per-org. A whole-set replace is the one write shape where a missing
// org_id predicate would silently revoke every other org's capabilities.
func TestSetOrgCapabilities_LeavesOtherOrgsUntouched(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	otherID := newOrg(t, db, "Bystander Org", "bystander-org")

	_, err := db.Store.SetOrgCapabilities(ctx, otherID, []string{"geofence", "mustering"})
	require.NoError(t, err)

	_, err = db.Store.SetOrgCapabilities(ctx, orgID, []string{"inventory"})
	require.NoError(t, err)

	assert.Equal(t, []string{"geofence", "mustering"}, orgCapabilitySet(t, db.AdminPool, otherID))
}

// No-rows convention, matching UpdateOrgEntitlement: (nil, nil) so the handler
// can answer 404 rather than inventing an org.
func TestSetOrgCapabilities_UnknownOrgReturnsNilNil(t *testing.T) {
	db := testutil.SetupTestDBFull(t)

	caps, err := db.Store.SetOrgCapabilities(context.Background(), 999_999_999, []string{"geofence"})
	require.NoError(t, err)
	assert.Nil(t, caps)
}

func TestSetOrgCapabilities_SoftDeletedOrgReturnsNilNil(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := newOrg(t, db, "Deleted Org", "deleted-org")
	_, err := db.AdminPool.Exec(ctx,
		`UPDATE trakrf.organizations SET deleted_at = now() WHERE id = $1`, orgID)
	require.NoError(t, err)

	caps, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence"})
	require.NoError(t, err)
	assert.Nil(t, caps)
}

// The lookup-table FK is the integrity guarantee (the Go registry check in the
// handler is only a nicer error). A name that slips past validation must still
// be rejected by the database, and the whole write must roll back with it.
func TestSetOrgCapabilities_UnknownCapabilityRejectedAndRollsBack(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.Store.SetOrgCapabilities(ctx, orgID, []string{"geofence"})
	require.NoError(t, err)

	_, err = db.Store.SetOrgCapabilities(ctx, orgID, []string{"wip_tracking"})
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "23503", pgErr.Code, "expected foreign_key_violation, got %s", pgErr.Code)

	assert.Equal(t, []string{"geofence"}, orgCapabilitySet(t, db.AdminPool, orgID),
		"a rejected write must not have revoked the existing grant")
}

// The UI cannot send duplicates, but an API caller can; a repeated name is the
// same grant, not a conflict.
func TestSetOrgCapabilities_DeduplicatesRepeatedNames(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	caps, err := db.Store.SetOrgCapabilities(context.Background(), orgID,
		[]string{"geofence", "geofence"})
	require.NoError(t, err)
	assert.Equal(t, []string{"geofence"}, caps)
}
