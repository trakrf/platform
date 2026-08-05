//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/user"
	"github.com/trakrf/platform/backend/internal/storage"
)

// insertUser seeds a user row directly.
//
// It replaces storage.CreateUser, which was removed with POST /api/v1/users
// (TRA-1103) and had become production code kept alive only by tests. The tests
// below never exercised creation itself — they need a user to exist so they can
// assert something about update, deletion, or invitations — so a raw insert says
// what they actually mean.
//
// password_hash is a stub: nothing here authenticates, and a real bcrypt hash
// would only slow the suite down.
func insertUser(t *testing.T, store *storage.Storage, email, name string) *user.User {
	t.Helper()

	pool := store.Pool().(*pgxpool.Pool)
	var usr user.User
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $2, 'stub-hash')
		RETURNING id, email, name`,
		email, name,
	).Scan(&usr.ID, &usr.Email, &usr.Name)
	require.NoError(t, err)

	return &usr
}
