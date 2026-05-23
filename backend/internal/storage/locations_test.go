package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/location"
)

func setupLocationTest(t *testing.T) (*Storage, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })
	return &Storage{pool: mock}, mock
}

func TestCreateLocation(t *testing.T) {
	storage, mock := setupLocationTest(t)

	now := time.Now()
	parentID := 1
	request := location.Location{
		Name:        "Warehouse 1",
		ExternalKey: "warehouse_1",
		ParentID:    &parentID,
		Description: "Main warehouse in California",
		ValidFrom:   now,
		ValidTo:     nil,
		IsActive:    true,
		OrgID:       1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		2, request.OrgID, request.Name, request.ExternalKey, request.ParentID,
		request.Description, request.ValidFrom, request.ValidTo,
		request.IsActive, now, now, nil,
	)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.locations`).
		WithArgs(
			request.Name, request.ExternalKey, request.ParentID,
			request.Description, request.ValidFrom, request.ValidTo,
			request.IsActive, request.OrgID,
		).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.CreateLocation(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.ID)
	assert.Equal(t, request.Name, result.Name)
	assert.Equal(t, request.ExternalKey, result.ExternalKey)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateLocation_RootLocation(t *testing.T) {
	storage, mock := setupLocationTest(t)

	now := time.Now()
	request := location.Location{
		Name:        "USA",
		ExternalKey: "usa",
		ParentID:    nil,
		Description: "United States region",
		ValidFrom:   now,
		ValidTo:     nil,
		IsActive:    true,
		OrgID:       1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		1, request.OrgID, request.Name, request.ExternalKey, nil,
		request.Description, request.ValidFrom, request.ValidTo,
		request.IsActive, now, now, nil,
	)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.locations`).
		WithArgs(
			request.Name, request.ExternalKey, request.ParentID,
			request.Description, request.ValidFrom, request.ValidTo,
			request.IsActive, request.OrgID,
		).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.CreateLocation(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ID)
	assert.Nil(t, result.ParentID)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateLocation_DuplicateExternalKey(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := location.Location{
		Name:        "Warehouse 1",
		ExternalKey: "warehouse_1",
		ParentID:    nil,
		Description: "Test",
		ValidFrom:   now,
		ValidTo:     nil,
		IsActive:    true,
		OrgID:       1,
	}

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.locations`).
		WithArgs(
			request.Name, request.ExternalKey, request.ParentID,
			request.Description, request.ValidFrom, request.ValidTo,
			request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("ERROR: duplicate key value violates unique constraint"))
	mock.ExpectRollback()

	result, err := storage.CreateLocation(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "location with external_key warehouse_1 already exists")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateLocation_InvalidParentID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	parentID := 99999
	request := location.Location{
		Name:        "Child Location",
		ExternalKey: "child",
		ParentID:    &parentID,
		Description: "Test",
		ValidFrom:   now,
		ValidTo:     nil,
		IsActive:    true,
		OrgID:       1,
	}

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.locations`).
		WithArgs(
			request.Name, request.ExternalKey, request.ParentID,
			request.Description, request.ValidFrom, request.ValidTo,
			request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("ERROR: insert or update on table \"locations\" violates foreign key constraint \"locations_parent_location_id_fkey\""))
	mock.ExpectRollback()

	result, err := storage.CreateLocation(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid parent_location_id: parent location does not exist")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	locationID := 1
	newName := "Updated Warehouse Name"
	newDescription := "Updated description"

	request := location.UpdateLocationRequest{
		Name:        &newName,
		Description: &newDescription,
	}

	// UPDATE ... RETURNING id (wrapped in WithOrgTx)
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`UPDATE trakrf.locations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(locationID))
	mock.ExpectCommit()

	// getLocationWithParentByID: SELECT location + joined parent external_key (wrapped in WithOrgTx)
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT[\s\S]+FROM trakrf.locations l[\s\S]+LEFT JOIN trakrf.locations p`).
		WithArgs(locationID, 1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "name", "external_key", "parent_location_id",
			"description", "valid_from", "valid_to", "is_active",
			"created_at", "updated_at", "deleted_at", "parent_external_key",
		}).AddRow(
			locationID, 1, newName, "warehouse_1", nil,
			newDescription, now, nil, true, now, now, nil, nil,
		))
	mock.ExpectCommit()

	// GetTagsByLocationID: empty tags (wrapped in WithOrgTx)
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value[\s\S]+FROM trakrf.tags`).
		WithArgs(locationID, 1).
		WillReturnRows(pgxmock.NewRows([]string{"id", "type", "value"}))
	mock.ExpectCommit()

	result, err := storage.UpdateLocation(context.Background(), 1, locationID, request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, locationID, result.ID)
	assert.Equal(t, newName, result.Name)
	assert.Equal(t, newDescription, result.Description)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateLocation_MoveToNewParent(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	locationID := 3
	newParentID := 2

	request := location.UpdateLocationRequest{
		ParentID: &newParentID,
	}

	// UPDATE ... RETURNING id (wrapped in WithOrgTx)
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`UPDATE trakrf.locations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(locationID))
	mock.ExpectCommit()

	// getLocationWithParentByID: SELECT location + joined parent external_key (wrapped in WithOrgTx)
	parentExternalKey := "california"
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT[\s\S]+FROM trakrf.locations l[\s\S]+LEFT JOIN trakrf.locations p`).
		WithArgs(locationID, 1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "name", "external_key", "parent_location_id",
			"description", "valid_from", "valid_to", "is_active",
			"created_at", "updated_at", "deleted_at", "parent_external_key",
		}).AddRow(
			locationID, 1, "Zone A", "zone_a", &newParentID,
			"Test zone", now, nil, true,
			now, now, nil, &parentExternalKey,
		))
	mock.ExpectCommit()

	// GetTagsByLocationID: empty tags (wrapped in WithOrgTx)
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value[\s\S]+FROM trakrf.tags`).
		WithArgs(locationID, 1).
		WillReturnRows(pgxmock.NewRows([]string{"id", "type", "value"}))
	mock.ExpectCommit()

	result, err := storage.UpdateLocation(context.Background(), 1, locationID, request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, locationID, result.ID)
	assert.Equal(t, &newParentID, result.ParentID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TRA-619 / TRA-783: an empty UpdateLocationRequest (e.g. the PUT body
// decoded to no writable fields after the read-only drop, or a literal `{}`)
// is a no-op-with-touch success — TRA-783 always advances updated_at on
// accepted PATCH (filesystem `touch` semantics), so the storage layer issues
// an UPDATE that only sets updated_at and returns the (now-touched) record.
func TestUpdateLocation_NoFields(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	locationID := 1
	request := location.UpdateLocationRequest{}

	// TRA-783: UPDATE always issued — even with no settable fields the query
	// sets `updated_at = NOW()` and RETURNINGs the row id.
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`UPDATE trakrf.locations[\s\S]+SET updated_at = NOW\(\)[\s\S]+RETURNING id`).
		WithArgs(locationID, 1).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(locationID))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT[\s\S]+FROM trakrf.locations l[\s\S]+LEFT JOIN trakrf.locations p`).
		WithArgs(locationID, 1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "org_id", "name", "external_key", "parent_location_id",
			"description", "valid_from", "valid_to", "is_active",
			"created_at", "updated_at", "deleted_at", "parent_external_key",
		}).AddRow(
			locationID, 1, "Warehouse 1", "warehouse_1", nil,
			"", now, nil, true, now, now, nil, nil,
		))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value[\s\S]+FROM trakrf.tags`).
		WithArgs(locationID, 1).
		WillReturnRows(pgxmock.NewRows([]string{"id", "type", "value"}))
	mock.ExpectCommit()

	result, err := storage.UpdateLocation(context.Background(), 1, locationID, request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, locationID, result.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateLocation_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 99999
	newName := "Updated Name"
	request := location.UpdateLocationRequest{
		Name: &newName,
	}

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`UPDATE trakrf.locations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("no rows in result set"))
	mock.ExpectRollback()

	result, err := storage.UpdateLocation(context.Background(), 1, locationID, request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to update location")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLocationByID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	locationID := 1

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		locationID, 1, "USA", "usa", nil,
		"United States", now, nil, true, now, now, nil,
	)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, org_id, name, external_key`).
		WithArgs(locationID, 1).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.GetLocationByID(context.Background(), 1, locationID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, locationID, result.ID)
	assert.Equal(t, "USA", result.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLocationByID_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 99999

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, org_id, name, external_key`).
		WithArgs(locationID, 1).
		WillReturnError(errors.New("no rows in result set"))
	mock.ExpectRollback()

	result, err := storage.GetLocationByID(context.Background(), 1, locationID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get location by id")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListAllLocations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	orgID := 1
	limit := 10
	offset := 0

	parent1 := 1
	parent2 := 2
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).
		AddRow(1, orgID, "USA", "usa", nil, "United States", now, nil, true, now, &now, nil).
		AddRow(2, orgID, "California", "california", &parent1, "California State", now, nil, true, now, &now, nil).
		AddRow(3, orgID, "Warehouse 1", "warehouse_1", &parent2, "Main Warehouse", now, nil, true, now, &now, nil)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, org_id, name, external_key`).
		WithArgs(orgID, limit, offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.ListAllLocations(context.Background(), orgID, limit, offset)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 3)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListAllLocations_Empty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	limit := 10
	offset := 0

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	})

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, org_id, name, external_key`).
		WithArgs(orgID, limit, offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.ListAllLocations(context.Background(), orgID, limit, offset)

	assert.NoError(t, err)
	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountAllLocations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	expectedCount := 42

	rows := pgxmock.NewRows([]string{"count"}).AddRow(expectedCount)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs(orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	count, err := storage.CountAllLocations(context.Background(), orgID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 1
	orgID := 1

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectExec(`UPDATE trakrf.locations SET deleted_at`).
		WithArgs(locationID, orgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`UPDATE trakrf.tags`).
		WithArgs(locationID, orgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectCommit()

	result, err := storage.DeleteLocation(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	assert.True(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteLocation_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 99999
	orgID := 1

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectExec(`UPDATE trakrf.locations SET deleted_at`).
		WithArgs(locationID, orgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectCommit()

	result, err := storage.DeleteLocation(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAncestors(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	orgID := 1
	locationID := 3 // usa.california.warehouse_1

	parent1 := 1
	usaIdent := "usa"
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	}).
		AddRow(1, 1, "USA", "usa", nil, "United States", now, nil, true, now, &now, nil, nil).
		AddRow(2, 1, "California", "california", &parent1, "California State", now, nil, true, now, &now, nil, &usaIdent)

	// scanHierarchyRows: hierarchy query (wrapped in WithOrgTx)
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.external_key`).
		WithArgs(orgID, locationID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	// getTagsForLocations (wrapped in WithOrgTx)
	identifierRows := pgxmock.NewRows([]string{"location_id", "id", "type", "value"})
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value`).
		WithArgs([]int{1, 2}, orgID).
		WillReturnRows(identifierRows)
	mock.ExpectCommit()

	results, err := storage.GetAncestors(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 2)
	assert.Nil(t, results[0].ParentExternalKey, "root ancestor must have no parent identifier")
	require.NotNil(t, results[1].ParentExternalKey)
	assert.Equal(t, "usa", *results[1].ParentExternalKey)
	assert.NotNil(t, results[1].Tags, "Tags must be non-nil empty slice, not nil")
	assert.Len(t, results[1].Tags, 0)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAncestors_RootLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 1 // Root location - no ancestors

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	})

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.external_key`).
		WithArgs(orgID, locationID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.GetAncestors(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListAncestorsPaginated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	orgID := 1
	locationID := 3
	limit := 1
	offset := 1

	parent1 := 1
	usaIdent := "usa"
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	}).
		AddRow(2, 1, "California", "california", &parent1, "California State", now, nil, true, now, &now, nil, &usaIdent)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`ORDER BY a.rdepth ASC, l.id ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(orgID, locationID, limit, offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value`).
		WithArgs([]int{2}, orgID).
		WillReturnRows(pgxmock.NewRows([]string{"location_id", "id", "type", "value"}))
	mock.ExpectCommit()

	results, err := storage.ListAncestorsPaginated(context.Background(), orgID, locationID, limit, offset)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountAncestors(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	orgID := 1
	locationID := 3

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT count\(\*\) FROM ancestors`).
		WithArgs(orgID, locationID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectCommit()

	n, err := storage.CountAncestors(context.Background(), orgID, locationID)
	assert.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDescendants(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	orgID := 1
	locationID := 1 // usa

	parent1 := 1
	parent2 := 2
	parent3 := 3
	usaIdent := "usa"
	caIdent := "california"
	whIdent := "warehouse_1"
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	}).
		AddRow(2, 1, "California", "california", &parent1, "California State", now, nil, true, now, &now, nil, &usaIdent).
		AddRow(3, 1, "Warehouse 1", "warehouse_1", &parent2, "Main Warehouse", now, nil, true, now, &now, nil, &caIdent).
		AddRow(4, 1, "Zone A", "zone_a", &parent3, "Storage Zone A", now, nil, true, now, &now, nil, &whIdent)

	// scanHierarchyRows: hierarchy query (wrapped in WithOrgTx)
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.external_key`).
		WithArgs(orgID, locationID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	// getTagsForLocations (wrapped in WithOrgTx)
	identifierRows := pgxmock.NewRows([]string{"location_id", "id", "type", "value"})
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value`).
		WithArgs([]int{2, 3, 4}, orgID).
		WillReturnRows(identifierRows)
	mock.ExpectCommit()

	results, err := storage.GetDescendants(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 3)
	require.NotNil(t, results[0].ParentExternalKey)
	assert.Equal(t, "usa", *results[0].ParentExternalKey)
	require.NotNil(t, results[1].ParentExternalKey)
	assert.Equal(t, "california", *results[1].ParentExternalKey)
	require.NotNil(t, results[2].ParentExternalKey)
	assert.Equal(t, "warehouse_1", *results[2].ParentExternalKey)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDescendants_LeafLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 10 // Leaf location - no descendants

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	})

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.external_key`).
		WithArgs(orgID, locationID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.GetDescendants(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListDescendantsPaginated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	now := time.Now()
	orgID := 1
	rootID := 1
	limit := 2
	offset := 1

	parentRef := 1
	rootIdent := "root"
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	}).
		AddRow(3, 1, "B", "b", &parentRef, "", now, nil, true, now, &now, nil, &rootIdent).
		AddRow(4, 1, "C", "c", &parentRef, "", now, nil, true, now, &now, nil, &rootIdent)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`ORDER BY s.sort_path ASC, l.id ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(orgID, rootID, limit, offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value`).
		WithArgs([]int{3, 4}, orgID).
		WillReturnRows(pgxmock.NewRows([]string{"location_id", "id", "type", "value"}))
	mock.ExpectCommit()

	results, err := storage.ListDescendantsPaginated(context.Background(), orgID, rootID, limit, offset)
	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountDescendants(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	orgID := 1
	rootID := 1

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT count\(\*\) FROM subtree`).
		WithArgs(orgID, rootID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(7))
	mock.ExpectCommit()

	n, err := storage.CountDescendants(context.Background(), orgID, rootID)
	assert.NoError(t, err)
	assert.Equal(t, 7, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetChildren(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	orgID := 1
	parentID := 2 // usa.california
	caIdent := "california"

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	}).
		AddRow(3, 1, "Warehouse 1", "warehouse_1", &parentID, "Main Warehouse", now, nil, true, now, &now, nil, &caIdent).
		AddRow(4, 1, "Warehouse 2", "warehouse_2", &parentID, "Secondary Warehouse", now, nil, true, now, &now, nil, &caIdent)

	// scanHierarchyRows: hierarchy query (wrapped in WithOrgTx)
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.external_key`).
		WithArgs(orgID, parentID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	// getTagsForLocations (wrapped in WithOrgTx)
	identifierRows := pgxmock.NewRows([]string{"location_id", "id", "type", "value"})
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value`).
		WithArgs([]int{3, 4}, orgID).
		WillReturnRows(identifierRows)
	mock.ExpectCommit()

	results, err := storage.GetChildren(context.Background(), orgID, parentID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 2)
	assert.Equal(t, "Warehouse 1", results[0].Name)
	assert.Equal(t, "Warehouse 2", results[1].Name)
	require.NotNil(t, results[0].ParentExternalKey)
	assert.Equal(t, "california", *results[0].ParentExternalKey)
	require.NotNil(t, results[1].ParentExternalKey)
	assert.Equal(t, "california", *results[1].ParentExternalKey)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetChildren_NoChildren(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	parentID := 10 // Location with no children

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	})

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.external_key`).
		WithArgs(orgID, parentID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.GetChildren(context.Background(), orgID, parentID)

	assert.NoError(t, err)
	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListChildrenPaginated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	now := time.Now()
	orgID := 1
	parentID := 1
	limit := 2
	offset := 0

	parentRef := 1
	parentIdent := "parent"
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_external_key",
	}).
		AddRow(2, 1, "Aisle A", "aisle-a", &parentRef, "", now, nil, true, now, &now, nil, &parentIdent).
		AddRow(3, 1, "Aisle B", "aisle-b", &parentRef, "", now, nil, true, now, &now, nil, &parentIdent)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`ORDER BY l.name ASC, l.id ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(orgID, parentID, limit, offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value`).
		WithArgs([]int{2, 3}, orgID).
		WillReturnRows(pgxmock.NewRows([]string{"location_id", "id", "type", "value"}))
	mock.ExpectCommit()

	results, err := storage.ListChildrenPaginated(context.Background(), orgID, parentID, limit, offset)
	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountChildren(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	orgID := 1
	parentID := 1

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM trakrf\.locations`).
		WithArgs(orgID, parentID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectCommit()

	n, err := storage.CountChildren(context.Background(), orgID, parentID)
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLocationWithRelations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	targetID := 3 // usa.california.warehouse_1
	parent1 := 1
	parent2 := 2

	// Simulate ltree query that returns target + ancestors + children in one query
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at", "relation_type",
	}).
		// Target location
		AddRow(3, 1, "Warehouse 1", "warehouse_1", &parent2,
			"Main Warehouse", now, nil, true, now, &now, nil, "target").
		// Ancestors (ordered by depth)
		AddRow(1, 1, "USA", "usa", nil,
			"United States", now, nil, true, now, &now, nil, "ancestor").
		AddRow(2, 1, "California", "california", &parent1,
			"California State", now, nil, true, now, &now, nil, "ancestor").
		// Children (immediate only)
		AddRow(4, 1, "Zone A", "zone_a", &targetID,
			"Storage Zone A", now, nil, true, now, &now, nil, "child").
		AddRow(5, 1, "Zone B", "zone_b", &targetID,
			"Storage Zone B", now, nil, true, now, &now, nil, "child")

	orgID := 1
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`WITH RECURSIVE ancestors_raw AS`).
		WithArgs(targetID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.GetLocationWithRelations(context.Background(), orgID, targetID)

	assert.NoError(t, err)
	require.NotNil(t, result)

	// Verify target location
	assert.Equal(t, targetID, result.ID)
	assert.Equal(t, "Warehouse 1", result.Name)

	// Verify ancestors (should have 2: USA and California)
	require.Len(t, result.Ancestors, 2)
	assert.Equal(t, "USA", result.Ancestors[0].Name)
	assert.Equal(t, "California", result.Ancestors[1].Name)

	// Verify children (should have 2: Zone A and Zone B)
	require.Len(t, result.Children, 2)
	assert.Equal(t, "Zone A", result.Children[0].Name)
	assert.Equal(t, "Zone B", result.Children[1].Name)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLocationWithRelations_RootLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	rootID := 1

	// Root location has no ancestors, only children
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at", "relation_type",
	}).
		AddRow(1, 1, "USA", "usa", nil,
			"United States", now, nil, true, now, &now, nil, "target").
		AddRow(2, 1, "California", "california", &rootID,
			"California State", now, nil, true, now, &now, nil, "child").
		AddRow(3, 1, "Texas", "texas", &rootID,
			"Texas State", now, nil, true, now, &now, nil, "child")

	orgID := 1
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`WITH RECURSIVE ancestors_raw AS`).
		WithArgs(rootID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.GetLocationWithRelations(context.Background(), orgID, rootID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "USA", result.Name)
	assert.Empty(t, result.Ancestors) // Root has no ancestors
	require.Len(t, result.Children, 2)
	assert.Equal(t, "California", result.Children[0].Name)
	assert.Equal(t, "Texas", result.Children[1].Name)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLocationWithRelations_LeafLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	leafID := 4
	parent1 := 1
	parent2 := 2
	parent3 := 3

	// Leaf location has ancestors but no children
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at", "relation_type",
	}).
		AddRow(4, 1, "Zone A", "zone_a", &parent3,
			"Storage Zone A", now, nil, true, now, &now, nil, "target").
		AddRow(1, 1, "USA", "usa", nil,
			"United States", now, nil, true, now, &now, nil, "ancestor").
		AddRow(2, 1, "California", "california", &parent1,
			"California State", now, nil, true, now, &now, nil, "ancestor").
		AddRow(3, 1, "Warehouse 1", "warehouse_1", &parent2,
			"Main Warehouse", now, nil, true, now, &now, nil, "ancestor")

	orgID := 1
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`WITH RECURSIVE ancestors_raw AS`).
		WithArgs(leafID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.GetLocationWithRelations(context.Background(), orgID, leafID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Zone A", result.Name)
	require.Len(t, result.Ancestors, 3) // Has 3 ancestors
	assert.Equal(t, "USA", result.Ancestors[0].Name)
	assert.Equal(t, "California", result.Ancestors[1].Name)
	assert.Equal(t, "Warehouse 1", result.Ancestors[2].Name)
	assert.Empty(t, result.Children) // Leaf has no children

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLocationWithRelations_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 99999

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "external_key", "parent_location_id",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at", "relation_type",
	})

	orgID := 1
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`WITH RECURSIVE ancestors_raw AS`).
		WithArgs(locationID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.GetLocationWithRelations(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}
