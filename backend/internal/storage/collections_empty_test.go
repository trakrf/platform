package storage

// Regression tests for TRA-182: collection storage methods must return an empty
// slice (which JSON-serializes to `[]`) rather than a nil slice (which serializes
// to `null`) when the underlying query yields zero rows.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/report"
)

// assertEmptyJSONArray marshals v and asserts the JSON output is "[]".
func assertEmptyJSONArray(t *testing.T, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(data),
		"empty collection must serialize to [] not null")
}

func newMockStorage(t *testing.T) (*Storage, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })
	return &Storage{pool: mock}, mock
}

func TestListUsers_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectQuery(`SELECT .* FROM trakrf.users`).
		WithArgs(10, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "email", "name", "password_hash", "last_login_at",
			"settings", "metadata", "created_at", "updated_at",
			"is_superadmin", "last_org_id",
		}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM trakrf.users`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	users, total, err := storage.ListUsers(context.Background(), 10, 0)
	assert.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.NotNil(t, users)
	assertEmptyJSONArray(t, users)
}

func TestListActiveAPIKeys_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectQuery(`FROM trakrf.api_keys`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "jti", "org_id", "name", "scopes",
			"created_by", "created_at", "expires_at", "last_used_at", "revoked_at",
		}))

	keys, err := storage.ListActiveAPIKeys(context.Background(), 1)
	assert.NoError(t, err)
	assert.NotNil(t, keys)
	assertEmptyJSONArray(t, keys)
}

func TestListAllAssets_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`from trakrf.assets`).
		WithArgs(1, 10, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "identifier", "name", "type", "description",
			"current_location_id", "valid_from", "valid_to", "metadata",
			"is_active", "created_at", "updated_at", "deleted_at",
		}))
	mock.ExpectCommit()

	assets, err := storage.ListAllAssets(context.Background(), 1, 10, 0)
	assert.NoError(t, err)
	assert.NotNil(t, assets)
	assertEmptyJSONArray(t, assets)
}

func TestGetAssetsByIDs_EmptyRowsReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	orgID := 1
	ids := []int{99}
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.assets[\s\S]*WHERE org_id = \$1 AND id = ANY\(\$2\)`).
		WithArgs(orgID, ids).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "identifier", "name", "type", "description",
			"current_location_id", "valid_from", "valid_to", "metadata",
			"is_active", "created_at", "updated_at", "deleted_at",
		}))
	mock.ExpectCommit()

	assets, err := storage.GetAssetsByIDs(context.Background(), orgID, ids)
	assert.NoError(t, err)
	assert.NotNil(t, assets)
	assertEmptyJSONArray(t, assets)
}

func TestListAllLocations_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.locations`).
		WithArgs(1, 10, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "name", "identifier", "parent_location_id",
			"path", "depth", "description", "valid_from", "valid_to",
			"is_active", "created_at", "updated_at", "deleted_at",
		}))
	mock.ExpectCommit()

	locations, err := storage.ListAllLocations(context.Background(), 1, 10, 0)
	assert.NoError(t, err)
	assert.NotNil(t, locations)
	assertEmptyJSONArray(t, locations)
}

func TestGetLocationsByIDs_EmptyRowsReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	orgID := 1
	ids := []int{99}
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.locations[\s\S]*WHERE org_id = \$1 AND id = ANY\(\$2\)`).
		WithArgs(orgID, ids).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "name", "identifier", "parent_location_id",
			"path", "depth", "description", "valid_from", "valid_to",
			"is_active", "created_at", "updated_at", "deleted_at",
		}))
	mock.ExpectCommit()

	locations, err := storage.GetLocationsByIDs(context.Background(), orgID, ids)
	assert.NoError(t, err)
	assert.NotNil(t, locations)
	assertEmptyJSONArray(t, locations)
}

