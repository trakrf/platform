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

func TestGetTagsByAssetID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 1

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(101, "rfid", "E20000001234", true).
		AddRow(102, "ble", "AA:BB:CC:DD:EE:FF", true)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(assetID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.GetTagsByAssetID(context.Background(), orgID, assetID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 2)
	assert.Equal(t, "rfid", results[0].TagType)
	assert.Equal(t, "E20000001234", results[0].Value)
	assert.Equal(t, "ble", results[1].TagType)
	assert.Equal(t, "AA:BB:CC:DD:EE:FF", results[1].Value)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTagsByAssetID_Empty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 1

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"})

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(assetID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.GetTagsByAssetID(context.Background(), orgID, assetID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 0) // Empty slice, not nil
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTagsByAssetID_DatabaseError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 1

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(assetID, orgID).
		WillReturnError(errors.New("connection lost"))
	mock.ExpectRollback()

	results, err := storage.GetTagsByAssetID(context.Background(), orgID, assetID)

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "failed to get tags for asset")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTagsByLocationID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 1

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(201, "barcode", "LOC-001", true)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(locationID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.GetTagsByLocationID(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 1)
	assert.Equal(t, "barcode", results[0].TagType)
	assert.Equal(t, "LOC-001", results[0].Value)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTagsByLocationID_Empty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 1

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"})

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(locationID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	results, err := storage.GetTagsByLocationID(context.Background(), orgID, locationID)

	assert.NoError(t, err)
	require.NotNil(t, results)
	assert.Len(t, results, 0) // Empty slice, not nil
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAddTagToAsset(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 10
	req := shared.TagIdentifierRequest{
		TagType: "rfid",
		Value:   "E20000009999",
	}

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(301, "rfid", "E20000009999", true)

	// Expect transaction flow for RLS context
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.tags`).
		WithArgs(orgID, req.GetType(), req.Value, assetID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.AddTagToAsset(context.Background(), orgID, assetID, req)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 301, result.SurrogateID)
	assert.Equal(t, "rfid", result.TagType)
	assert.Equal(t, "E20000009999", result.Value)
	assert.True(t, result.IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAddTagToAsset_Duplicate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 10
	req := shared.TagIdentifierRequest{
		TagType: "rfid",
		Value:   "E20000009999",
	}

	// Expect transaction flow for RLS context - with rollback on error
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.tags`).
		WithArgs(orgID, req.GetType(), req.Value, assetID).
		WillReturnError(errors.New("duplicate key value violates unique constraint"))
	mock.ExpectRollback()

	result, err := storage.AddTagToAsset(context.Background(), orgID, assetID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "already exists")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAddTagToLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 20
	req := shared.TagIdentifierRequest{
		TagType: "barcode",
		Value:   "LOC-SHELF-01",
	}

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(401, "barcode", "LOC-SHELF-01", true)

	// Expect transaction flow for RLS context
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`INSERT INTO trakrf.tags`).
		WithArgs(orgID, req.GetType(), req.Value, locationID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.AddTagToLocation(context.Background(), orgID, locationID, req)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 401, result.SurrogateID)
	assert.Equal(t, "barcode", result.TagType)
	assert.Equal(t, "LOC-SHELF-01", result.Value)
	assert.True(t, result.IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveAssetTag(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 42
	tagID := 101

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectExec(`UPDATE trakrf.tags\s+SET deleted_at = NOW\(\)`).
		WithArgs(tagID, assetID, orgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	result, err := storage.RemoveAssetTag(context.Background(), orgID, assetID, tagID)

	assert.NoError(t, err)
	assert.True(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveAssetTag_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 42
	tagID := 99999

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectExec(`UPDATE trakrf.tags\s+SET deleted_at = NOW\(\)`).
		WithArgs(tagID, assetID, orgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectCommit()

	result, err := storage.RemoveAssetTag(context.Background(), orgID, assetID, tagID)

	assert.NoError(t, err)
	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveAssetTag_DatabaseError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetID := 42
	tagID := 101

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectExec(`UPDATE trakrf.tags\s+SET deleted_at = NOW\(\)`).
		WithArgs(tagID, assetID, orgID).
		WillReturnError(errors.New("database error"))
	mock.ExpectRollback()

	result, err := storage.RemoveAssetTag(context.Background(), orgID, assetID, tagID)

	assert.Error(t, err)
	assert.False(t, result)
	assert.Contains(t, err.Error(), "failed to remove asset tag")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveLocationTag(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 77
	tagID := 201

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectExec(`UPDATE trakrf.tags\s+SET deleted_at = NOW\(\)`).
		WithArgs(tagID, locationID, orgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	result, err := storage.RemoveLocationTag(context.Background(), orgID, locationID, tagID)

	assert.NoError(t, err)
	assert.True(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveLocationTag_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 77
	tagID := 99999

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectExec(`UPDATE trakrf.tags\s+SET deleted_at = NOW\(\)`).
		WithArgs(tagID, locationID, orgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectCommit()

	result, err := storage.RemoveLocationTag(context.Background(), orgID, locationID, tagID)

	assert.NoError(t, err)
	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveLocationTag_DatabaseError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationID := 77
	tagID := 201

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectExec(`UPDATE trakrf.tags\s+SET deleted_at = NOW\(\)`).
		WithArgs(tagID, locationID, orgID).
		WillReturnError(errors.New("database error"))
	mock.ExpectRollback()

	result, err := storage.RemoveLocationTag(context.Background(), orgID, locationID, tagID)

	assert.Error(t, err)
	assert.False(t, result)
	assert.Contains(t, err.Error(), "failed to remove location tag")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTagByID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	tagID := 101

	rows := pgxmock.NewRows([]string{"id", "type", "value", "is_active"}).
		AddRow(101, "rfid", "E20000001234", true)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(tagID, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.GetTagByID(context.Background(), orgID, tagID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 101, result.SurrogateID)
	assert.Equal(t, "rfid", result.TagType)
	assert.Equal(t, "E20000001234", result.Value)
	assert.True(t, result.IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTagByID_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	tagID := 99999

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT id, type, value, is_active`).
		WithArgs(tagID, orgID).
		WillReturnError(errors.New("no rows in result set"))
	mock.ExpectCommit()

	result, err := storage.GetTagByID(context.Background(), orgID, tagID)

	assert.NoError(t, err) // Not found is not an error, returns nil
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTagsForAssets_Batch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	assetIDs := []int{1, 2, 3}

	rows := pgxmock.NewRows([]string{"asset_id", "id", "type", "value", "is_active"}).
		AddRow(1, 101, "rfid", "E20000001111", true).
		AddRow(1, 102, "ble", "AA:AA:AA:AA:AA:AA", true).
		AddRow(2, 201, "barcode", "BC-002", true)
	// Note: asset 3 has no tags

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT asset_id, id, type, value, is_active`).
		WithArgs(assetIDs, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.getTagsForAssets(context.Background(), orgID, assetIDs)

	assert.NoError(t, err)
	require.NotNil(t, result)

	// Asset 1 has 2 tags
	assert.Len(t, result[1], 2)
	assert.Equal(t, "rfid", result[1][0].TagType)
	assert.Equal(t, "ble", result[1][1].TagType)

	// Asset 2 has 1 tag
	assert.Len(t, result[2], 1)
	assert.Equal(t, "barcode", result[2][0].TagType)

	// Asset 3 has empty slice (not nil)
	assert.Len(t, result[3], 0)
	assert.NotNil(t, result[3])

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTagsForLocations_Batch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	orgID := 1
	locationIDs := []int{10, 20}

	rows := pgxmock.NewRows([]string{"location_id", "id", "type", "value", "is_active"}).
		AddRow(10, 1001, "barcode", "LOC-10", true).
		AddRow(20, 2001, "rfid", "E20000020001", true).
		AddRow(20, 2002, "barcode", "LOC-20", true)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value, is_active`).
		WithArgs(locationIDs, orgID).
		WillReturnRows(rows)
	mock.ExpectCommit()

	result, err := storage.getTagsForLocations(context.Background(), orgID, locationIDs)

	assert.NoError(t, err)
	require.NotNil(t, result)

	// Location 10 has 1 tag
	assert.Len(t, result[10], 1)
	assert.Equal(t, "barcode", result[10][0].TagType)

	// Location 20 has 2 tags
	assert.Len(t, result[20], 2)
	assert.Equal(t, "rfid", result[20][0].TagType)
	assert.Equal(t, "barcode", result[20][1].TagType)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTagsToJSON(t *testing.T) {
	t.Run("empty slice returns empty array", func(t *testing.T) {
		result, err := tagsToJSON([]shared.TagIdentifierRequest{})
		assert.NoError(t, err)
		assert.Equal(t, "[]", string(result))
	})

	t.Run("nil slice returns empty array", func(t *testing.T) {
		result, err := tagsToJSON(nil)
		assert.NoError(t, err)
		assert.Equal(t, "[]", string(result))
	})

	t.Run("single tag", func(t *testing.T) {
		input := []shared.TagIdentifierRequest{
			{TagType: "rfid", Value: "E20000001234"},
		}
		result, err := tagsToJSON(input)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"rfid","value":"E20000001234"}]`, string(result))
	})

	t.Run("multiple tags", func(t *testing.T) {
		input := []shared.TagIdentifierRequest{
			{TagType: "rfid", Value: "E20000001234"},
			{TagType: "ble", Value: "AA:BB:CC:DD:EE:FF"},
		}
		result, err := tagsToJSON(input)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"rfid","value":"E20000001234"},{"type":"ble","value":"AA:BB:CC:DD:EE:FF"}]`, string(result))
	})

	t.Run("applies default type when empty", func(t *testing.T) {
		input := []shared.TagIdentifierRequest{
			{Value: "E20000001234"}, // no tag_type specified
		}
		result, err := tagsToJSON(input)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"rfid","value":"E20000001234"}]`, string(result)) // defaults to rfid
	})

	t.Run("mixed explicit and default types", func(t *testing.T) {
		input := []shared.TagIdentifierRequest{
			{TagType: "ble", Value: "AA:BB:CC:DD:EE:FF"},
			{Value: "E20000001234"}, // no tag_type, defaults to rfid
		}
		result, err := tagsToJSON(input)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"ble","value":"AA:BB:CC:DD:EE:FF"},{"type":"rfid","value":"E20000001234"}]`, string(result))
	})
}

func TestTagIdentifierRequestGetType(t *testing.T) {
	t.Run("returns explicit type", func(t *testing.T) {
		req := shared.TagIdentifierRequest{TagType: "ble", Value: "test"}
		assert.Equal(t, "ble", req.GetType())
	})

	t.Run("returns default rfid when empty", func(t *testing.T) {
		req := shared.TagIdentifierRequest{Value: "test"}
		assert.Equal(t, "rfid", req.GetType())
	})
}

func TestParseTagError(t *testing.T) {
	t.Run("duplicate key error", func(t *testing.T) {
		err := parseTagError(errors.New("duplicate key value violates unique constraint"), "rfid", "E20000001234")
		assert.Contains(t, err.Error(), "rfid:E20000001234 already exists")
	})

	t.Run("generic error", func(t *testing.T) {
		err := parseTagError(errors.New("connection lost"), "rfid", "E20000001234")
		assert.Contains(t, err.Error(), "failed to create tag")
		assert.Contains(t, err.Error(), "connection lost")
	})
}
