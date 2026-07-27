//go:build integration
// +build integration

// TRA-1025 / ADR 0002: the capability vocabulary is code-owned, and the seeded
// `trakrf.capabilities` lookup table mirrors it. Two artifacts, one truth —
// this test is the thing that stops them drifting.
//
// Drift is silent in both directions and neither is caught elsewhere: a name
// added to the Go registry but not seeded can never be granted (the FK rejects
// it), and a name seeded but absent from the registry is grantable yet has no
// constant to gate a route with.

package capability_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/capability"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestRegistryMatchesSeededTable(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	rows, err := pool.Query(context.Background(),
		`SELECT name FROM trakrf.capabilities ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()

	var seeded []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		seeded = append(seeded, n)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, capability.All, seeded,
		"the Go capability registry and the seeded capabilities table have drifted — "+
			"add the name to BOTH internal/capability and a migration, or to neither")
}

func TestIsValidAcceptsExactlyTheSeededNames(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	for _, name := range capability.All {
		var exists bool
		require.NoError(t, pool.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM trakrf.capabilities WHERE name = $1)`, name).Scan(&exists))
		assert.True(t, exists, "%q is in the registry but not seeded", name)
		assert.True(t, capability.IsValid(name))
	}

	// A workflow name that was never minted — the shape a one-off would take
	// before it is added to both artifacts.
	assert.False(t, capability.IsValid("wip_tracking"))
	// Asset management is the always-on base, never a capability.
	assert.False(t, capability.IsValid("asset"))
}
