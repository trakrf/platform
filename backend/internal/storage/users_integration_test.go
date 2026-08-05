//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/user"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-958: UpdateUser used to fall through to a wrapped pgx error on a unique
// violation, which made the 409 branch in the users handlers unreachable and
// surfaced a colliding email as a 500.
func TestUpdateUser_DuplicateEmailReturnsSentinel(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	// The email this test collides with; the row exists only to occupy it.
	insertUser(t, store, "taken-tra958@example.com", "Taken")

	mover := insertUser(t, store, "mover-tra958@example.com", "Mover")

	email := "taken-tra958@example.com"
	got, err := store.UpdateUser(ctx, mover.ID, user.UpdateUserRequest{Email: &email})
	require.Nil(t, got)
	require.ErrorIs(t, err, modelerrors.ErrUserDuplicateEmail)
}

// A rename that collides with nothing still has to work — the mapping above
// must not swallow ordinary updates.
func TestUpdateUser_RenameSucceeds(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	created := insertUser(t, store, "rename-tra958@example.com", "Before")

	name := "After"
	got, err := store.UpdateUser(ctx, created.ID, user.UpdateUserRequest{Name: &name})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "After", got.Name)
	require.Equal(t, "rename-tra958@example.com", got.Email)
}
