package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

func TestGetIdentifiersByAssetID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 1

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(101, "rfid", "E20000001234", true).
		AddRow(102, "ble", "AA:BB:CC:DD:EE:FF", true)

	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(assetID).
		WillReturnRows(rows)

	results, err := storage.GetIdentifiersByAssetID(context.Background(), assetID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 2)
	assert.Equal(t, "rfid", results[0].Type)
	assert.Equal(t, "E20000001234", results[0].Value)
	assert.Equal(t, "ble", results[1].Type)
	assert.Equal(t, "AA:BB:CC:DD:EE:FF", results[1].Value)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentifiersByAssetID_Empty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 1

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"})

	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(assetID).
		WillReturnRows(rows)

	results, err := storage.GetIdentifiersByAssetID(context.Background(), assetID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 0) // Empty slice, not nil
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentifiersByAssetID_DatabaseError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetID := 1

	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(assetID).
		WillReturnError(errors.New("connection lost"))

	results, err := storage.GetIdentifiersByAssetID(context.Background(), assetID)

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "failed to get identifiers for asset")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentifiersByLocationID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 1

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(201, "barcode", "LOC-001", true)

	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(locationID).
		WillReturnRows(rows)

	results, err := storage.GetIdentifiersByLocationID(context.Background(), locationID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 1)
	assert.Equal(t, "barcode", results[0].Type)
	assert.Equal(t, "LOC-001", results[0].Value)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentifiersByLocationID_Empty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationID := 1

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"})

	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(locationID).
		WillReturnRows(rows)

	results, err := storage.GetIdentifiersByLocationID(context.Background(), locationID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 0) // Empty slice, not nil
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAddIdentifierToAsset(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 10
	req := shared.TagIdentifierRequest{
		Type:  "rfid",
		Value: "E20000009999",
	}

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(301, "rfid", "E20000009999", true)

	// Expect transaction flow for RLS context
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.identifiers`).
		WithArgs(orgID, req.Type, req.Value, assetID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.AddIdentifierToAsset(context.Background(), orgID, assetID, req)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 301, result.ID)
	assert.Equal(t, "rfid", result.Type)
	assert.Equal(t, "E20000009999", result.Value)
	assert.True(t, result.IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAddIdentifierToAsset_Duplicate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 10
	req := shared.TagIdentifierRequest{
		Type:  "rfid",
		Value: "E20000009999",
	}

	// Expect transaction flow for RLS context - with rollback on error
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.identifiers`).
		WithArgs(orgID, req.Type, req.Value, assetID).
		WillReturnError(errors.New("duplicate key value violates unique constraint"))
	mock.ExpectRollback()

	result, err := storage.AddIdentifierToAsset(context.Background(), orgID, assetID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "already exists")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAddIdentifierToLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 20
	req := shared.TagIdentifierRequest{
		Type:  "barcode",
		Value: "LOC-SHELF-01",
	}

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(401, "barcode", "LOC-SHELF-01", true)

	// Expect transaction flow for RLS context
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.identifiers`).
		WithArgs(orgID, req.Type, req.Value, locationID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.AddIdentifierToLocation(context.Background(), orgID, locationID, req)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 401, result.ID)
	assert.Equal(t, "barcode", result.Type)
	assert.Equal(t, "LOC-SHELF-01", result.Value)
	assert.True(t, result.IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveIdentifier(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	identifierID := 101

	mock.ExpectExec(`UPDATE trakrf.identifiers SET deleted_at = NOW()`).
		WithArgs(identifierID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	result, err := storage.RemoveIdentifier(context.Background(), identifierID)

	assert.NoError(t, err)
	assert.True(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveIdentifier_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	identifierID := 99999

	mock.ExpectExec(`UPDATE trakrf.identifiers SET deleted_at = NOW()`).
		WithArgs(identifierID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	result, err := storage.RemoveIdentifier(context.Background(), identifierID)

	assert.NoError(t, err)
	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveIdentifier_DatabaseError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	identifierID := 101

	mock.ExpectExec(`UPDATE trakrf.identifiers SET deleted_at = NOW()`).
		WithArgs(identifierID).
		WillReturnError(errors.New("database error"))

	result, err := storage.RemoveIdentifier(context.Background(), identifierID)

	assert.Error(t, err)
	assert.False(t, result)
	assert.Contains(t, err.Error(), "failed to remove identifier")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentifierByID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	identifierID := 101

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(101, "rfid", "E20000001234", true)

	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(identifierID).
		WillReturnRows(rows)

	result, err := storage.GetIdentifierByID(context.Background(), identifierID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 101, result.ID)
	assert.Equal(t, "rfid", result.Type)
	assert.Equal(t, "E20000001234", result.Value)
	assert.True(t, result.IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentifierByID_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	identifierID := 99999

	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(identifierID).
		WillReturnError(errors.New("no rows in result set"))

	result, err := storage.GetIdentifierByID(context.Background(), identifierID)

	assert.NoError(t, err) // Not found is not an error, returns nil
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentifiersForAssets_Batch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	assetIDs := []int{1, 2, 3}

	rows := pgxmock.NewRows([]string{"asset_id", "id", "type", "value", "is_active"}).
		AddRow(1, 101, "rfid", "E20000001111", true).
		AddRow(1, 102, "ble", "AA:AA:AA:AA:AA:AA", true).
		AddRow(2, 201, "barcode", "BC-002", true)
	// Note: asset 3 has no identifiers

	mock.ExpectQuery(`SELECT asset_id, id, type, value, is_active`).
		WithArgs(assetIDs).
		WillReturnRows(rows)

	result, err := storage.getIdentifiersForAssets(context.Background(), assetIDs)

	assert.NoError(t, err)
	require.NotNil(t, result)

	// Asset 1 has 2 identifiers
	assert.Len(t, result[1], 2)
	assert.Equal(t, "rfid", result[1][0].Type)
	assert.Equal(t, "ble", result[1][1].Type)

	// Asset 2 has 1 identifier
	assert.Len(t, result[2], 1)
	assert.Equal(t, "barcode", result[2][0].Type)

	// Asset 3 has empty slice (not nil)
	assert.Len(t, result[3], 0)
	assert.NotNil(t, result[3])

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentifiersForLocations_Batch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	locationIDs := []int{10, 20}

	rows := pgxmock.NewRows([]string{"location_id", "id", "type", "value", "is_active"}).
		AddRow(10, 1001, "barcode", "LOC-10", true).
		AddRow(20, 2001, "rfid", "E20000020001", true).
		AddRow(20, 2002, "barcode", "LOC-20", true)

	mock.ExpectQuery(`SELECT location_id, id, type, value, is_active`).
		WithArgs(locationIDs).
		WillReturnRows(rows)

	result, err := storage.getIdentifiersForLocations(context.Background(), locationIDs)

	assert.NoError(t, err)
	require.NotNil(t, result)

	// Location 10 has 1 identifier
	assert.Len(t, result[10], 1)
	assert.Equal(t, "barcode", result[10][0].Type)

	// Location 20 has 2 identifiers
	assert.Len(t, result[20], 2)
	assert.Equal(t, "rfid", result[20][0].Type)
	assert.Equal(t, "barcode", result[20][1].Type)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestIdentifiersToJSON(t *testing.T) {
	t.Run("empty slice returns empty array", func(t *testing.T) {
		result, err := identifiersToJSON([]shared.TagIdentifierRequest{})
		assert.NoError(t, err)
		assert.Equal(t, "[]", string(result))
	})

	t.Run("nil slice returns empty array", func(t *testing.T) {
		result, err := identifiersToJSON(nil)
		assert.NoError(t, err)
		assert.Equal(t, "[]", string(result))
	})

	t.Run("single identifier", func(t *testing.T) {
		input := []shared.TagIdentifierRequest{
			{Type: "rfid", Value: "E20000001234"},
		}
		result, err := identifiersToJSON(input)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"rfid","value":"E20000001234"}]`, string(result))
	})

	t.Run("multiple identifiers", func(t *testing.T) {
		input := []shared.TagIdentifierRequest{
			{Type: "rfid", Value: "E20000001234"},
			{Type: "ble", Value: "AA:BB:CC:DD:EE:FF"},
		}
		result, err := identifiersToJSON(input)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"rfid","value":"E20000001234"},{"type":"ble","value":"AA:BB:CC:DD:EE:FF"}]`, string(result))
	})

	t.Run("applies default type when empty", func(t *testing.T) {
		input := []shared.TagIdentifierRequest{
			{Value: "E20000001234"}, // no type specified
		}
		result, err := identifiersToJSON(input)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"rfid","value":"E20000001234"}]`, string(result)) // defaults to rfid
	})

	t.Run("mixed explicit and default types", func(t *testing.T) {
		input := []shared.TagIdentifierRequest{
			{Type: "ble", Value: "AA:BB:CC:DD:EE:FF"},
			{Value: "E20000001234"}, // no type, defaults to rfid
		}
		result, err := identifiersToJSON(input)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"ble","value":"AA:BB:CC:DD:EE:FF"},{"type":"rfid","value":"E20000001234"}]`, string(result))
	})
}

func TestTagIdentifierRequestGetType(t *testing.T) {
	t.Run("returns explicit type", func(t *testing.T) {
		req := shared.TagIdentifierRequest{Type: "ble", Value: "test"}
		assert.Equal(t, "ble", req.GetType())
	})

	t.Run("returns default rfid when empty", func(t *testing.T) {
		req := shared.TagIdentifierRequest{Value: "test"}
		assert.Equal(t, "rfid", req.GetType())
	})
}

func TestParseIdentifierError(t *testing.T) {
	t.Run("duplicate key error", func(t *testing.T) {
		err := parseIdentifierError(errors.New("duplicate key value violates unique constraint"), "rfid", "E20000001234")
		assert.Contains(t, err.Error(), "rfid:E20000001234 already exists")
	})

	t.Run("generic error", func(t *testing.T) {
		err := parseIdentifierError(errors.New("connection lost"), "rfid", "E20000001234")
		assert.Contains(t, err.Error(), "failed to create identifier")
		assert.Contains(t, err.Error(), "connection lost")
	})
}