func TestGetAncestors_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.locations l`).
		WithArgs(1, 1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "name", "identifier", "parent_location_id",
			"path", "depth", "description", "valid_from", "valid_to",
			"is_active", "created_at", "updated_at", "deleted_at",
			"parent_identifier",
		}))
	mock.ExpectCommit()

	ancestors, err := storage.GetAncestors(context.Background(), 1, 1)
	assert.NoError(t, err)
	assert.NotNil(t, ancestors)
	assertEmptyJSONArray(t, ancestors)
}

func TestGetDescendants_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.locations l`).
		WithArgs(1, 1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "name", "identifier", "parent_location_id",
			"path", "depth", "description", "valid_from", "valid_to",
			"is_active", "created_at", "updated_at", "deleted_at",
			"parent_identifier",
		}))
	mock.ExpectCommit()

	descendants, err := storage.GetDescendants(context.Background(), 1, 1)
	assert.NoError(t, err)
	assert.NotNil(t, descendants)
	assertEmptyJSONArray(t, descendants)
}

func TestGetChildren_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.locations l`).
		WithArgs(1, 1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "name", "identifier", "parent_location_id",
			"path", "depth", "description", "valid_from", "valid_to",
			"is_active", "created_at", "updated_at", "deleted_at",
			"parent_identifier",
		}))
	mock.ExpectCommit()

	children, err := storage.GetChildren(context.Background(), 1, 1)
	assert.NoError(t, err)
	assert.NotNil(t, children)
	assertEmptyJSONArray(t, children)
}

func TestGetIdentifiersByAssetID_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.identifiers`).
		WithArgs(1, 1).
		WillReturnRows(pgxmock.NewRows([]string{"id", "type", "value", "is_active"}))
	mock.ExpectCommit()

	ids, err := storage.GetIdentifiersByAssetID(context.Background(), 1, 1)
	assert.NoError(t, err)
	assert.NotNil(t, ids)
	assertEmptyJSONArray(t, ids)
}

func TestGetIdentifiersByLocationID_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.identifiers`).
		WithArgs(1, 1).
		WillReturnRows(pgxmock.NewRows([]string{"id", "type", "value", "is_active"}))
	mock.ExpectCommit()

	ids, err := storage.GetIdentifiersByLocationID(context.Background(), 1, 1)
	assert.NoError(t, err)
	assert.NotNil(t, ids)
	assertEmptyJSONArray(t, ids)
}

func TestListUserOrgs_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectQuery(`FROM trakrf.organizations o`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}))

	orgs, err := storage.ListUserOrgs(context.Background(), 1)
	assert.NoError(t, err)
	assert.NotNil(t, orgs)
	assertEmptyJSONArray(t, orgs)
}

func TestListPendingInvitations_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectQuery(`FROM trakrf.org_invitations`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "email", "role", "expires_at", "created_at", "u.id", "u.name",
		}))

	invites, err := storage.ListPendingInvitations(context.Background(), 1)
	assert.NoError(t, err)
	assert.NotNil(t, invites)
	assertEmptyJSONArray(t, invites)
}

func TestListOrgMembers_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectQuery(`FROM trakrf.org_users`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{
			"user_id", "name", "email", "role", "joined_at",
		}))

	members, err := storage.ListOrgMembers(context.Background(), 1)
	assert.NoError(t, err)
	assert.NotNil(t, members)
	assertEmptyJSONArray(t, members)
}

func TestListCurrentLocations_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`FROM trakrf.asset_scans`).
		WithArgs(1, nil, nil, 10, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"asset_id", "asset_name", "asset_identifier",
			"location_id", "location_name", "location_identifier", "last_seen",
		}))
	mock.ExpectCommit()

	items, err := storage.ListCurrentLocations(context.Background(), 1, report.CurrentLocationFilter{Limit: 10, Offset: 0})
	assert.NoError(t, err)
	assert.NotNil(t, items)
	assertEmptyJSONArray(t, items)
}

func TestListAssetHistory_EmptyReturnsNonNil(t *testing.T) {
	storage, mock := newMockStorage(t)

	mock.ExpectQuery(`FROM trakrf.asset_scans`).
		WithArgs(1, 2, (*time.Time)(nil), (*time.Time)(nil), 10, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"timestamp", "location_id", "location_name",
			"location_identifier", "duration_seconds",
		}))

	items, err := storage.ListAssetHistory(context.Background(), 1, 2, report.AssetHistoryFilter{Limit: 10, Offset: 0})
	assert.NoError(t, err)
	assert.NotNil(t, items)
	assertEmptyJSONArray(t, items)
}
