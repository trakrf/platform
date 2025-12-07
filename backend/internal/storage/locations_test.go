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
		Name:             "Warehouse 1",
		Identifier:       "warehouse_1",
		ParentLocationID: &parentID,
		Description:      "Main warehouse in California",
		ValidFrom:        now,
		ValidTo:          nil,
		IsActive:         true,
		OrgID:            1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		2, request.OrgID, request.Name, request.Identifier, request.ParentLocationID,
		"usa.warehouse_1", 2, request.Description, request.ValidFrom, request.ValidTo,
		request.IsActive, now, now, nil,
	)

	mock.ExpectQuery(`INSERT INTO trakrf.locations`).
		WithArgs(
			request.Name, request.Identifier, request.ParentLocationID,
			request.Description, request.ValidFrom, request.ValidTo,
			request.IsActive, request.OrgID,
		).
		WillReturnRows(rows)

	result, err := storage.CreateLocation(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.ID)
	assert.Equal(t, request.Name, result.Name)
	assert.Equal(t, request.Identifier, result.Identifier)
	assert.Equal(t, "usa.warehouse_1", result.Path)
	assert.Equal(t, 2, result.Depth)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateLocation_RootLocation(t *testing.T) {
	storage, mock := setupLocationTest(t)

	now := time.Now()
	request := location.Location{
		Name:             "USA",
		Identifier:       "usa",
		ParentLocationID: nil,
		Description:      "United States region",
		ValidFrom:        now,
		ValidTo:          nil,
		IsActive:         true,
		OrgID:            1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		1, request.OrgID, request.Name, request.Identifier, nil,
		"usa", 1, request.Description, request.ValidFrom, request.ValidTo,
		request.IsActive, now, now, nil,
	)

	mock.ExpectQuery(`INSERT INTO trakrf.locations`).
		WithArgs(
			request.Name, request.Identifier, request.ParentLocationID,
			request.Description, request.ValidFrom, request.ValidTo,
			request.IsActive, request.OrgID,
		).
		WillReturnRows(rows)

	result, err := storage.CreateLocation(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, "usa", result.Path)
	assert.Equal(t, 1, result.Depth)
	assert.Nil(t, result.ParentLocationID)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateLocation_DuplicateIdentifier(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := location.Location{
		Name:             "Warehouse 1",
		Identifier:       "warehouse_1",
		ParentLocationID: nil,
		Description:      "Test",
		ValidFrom:        now,
		ValidTo:          nil,
		IsActive:         true,
		OrgID:            1,
	}

	mock.ExpectQuery(`INSERT INTO trakrf.locations`).
		WithArgs(
			request.Name, request.Identifier, request.ParentLocationID,
			request.Description, request.ValidFrom, request.ValidTo,
			request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("ERROR: duplicate key value violates unique constraint"))

	result, err := storage.CreateLocation(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "location with identifier warehouse_1 already exists")
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
		Name:             "Child Location",
		Identifier:       "child",
		ParentLocationID: &parentID,
		Description:      "Test",
		ValidFrom:        now,
		ValidTo:          nil,
		IsActive:         true,
		OrgID:            1,
	}

	mock.ExpectQuery(`INSERT INTO trakrf.locations`).
		WithArgs(
			request.Name, request.Identifier, request.ParentLocationID,
			request.Description, request.ValidFrom, request.ValidTo,
			request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("ERROR: insert or update on table \"locations\" violates foreign key constraint"))

	result, err := storage.CreateLocation(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid parent location ID or organization ID")
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

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		locationID, 1, newName, "warehouse_1", nil, "warehouse_1", 1,
		newDescription, now, nil, true, now, now, nil,
	)

	mock.ExpectQuery(`UPDATE trakrf.locations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(rows)

	result, err := storage.UpdateLocation(context.Background(), locationID, request)

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
		ParentLocationID: &newParentID,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		locationID, 1, "Zone A", "zone_a", &newParentID,
		"usa.california.zone_a", 3, "Test zone", now, nil, true,
		now, now, nil,
	)

	mock.ExpectQuery(`UPDATE trakrf.locations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(rows)

	result, err := storage.UpdateLocation(context.Background(), locationID, request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, locationID, result.ID)
	assert.Equal(t, &newParentID, result.ParentLocationID)
	assert.Equal(t, "usa.california.zone_a", result.Path)
	assert.Equal(t, 3, result.Depth)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateLocation_NoFields(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 1
	request := location.UpdateLocationRequest{}

	result, err := storage.UpdateLocation(context.Background(), locationID, request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no fields to update")
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

	mock.ExpectQuery(`UPDATE trakrf.locations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("no rows in result set"))

	result, err := storage.UpdateLocation(context.Background(), locationID, request)

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
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		locationID, 1, "USA", "usa", nil, "usa", 1,
		"United States", now, nil, true, now, now, nil,
	)

	mock.ExpectQuery(`SELECT id, org_id, name, identifier`).
		WithArgs(locationID).
		WillReturnRows(rows)

	result, err := storage.GetLocationByID(context.Background(), locationID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, locationID, result.ID)
	assert.Equal(t, "USA", result.Name)
	assert.Equal(t, "usa", result.Path)
	assert.Equal(t, 1, result.Depth)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLocationByID_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 99999

	mock.ExpectQuery(`SELECT id, org_id, name, identifier`).
		WithArgs(locationID).
		WillReturnError(errors.New("no rows in result set"))

	result, err := storage.GetLocationByID(context.Background(), locationID)

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
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).
		AddRow(1, orgID, "USA", "usa", nil, "usa", 1, "United States", now, nil, true, now, &now, nil).
		AddRow(2, orgID, "California", "california", &parent1, "usa.california", 2, "California State", now, nil, true, now, &now, nil).
		AddRow(3, orgID, "Warehouse 1", "warehouse_1", &parent2, "usa.california.warehouse_1", 3, "Main Warehouse", now, nil, true, now, &now, nil)

	mock.ExpectQuery(`SELECT id, org_id, name, identifier`).
		WithArgs(orgID, limit, offset).
		WillReturnRows(rows)

	results, err := storage.ListAllLocations(context.Background(), orgID, limit, offset)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 3)
	assert.Equal(t, "usa", results[0].Path)
	assert.Equal(t, "usa.california", results[1].Path)
	assert.Equal(t, "usa.california.warehouse_1", results[2].Path)
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
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	})

	mock.ExpectQuery(`SELECT id, org_id, name, identifier`).
		WithArgs(orgID, limit, offset).
		WillReturnRows(rows)

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

	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs(orgID).
		WillReturnRows(rows)

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

	mock.ExpectExec(`UPDATE trakrf.locations SET deleted_at`).
		WithArgs(locationID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	result, err := storage.DeleteLocation(context.Background(), locationID)

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

	mock.ExpectExec(`UPDATE trakrf.locations SET deleted_at`).
		WithArgs(locationID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	result, err := storage.DeleteLocation(context.Background(), locationID)

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
	locationID := 3 // usa.california.warehouse_1

	parent1 := 1
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).
		AddRow(1, 1, "USA", "usa", nil, "usa", 1, "United States", now, nil, true, now, &now, nil).
		AddRow(2, 1, "California", "california", &parent1, "usa.california", 2, "California State", now, nil, true, now, &now, nil)

	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.identifier`).
		WithArgs(locationID).
		WillReturnRows(rows)

	results, err := storage.GetAncestors(context.Background(), locationID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 2)
	assert.Equal(t, "usa", results[0].Path)
	assert.Equal(t, "usa.california", results[1].Path)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAncestors_RootLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 1 // Root location - no ancestors

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	})

	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.identifier`).
		WithArgs(locationID).
		WillReturnRows(rows)

	results, err := storage.GetAncestors(context.Background(), locationID)

	assert.NoError(t, err)
	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDescendants(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	locationID := 1 // usa

	parent1 := 1
	parent2 := 2
	parent3 := 3
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).
		AddRow(2, 1, "California", "california", &parent1, "usa.california", 2, "California State", now, nil, true, now, &now, nil).
		AddRow(3, 1, "Warehouse 1", "warehouse_1", &parent2, "usa.california.warehouse_1", 3, "Main Warehouse", now, nil, true, now, &now, nil).
		AddRow(4, 1, "Zone A", "zone_a", &parent3, "usa.california.warehouse_1.zone_a", 4, "Storage Zone A", now, nil, true, now, &now, nil)

	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.identifier`).
		WithArgs(locationID).
		WillReturnRows(rows)

	results, err := storage.GetDescendants(context.Background(), locationID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 3)
	assert.Equal(t, "usa.california", results[0].Path)
	assert.Equal(t, "usa.california.warehouse_1", results[1].Path)
	assert.Equal(t, "usa.california.warehouse_1.zone_a", results[2].Path)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDescendants_LeafLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 10 // Leaf location - no descendants

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	})

	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.identifier`).
		WithArgs(locationID).
		WillReturnRows(rows)

	results, err := storage.GetDescendants(context.Background(), locationID)

	assert.NoError(t, err)
	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetChildren(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	parentID := 2 // usa.california

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).
		AddRow(3, 1, "Warehouse 1", "warehouse_1", &parentID, "usa.california.warehouse_1", 3, "Main Warehouse", now, nil, true, now, &now, nil).
		AddRow(4, 1, "Warehouse 2", "warehouse_2", &parentID, "usa.california.warehouse_2", 3, "Secondary Warehouse", now, nil, true, now, &now, nil)

	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.identifier`).
		WithArgs(parentID).
		WillReturnRows(rows)

	results, err := storage.GetChildren(context.Background(), parentID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 2)
	assert.Equal(t, 3, results[0].Depth)
	assert.Equal(t, 3, results[1].Depth)
	assert.Equal(t, "Warehouse 1", results[0].Name)
	assert.Equal(t, "Warehouse 2", results[1].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetChildren_NoChildren(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	parentID := 10 // Location with no children

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
	})

	mock.ExpectQuery(`SELECT l.id, l.org_id, l.name, l.identifier`).
		WithArgs(parentID).
		WillReturnRows(rows)

	results, err := storage.GetChildren(context.Background(), parentID)

	assert.NoError(t, err)
	assert.Empty(t, results)
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
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at", "relation_type",
	}).
		// Target location
		AddRow(3, 1, "Warehouse 1", "warehouse_1", &parent2, "usa.california.warehouse_1", 3,
			"Main Warehouse", now, nil, true, now, &now, nil, "target").
		// Ancestors (ordered by depth)
		AddRow(1, 1, "USA", "usa", nil, "usa", 1,
			"United States", now, nil, true, now, &now, nil, "ancestor").
		AddRow(2, 1, "California", "california", &parent1, "usa.california", 2,
			"California State", now, nil, true, now, &now, nil, "ancestor").
		// Children (immediate only)
		AddRow(4, 1, "Zone A", "zone_a", &targetID, "usa.california.warehouse_1.zone_a", 4,
			"Storage Zone A", now, nil, true, now, &now, nil, "child").
		AddRow(5, 1, "Zone B", "zone_b", &targetID, "usa.california.warehouse_1.zone_b", 4,
			"Storage Zone B", now, nil, true, now, &now, nil, "child")

	mock.ExpectQuery(`WITH target AS`).
		WithArgs(targetID).
		WillReturnRows(rows)

	result, err := storage.GetLocationWithRelations(context.Background(), targetID)

	assert.NoError(t, err)
	require.NotNil(t, result)

	// Verify target location
	assert.Equal(t, targetID, result.ID)
	assert.Equal(t, "Warehouse 1", result.Name)
	assert.Equal(t, "usa.california.warehouse_1", result.Path)

	// Verify ancestors (should have 2: USA and California)
	require.Len(t, result.Ancestors, 2)
	assert.Equal(t, "USA", result.Ancestors[0].Name)
	assert.Equal(t, "usa", result.Ancestors[0].Path)
	assert.Equal(t, "California", result.Ancestors[1].Name)
	assert.Equal(t, "usa.california", result.Ancestors[1].Path)

	// Verify children (should have 2: Zone A and Zone B)
	require.Len(t, result.Children, 2)
	assert.Equal(t, "Zone A", result.Children[0].Name)
	assert.Equal(t, "usa.california.warehouse_1.zone_a", result.Children[0].Path)
	assert.Equal(t, "Zone B", result.Children[1].Name)
	assert.Equal(t, "usa.california.warehouse_1.zone_b", result.Children[1].Path)

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
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at", "relation_type",
	}).
		AddRow(1, 1, "USA", "usa", nil, "usa", 1,
			"United States", now, nil, true, now, &now, nil, "target").
		AddRow(2, 1, "California", "california", &rootID, "usa.california", 2,
			"California State", now, nil, true, now, &now, nil, "child").
		AddRow(3, 1, "Texas", "texas", &rootID, "usa.texas", 2,
			"Texas State", now, nil, true, now, &now, nil, "child")

	mock.ExpectQuery(`WITH target AS`).
		WithArgs(rootID).
		WillReturnRows(rows)

	result, err := storage.GetLocationWithRelations(context.Background(), rootID)

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
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at", "relation_type",
	}).
		AddRow(4, 1, "Zone A", "zone_a", &parent3, "usa.california.warehouse_1.zone_a", 4,
			"Storage Zone A", now, nil, true, now, &now, nil, "target").
		AddRow(1, 1, "USA", "usa", nil, "usa", 1,
			"United States", now, nil, true, now, &now, nil, "ancestor").
		AddRow(2, 1, "California", "california", &parent1, "usa.california", 2,
			"California State", now, nil, true, now, &now, nil, "ancestor").
		AddRow(3, 1, "Warehouse 1", "warehouse_1", &parent2, "usa.california.warehouse_1", 3,
			"Main Warehouse", now, nil, true, now, &now, nil, "ancestor")

	mock.ExpectQuery(`WITH target AS`).
		WithArgs(leafID).
		WillReturnRows(rows)

	result, err := storage.GetLocationWithRelations(context.Background(), leafID)

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
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at", "relation_type",
	})

	mock.ExpectQuery(`WITH target AS`).
		WithArgs(locationID).
		WillReturnRows(rows)

	result, err := storage.GetLocationWithRelations(context.Background(), locationID)

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}
