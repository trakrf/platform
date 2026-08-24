//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/user"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-1135: users carry a must_change_password flag so an operator-provisioned
// account can be forced to rotate its bootstrap password at first login.

// setMustChangePassword flips the flag with raw SQL, standing in for whatever
// provisioned the account. Reads under test go through storage.
func setMustChangePassword(t *testing.T, pool *pgxpool.Pool, userID int, value bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.users SET must_change_password = $2 WHERE id = $1`, userID, value)
	require.NoError(t, err)
}

// A user created without saying anything about the flag must not be forced to
// rotate. Every account that predates this column is in exactly that position,
// so the default is what keeps them logged in.
func TestUserMustChangePasswordDefaultsFalse(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	seeded := insertUser(t, store, "must-change-default@example.com", "Default User")

	byID, err := store.GetUserByID(context.Background(), seeded.ID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	assert.False(t, byID.MustChangePassword, "a newly seeded user must not be flagged")
}

// The login path reads users by email and the change-password path reads them
// by id, so both have to carry the flag or the gate is invisible to one of them.
func TestUserMustChangePasswordVisibleByIDAndEmail(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	const email = "must-change-flagged@example.com"
	seeded := insertUser(t, store, email, "Flagged User")
	setMustChangePassword(t, pool, seeded.ID, true)

	byID, err := store.GetUserByID(ctx, seeded.ID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	assert.True(t, byID.MustChangePassword, "GetUserByID must surface the flag")

	byEmail, err := store.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	require.NotNil(t, byEmail)
	assert.True(t, byEmail.MustChangePassword, "GetUserByEmail must surface the flag")
}

// The superadmin back-office surface (PUT /api/v1/users/{id}) is the onsite
// operator's tool for flagging an already-provisioned account.
func TestUpdateUserSetsMustChangePassword(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	seeded := insertUser(t, store, "must-change-toggle@example.com", "Toggle User")

	flag := true
	updated, err := store.UpdateUser(ctx, seeded.ID, user.UpdateUserRequest{MustChangePassword: &flag})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.True(t, updated.MustChangePassword, "the update response must reflect the new flag")

	reread, err := store.GetUserByID(ctx, seeded.ID)
	require.NoError(t, err)
	assert.True(t, reread.MustChangePassword, "the flag must persist")

	clear := false
	cleared, err := store.UpdateUser(ctx, seeded.ID, user.UpdateUserRequest{MustChangePassword: &clear})
	require.NoError(t, err)
	assert.False(t, cleared.MustChangePassword, "an operator must be able to clear it again")
}

// A partial update that says nothing about the flag must leave it alone —
// otherwise renaming a flagged user would quietly let them back into the app.
func TestUpdateUserLeavesMustChangePasswordAloneWhenOmitted(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	seeded := insertUser(t, store, "must-change-untouched@example.com", "Untouched User")
	setMustChangePassword(t, pool, seeded.ID, true)

	name := "Renamed User"
	updated, err := store.UpdateUser(ctx, seeded.ID, user.UpdateUserRequest{Name: &name})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Renamed User", updated.Name)
	assert.True(t, updated.MustChangePassword, "an unrelated edit must not clear the flag")
}
