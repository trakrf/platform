package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/asset"
)

func TestCreateAsset(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    []byte(`{"key":"value"}`),
		IsActive:    true,
		OrgID:       1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		1, request.OrgID, request.Identifier, request.Name,
		request.Type, request.Description, request.ValidFrom, request.ValidTo,
		request.Metadata, request.IsActive, now, now, nil,
	)

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnRows(rows)

	result, err := storage.CreateAsset(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, request.Name, result.Name)
	assert.Equal(t, request.Identifier, result.Identifier)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAsset_DuplicateIdentifier(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    []byte(`{"key":"value"}`),
		IsActive:    true,
		OrgID:       1,
	}

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("ERROR: duplicate key value violates unique constraint"))

	result, err := storage.CreateAsset(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "asset with identifier TEST-001 already exists")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAsset_EmptyName(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    []byte(`{"key":"value"}`),
		IsActive:    true,
		OrgID:       1,
	}

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("ERROR: null value in column \"name\" violates not-null constraint"))

	result, err := storage.CreateAsset(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create asset")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAsset_EmptyIdentifier(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    []byte(`{"key":"value"}`),
		IsActive:    true,
		OrgID:       1,
	}

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("ERROR: null value in column \"identifier\" violates not-null constraint"))

	result, err := storage.CreateAsset(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create asset")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAsset_InvalidOrgID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    []byte(`{"key":"value"}`),
		IsActive:    true,
		OrgID:       99999,
	}

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("ERROR: insert or update on table \"assets\" violates foreign key constraint"))

	result, err := storage.CreateAsset(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create asset")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAsset_NullMetadata(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    nil,
		IsActive:    true,
		OrgID:       1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		1, request.OrgID, request.Identifier, request.Name,
		request.Type, request.Description, request.ValidFrom, request.ValidTo,
		nil, request.IsActive, now, now, nil,
	)

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnRows(rows)

	result, err := storage.CreateAsset(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ID)
	assert.Nil(t, result.Metadata)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAsset_EmptyMetadata(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    []byte(`{}`),
		IsActive:    true,
		OrgID:       1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		1, request.OrgID, request.Identifier, request.Name,
		request.Type, request.Description, request.ValidFrom, request.ValidTo,
		request.Metadata, request.IsActive, now, now, nil,
	)

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnRows(rows)

	result, err := storage.CreateAsset(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ID)

	metadataBytes, ok := result.Metadata.([]byte)
	require.True(t, ok, "Metadata should be []byte")
	assert.Equal(t, []byte(`{}`), metadataBytes)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAsset_DatabaseConnectionError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    []byte(`{"key":"value"}`),
		IsActive:    true,
		OrgID:       1,
	}

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnError(errors.New("connection refused"))

	result, err := storage.CreateAsset(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create asset")
	assert.Contains(t, err.Error(), "connection refused")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAsset_ComplexMetadata(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	complexMetadata := []byte(`{
		"manufacturer": "Acme Corp",
		"model": "X1000",
		"serial": "ABC123",
		"specifications": {
			"weight": 150.5,
			"dimensions": {
				"length": 100,
				"width": 50,
				"height": 75
			}
		},
		"features": ["GPS", "Bluetooth", "WiFi"]
	}`)

	request := asset.Asset{
		Name:        "Advanced Equipment",
		Identifier:  "ADV-001",
		Type:        "equipment",
		Description: "Equipment with complex metadata",
		ValidFrom:   now,
		ValidTo:     now.Add(365 * 24 * time.Hour),
		Metadata:    complexMetadata,
		IsActive:    true,
		OrgID:       1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		1, request.OrgID, request.Identifier, request.Name,
		request.Type, request.Description, request.ValidFrom, request.ValidTo,
		request.Metadata, request.IsActive, now, now, nil,
	)

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.OrgID,
		).
		WillReturnRows(rows)

	result, err := storage.CreateAsset(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, "Advanced Equipment", result.Name)

	metadataBytes, ok := result.Metadata.([]byte)
	require.True(t, ok, "Metadata should be []byte")
	assert.JSONEq(t, string(complexMetadata), string(metadataBytes))

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateAsset(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	assetID := 1
	newName := "Updated Asset Name"
	newDescription := "Updated description"

	request := asset.UpdateAssetRequest{
		Name:        &newName,
		Description: &newDescription,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		assetID, 1, "TEST-001", newName, "equipment", newDescription,
		now, now.Add(24*time.Hour), []byte(`{"key":"value"}`), true,
		now, now, nil,
	)

	mock.ExpectQuery(`update trakrf.assets`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(rows)

	result, err := storage.UpdateAsset(context.Background(), assetID, request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, assetID, result.ID)
	assert.Equal(t, newName, result.Name)
	assert.Equal(t, newDescription, result.Description)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateAsset_NoFields(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 1
	request := asset.UpdateAssetRequest{}

	result, err := storage.UpdateAsset(context.Background(), assetID, request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no fields to update")
}

func TestUpdateAsset_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 99999
	newName := "Updated Asset Name"
	request := asset.UpdateAssetRequest{
		Name: &newName,
	}

	mock.ExpectQuery(`update trakrf.assets`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("no rows in result set"))

	result, err := storage.UpdateAsset(context.Background(), assetID, request)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to update asset")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateAsset_PartialUpdate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	assetID := 1
	isActive := false

	request := asset.UpdateAssetRequest{
		IsActive: &isActive,
	}

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		assetID, 1, "TEST-001", "Test Asset", "equipment", "Test description",
		now, now.Add(24*time.Hour), []byte(`{"key":"value"}`), false,
		now, now, nil,
	)

	mock.ExpectQuery(`update trakrf.assets`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(rows)

	result, err := storage.UpdateAsset(context.Background(), assetID, request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, assetID, result.ID)
	assert.False(t, result.IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAssetByID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	assetID := 1

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		assetID, 1, "TEST-001", "Test Asset", "equipment", "Test description",
		now, now.Add(24*time.Hour), []byte(`{"key":"value"}`), true,
		now, now, nil,
	)

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(&assetID).
		WillReturnRows(rows)

	result, err := storage.GetAssetByID(context.Background(), &assetID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, assetID, result.ID)
	assert.Equal(t, "TEST-001", result.Identifier)
	assert.Equal(t, "Test Asset", result.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAssetByID_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 99999

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(&assetID).
		WillReturnError(errors.New("no rows in result set"))

	result, err := storage.GetAssetByID(context.Background(), &assetID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get asset by id")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAssetByID_WithNullMetadata(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	assetID := 1

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		assetID, 1, "TEST-001", "Test Asset", "equipment", "Test description",
		now, now.Add(24*time.Hour), nil, true,
		now, now, nil,
	)

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(&assetID).
		WillReturnRows(rows)

	result, err := storage.GetAssetByID(context.Background(), &assetID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, assetID, result.ID)
	assert.Nil(t, result.Metadata)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAssetByID_DatabaseError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 1

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(&assetID).
		WillReturnError(errors.New("connection timeout"))

	result, err := storage.GetAssetByID(context.Background(), &assetID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get asset by id")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListAllAssets(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	limit := 10
	offset := 0

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).
		AddRow(1, 1, "TEST-001", "Asset 1", "equipment", "Description 1",
			now, now.Add(24*time.Hour), []byte(`{"key":"value1"}`), true,
			now, now, nil).
		AddRow(2, 1, "TEST-002", "Asset 2", "device", "Description 2",
			now, now.Add(24*time.Hour), []byte(`{"key":"value2"}`), true,
			now, now, nil).
		AddRow(3, 2, "TEST-003", "Asset 3", "equipment", "Description 3",
			now, now.Add(24*time.Hour), nil, false,
			now, now, nil)

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(limit, offset).
		WillReturnRows(rows)

	results, err := storage.ListAllAssets(context.Background(), limit, offset)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 3)
	assert.Equal(t, "TEST-001", results[0].Identifier)
	assert.Equal(t, "TEST-002", results[1].Identifier)
	assert.Equal(t, "TEST-003", results[2].Identifier)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListAllAssets_Empty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	limit := 10
	offset := 0

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	})

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(limit, offset).
		WillReturnRows(rows)

	results, err := storage.ListAllAssets(context.Background(), limit, offset)

	assert.NoError(t, err)
	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListAllAssets_WithPagination(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	limit := 2
	offset := 5

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).
		AddRow(6, 1, "TEST-006", "Asset 6", "equipment", "Description 6",
			now, now.Add(24*time.Hour), []byte(`{}`), true,
			now, now, nil).
		AddRow(7, 1, "TEST-007", "Asset 7", "device", "Description 7",
			now, now.Add(24*time.Hour), []byte(`{}`), true,
			now, now, nil)

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(limit, offset).
		WillReturnRows(rows)

	results, err := storage.ListAllAssets(context.Background(), limit, offset)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 2)
	assert.Equal(t, 6, results[0].ID)
	assert.Equal(t, 7, results[1].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListAllAssets_DatabaseError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	limit := 10
	offset := 0

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(limit, offset).
		WillReturnError(errors.New("connection lost"))

	results, err := storage.ListAllAssets(context.Background(), limit, offset)

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "failed to list assets")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListAllAssets_ScanError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	limit := 10
	offset := 0

	rows := pgxmock.NewRows([]string{
		"id", "org_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		"not-an-int", 1, "TEST-001", "Asset 1", "equipment", "Description",
		now, now.Add(24*time.Hour), []byte(`{}`), true,
		now, now, nil,
	).RowError(0, errors.New("scan error: invalid type"))

	mock.ExpectQuery(`select id, org_id, identifier, name, type, description`).
		WithArgs(limit, offset).
		WillReturnRows(rows)

	results, err := storage.ListAllAssets(context.Background(), limit, offset)

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "failed to scan asset")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteAsset(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 1

	mock.ExpectExec(`update trakrf.assets set deleted_at = now()`).
		WithArgs(&assetID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	result, err := storage.DeleteAsset(context.Background(), &assetID)

	assert.NoError(t, err)
	assert.True(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteAsset_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 99999

	mock.ExpectExec(`update trakrf.assets set deleted_at = now()`).
		WithArgs(&assetID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	result, err := storage.DeleteAsset(context.Background(), &assetID)

	assert.NoError(t, err)
	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteAsset_AlreadyDeleted(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 1

	mock.ExpectExec(`update trakrf.assets set deleted_at = now()`).
		WithArgs(&assetID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	result, err := storage.DeleteAsset(context.Background(), &assetID)

	assert.NoError(t, err)
	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteAsset_DatabaseError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 1

	mock.ExpectExec(`update trakrf.assets set deleted_at = now()`).
		WithArgs(&assetID).
		WillReturnError(errors.New("database connection lost"))

	result, err := storage.DeleteAsset(context.Background(), &assetID)

	assert.Error(t, err)
	assert.False(t, result)
	assert.Contains(t, err.Error(), "could not delete asset")
	assert.NoError(t, mock.ExpectationsWereMet())
}
