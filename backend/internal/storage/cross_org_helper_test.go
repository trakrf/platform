//go:build integration
// +build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// createOrg creates an additional organization beyond the default test_org
// for cross-tenant integration tests. testutil.CreateTestAccount hardcodes
// identifier="test-org" and the organizations.identifier column is UNIQUE,
// so cross-org tests need a second org with a distinct natural key.
func createOrg(t *testing.T, pool *pgxpool.Pool, name, identifier string) int {
	t.Helper()
	var orgID int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ($1, $2, true) RETURNING id`,
		name, identifier,
	).Scan(&orgID)
	require.NoError(t, err)
	return orgID
}
