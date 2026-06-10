//go:build integration

package orgs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-922: /users/me must surface the current org's identifier (slug) so the UI
// can pre-fill the required {org_slug}/ publish_topic prefix.
func TestGetUserProfile_IncludesCurrentOrgIdentifier(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool) // identifier "test-org"

	var userID int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.users (email, name, password_hash, is_superadmin) VALUES ('tra922@t.com', 'TRA922', 'x', false) RETURNING id`,
	).Scan(&userID))
	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.org_users (org_id, user_id, role, status) VALUES ($1, $2, 'admin', 'active')`,
		orgID, userID)
	require.NoError(t, err)

	svc := orgsservice.NewService(db.AdminPool, db.Store, nil)
	profile, err := svc.GetUserProfile(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, profile.CurrentOrg)
	assert.Equal(t, orgID, profile.CurrentOrg.ID)
	assert.Equal(t, "test-org", profile.CurrentOrg.Identifier)
}
