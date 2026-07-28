//go:build integration
// +build integration

// TRA-1024: capability grant schema (ADR 0002). These tests pin the four things
// the schema promises and nothing more — the middleware that consumes them is
// TRA-1025, the frontend gating TRA-1026, grant management TRA-1027.
//
// The load-bearing assertion is the zero-grant default: asset management is the
// always-on base and every org — pre-existing and newly created, on both
// creation paths — starts with no capability rows. A "helpful" default grant
// added later would silently unlock gated surfaces for every org, so it is
// pinned here on the real signup and org-create paths, not just on the table.

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmodels "github.com/trakrf/platform/backend/internal/models/auth"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// orgCapabilitySet calls the SQL function under test and returns the raw slice.
func orgCapabilitySet(t *testing.T, pool *pgxpool.Pool, orgID int) []string {
	t.Helper()
	var caps []string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT trakrf.org_capability_set($1)`, orgID).Scan(&caps))
	return caps
}

// The lookup table is the code-owned vocabulary: it must match capability.All
// (TRA-1025) exactly, whatever that set's current size. Drift in either
// direction — a seeded name missing from the Go registry, or a registry entry
// not yet seeded — is the failure this test exists to catch.
func TestCapabilities_SeededVocabulary(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	rows, err := pool.Query(context.Background(),
		`SELECT name FROM trakrf.capabilities ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []string{"geofence", "inventory", "kitting", "mustering"}, names)
}

// Zero grants is the default, and the function must say so with an empty array
// rather than NULL — a NULL would make every caller's len()/contains check a
// nil-handling exercise, and RequireCap would read "no capability set loaded"
// where the truth is "loaded, and it is empty".
func TestOrgCapabilitySet_EmptyArrayForUngrantedOrg(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	var caps []string
	var isNull bool
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`SELECT trakrf.org_capability_set($1), trakrf.org_capability_set($1) IS NULL`,
		orgID).Scan(&caps, &isNull))

	assert.False(t, isNull, "org_capability_set must return an empty array, never NULL")
	assert.Empty(t, caps)
}

func TestOrgCapabilitySet_ReturnsGrantedNames(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.org_capabilities (org_id, capability)
		 VALUES ($1, 'geofence'), ($1, 'mustering')`, orgID)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"geofence", "mustering"},
		orgCapabilitySet(t, db.AdminPool, orgID))

	// A second org's grants must not bleed into the first org's set.
	// (CreateTestAccount hardcodes one identifier, so seed this one directly.)
	var otherID int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.organizations (name, identifier, is_active)
		VALUES ('Other Capabilities Org', 'other-capabilities-org', true)
		RETURNING id`).Scan(&otherID))
	_, err = db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.org_capabilities (org_id, capability) VALUES ($1, 'inventory')`, otherID)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"geofence", "mustering"},
		orgCapabilitySet(t, db.AdminPool, orgID))
	assert.Equal(t, []string{"inventory"}, orgCapabilitySet(t, db.AdminPool, otherID))
}

// The FK to the lookup table is the integrity guarantee that lets the Go
// registry be the only place capability names are minted: a typo'd or invented
// name cannot be granted.
func TestOrgCapabilities_UnknownCapabilityRejected(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.org_capabilities (org_id, capability) VALUES ($1, 'wip_tracking')`, orgID)
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "23503", pgErr.Code, "expected foreign_key_violation, got %s", pgErr.Code)
}

// Grants are per (org, capability) and idempotent-by-conflict: granting twice is
// a PK violation, not a duplicate row that makes revoke a partial operation.
func TestOrgCapabilities_DuplicateGrantRejected(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.org_capabilities (org_id, capability) VALUES ($1, 'geofence')`, orgID)
	require.NoError(t, err)

	_, err = db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.org_capabilities (org_id, capability) VALUES ($1, 'geofence')`, orgID)
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "23505", pgErr.Code, "expected unique_violation, got %s", pgErr.Code)
}

// No backfill: the migration seeds the vocabulary but grants it to nobody. This
// is what makes the deploy sequencing in TRA-1046 matter, so it is pinned.
func TestOrgCapabilities_MigrationGrantsNothing(t *testing.T) {
	db := testutil.SetupTestDBFull(t)

	var n int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`SELECT count(*) FROM trakrf.org_capabilities`).Scan(&n))
	assert.Zero(t, n, "migration must not backfill any org with capability grants")
}

// The set is read by request middleware before org context exists, by the
// RLS-enforced app role. SECURITY DEFINER is what makes that possible — the same
// posture as trakrf.org_is_entitled.
func TestOrgCapabilitySet_CallableByAppRoleWithoutOrgContext(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.org_capabilities (org_id, capability) VALUES ($1, 'geofence')`, orgID)
	require.NoError(t, err)

	var caps []string
	err = db.AppPool.QueryRow(ctx, `SELECT trakrf.org_capability_set($1)`, orgID).Scan(&caps)
	require.NoError(t, err,
		"app role must be able to read the capability set with no app.current_org_id set")
	assert.Equal(t, []string{"geofence"}, caps)
}

// Signup path (personal org on self-service signup): zero grants. Trial orgs get
// the always-on asset base; gated capabilities come from a sales conversation.
func TestSignup_CreatesOrgWithNoCapabilityGrants(t *testing.T) {
	t.Setenv("JWT_SECRET", "capabilities-signup-test")
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()

	svc := authservice.NewService(db.AdminPool, db.Store, nil)
	resp, err := svc.Signup(ctx, authmodels.SignupRequest{
		Email:    "capabilities-signup@example.com",
		Password: "s3cret!!",
		OrgName:  "Capabilities Signup Org",
		Name:     "Cap Tester",
		Phone:    "+1-555-0100",
		Website:  "https://example.com",
	}, "", "",
		func(pw string) (string, error) { return "hashed-" + pw, nil },
		func(int, string, *int) (string, error) { return "stub-token", nil },
	)
	require.NoError(t, err)

	var orgID int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT org_id FROM trakrf.org_users WHERE user_id = $1 LIMIT 1`, resp.User.ID).Scan(&orgID))

	assert.Empty(t, orgCapabilitySet(t, db.AdminPool, orgID),
		"self-service signup must not grant any capability")
}

// Explicit org-create path (CreateOrgWithAdmin): zero grants, same as signup.
func TestCreateOrgWithAdmin_CreatesOrgWithNoCapabilityGrants(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()

	var userID int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, 'Cap Tester', 'stub')
		RETURNING id`, "capabilities-orgcreate@example.com").Scan(&userID))

	svc := orgsservice.NewService(db.AdminPool, db.Store, nil)
	org, err := svc.CreateOrgWithAdmin(ctx, "Capabilities Team Org", userID, "capabilities-orgcreate@example.com")
	require.NoError(t, err)

	assert.Empty(t, orgCapabilitySet(t, db.AdminPool, org.ID),
		"CreateOrgWithAdmin must not grant any capability")
}
