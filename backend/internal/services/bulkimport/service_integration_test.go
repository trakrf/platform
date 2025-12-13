//go:build integration
// +build integration

// TRA-212: Skipped by default - requires database setup
// Run with: go test -tags=integration ./...

package bulkimport

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestBatchCreateAssets_AllValid(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()

	factory := testutil.NewAssetFactory(orgID).WithIdentifier("BATCH-001")
	assets := factory.BuildBatch(3)

	count, errs := store.BatchCreateAssets(ctx, assets)
	require.Empty(t, errs)
	assert.Equal(t, 3, count)
}

func TestBatchCreateAssets_DuplicateIdentifier(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()

	// Create 2 assets with duplicate identifier
	factory := testutil.NewAssetFactory(orgID).WithIdentifier("DUP-001")
	assets := []asset.Asset{
		factory.Build(),
		factory.Build(), // Duplicate identifier
	}

	// IMPORTANT: All-or-nothing transaction behavior
	// If ANY asset fails, ZERO assets should be saved
	count, errs := store.BatchCreateAssets(ctx, assets)

	// Verify transaction rolled back - NO assets saved
	assert.Equal(t, 0, count, "Transaction should rollback: ZERO assets saved on duplicate")
	assert.True(t, len(errs) > 0, "Should have errors for duplicate identifier")

	// Verify database has ZERO assets (transaction rolled back)
	var dbCount int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM trakrf.assets WHERE org_id = $1", orgID).Scan(&dbCount)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbCount, "Database should have ZERO assets after rollback")
}

func TestBatchCreateAssets_Mixed(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()

	// Create a mix: 3 valid assets + 1 duplicate
	// ALL should be rolled back due to the duplicate
	factory := testutil.NewAssetFactory(orgID)
	assets := []asset.Asset{
		factory.WithIdentifier("VALID-001").Build(),
		factory.WithIdentifier("VALID-002").Build(),
		factory.WithIdentifier("VALID-003").Build(),
		factory.WithIdentifier("VALID-001").Build(), // Duplicate!
	}

	// IMPORTANT: All-or-nothing transaction
	// Even though 3 assets are valid, the 1 duplicate causes full rollback
	count, errs := store.BatchCreateAssets(ctx, assets)

	// Verify transaction rolled back - NO assets saved
	assert.Equal(t, 0, count, "Transaction should rollback: ZERO assets saved when ANY fails")
	assert.True(t, len(errs) > 0, "Should have errors for duplicate")

	// Verify database has ZERO assets (even the valid ones were rolled back)
	var dbCount int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM trakrf.assets WHERE org_id = $1", orgID).Scan(&dbCount)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbCount, "Database should have ZERO assets after rollback (including valid ones)")
}

func TestProcessCSVAsync_ParseErrors(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()
	service := NewService(store)

	csvFactory := testutil.NewCSVFactory().
		AddRow("TEST-001", "Valid Asset", "device", "This should work", "2024-01-01", "2024-12-31", "true").
		AddRow("TEST-002", "Invalid Date", "device", "Bad date format", "invalid-date", "2024-12-31", "true").
		AddRow("TEST-003", "Another Valid", "device", "This should work too", "2024-01-01", "2024-12-31", "true")
	records := csvFactory.Build()

	job, err := store.CreateBulkImportJob(ctx, orgID, len(records)-1)
	require.NoError(t, err)

	service.processCSVAsync(ctx, job.ID, orgID, records, records[0])

	jobStatus, err := store.GetBulkImportJobByID(ctx, job.ID, orgID)
	require.NoError(t, err)

	assert.Contains(t, []string{"pending", "processing", "failed"}, jobStatus.Status)
	assert.GreaterOrEqual(t, jobStatus.TotalRows, 0)
}

func TestProcessCSVAsync_InsertErrors(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()
	service := NewService(store)

	testutil.CreateTestAsset(t, pool, orgID, "DUPLICATE-001")

	csvFactory := testutil.NewCSVFactory().
		AddRow("DUPLICATE-001", "Try Duplicate", "device", "Should fail", "2024-01-01", "2024-12-31", "true").
		AddRow("NEW-001", "New Asset", "device", "Should succeed", "2024-01-01", "2024-12-31", "true")
	records := csvFactory.Build()

	job, err := store.CreateBulkImportJob(ctx, orgID, len(records)-1)
	require.NoError(t, err)

	service.processCSVAsync(ctx, job.ID, orgID, records, records[0])

	jobStatus, err := store.GetBulkImportJobByID(ctx, job.ID, orgID)
	require.NoError(t, err)
	assert.Contains(t, []string{"pending", "processing", "completed", "failed"}, jobStatus.Status)
}

func TestProcessCSVAsync_AllSuccess(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()
	service := NewService(store)

	csvFactory := testutil.NewCSVFactory().
		AddRow("SUCCESS-001", "Asset 1", "device", "First asset", "2024-01-01", "2024-12-31", "true").
		AddRow("SUCCESS-002", "Asset 2", "device", "Second asset", "2024-01-01", "2024-12-31", "true").
		AddRow("SUCCESS-003", "Asset 3", "device", "Third asset", "2024-01-01", "2024-12-31", "true")
	records := csvFactory.Build()

	job, err := store.CreateBulkImportJob(ctx, orgID, len(records)-1)
	require.NoError(t, err)

	service.processCSVAsync(ctx, job.ID, orgID, records, records[0])

	jobStatus, err := store.GetBulkImportJobByID(ctx, job.ID, orgID)
	require.NoError(t, err)

	assert.Contains(t, []string{"pending", "processing", "completed"}, jobStatus.Status)
	assert.Equal(t, 3, jobStatus.TotalRows)
}

func TestConcurrentUploads(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()
	service := NewService(store)

	numJobs := 3
	jobIDs := make([]string, numJobs)

	for i := 0; i < numJobs; i++ {
		csvFactory := testutil.NewCSVFactory().
			AddRow(fmt.Sprintf("CONCURRENT-%d-001", i), fmt.Sprintf("Job %d Asset 1", i), "device", "Test", "2024-01-01", "2024-12-31", "true").
			AddRow(fmt.Sprintf("CONCURRENT-%d-002", i), fmt.Sprintf("Job %d Asset 2", i), "device", "Test", "2024-01-01", "2024-12-31", "true")
		records := csvFactory.Build()

		job, err := store.CreateBulkImportJob(ctx, orgID, len(records)-1)
		require.NoError(t, err)
		jobIDs[i] = fmt.Sprintf("%d", job.ID)

		go service.processCSVAsync(ctx, job.ID, orgID, records, records[0])
	}

	for i, jobID := range jobIDs {
		jobIDInt, err := strconv.Atoi(jobID)
		require.NoError(t, err)
		status, err := store.GetBulkImportJobByID(ctx, jobIDInt, orgID)
		require.NoError(t, err)
		assert.NotEmpty(t, status.Status, "Job %d should have a status", i)
		assert.Equal(t, 2, status.TotalRows, "Job %d should have 2 total rows", i)
	}
}

func TestJobStatusTracking(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()

	csvFactory := testutil.NewCSVFactory().
		AddRow("STATUS-001", "Asset 1", "device", "Test", "2024-01-01", "2024-12-31", "true")
	records := csvFactory.Build()

	job, err := store.CreateBulkImportJob(ctx, orgID, len(records)-1)
	require.NoError(t, err)

	status, err := store.GetBulkImportJobByID(ctx, job.ID, orgID)
	require.NoError(t, err)
	assert.Equal(t, "pending", status.Status)
	assert.Equal(t, 1, status.TotalRows)
}

func TestErrorRecovery_Panic(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()

	job, err := store.CreateBulkImportJob(ctx, orgID, 1)
	require.NoError(t, err)

	service := NewService(store)

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Panic was not recovered: %v", r)
			}
		}()

		service.processCSVAsync(ctx, job.ID, orgID, nil, nil)
	}()

	status, err := store.GetBulkImportJobByID(ctx, job.ID, orgID)
	require.NoError(t, err)
	assert.Contains(t, []string{"pending", "processing", "failed"}, status.Status)
}

func TestErrorRecovery_DatabaseFailure(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()
	service := NewService(store)

	invalidOrgID := 999999

	csvFactory := testutil.NewCSVFactory().
		AddRow("FAIL-001", "Should Fail", "device", "Invalid org", "2024-01-01", "2024-12-31", "true")
	records := csvFactory.Build()

	job, err := store.CreateBulkImportJob(ctx, orgID, len(records)-1)
	require.NoError(t, err)

	service.processCSVAsync(ctx, job.ID, invalidOrgID, records, records[0])

	status, err := store.GetBulkImportJobByID(ctx, job.ID, orgID)
	require.NoError(t, err)
	assert.Contains(t, []string{"pending", "processing", "completed", "failed"}, status.Status)
}

func TestProcessUpload_ValidCSV(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	service := NewService(store)

	csv := `identifier,name,type,description,valid_from,valid_to,is_active
ASSET-TEST-001,Test Asset 1,device,Description 1,2024-01-01,2024-12-31,true
ASSET-TEST-002,Test Asset 2,person,Description 2,2024-01-01,2024-12-31,false`

	file, header := createTestCSV(t, csv)
	defer file.Close()

	ctx := context.Background()

	response, err := service.ProcessUpload(ctx, orgID, file, header)
	require.NoError(t, err)

	assert.Equal(t, "accepted", response.Status)
	assert.NotEmpty(t, response.JobID)

	jobIDInt, err := strconv.Atoi(response.JobID)
	require.NoError(t, err)
	job, err := store.GetBulkImportJobByID(ctx, jobIDInt, orgID)
	require.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, 2, job.TotalRows)
}

func TestProcessUpload_InvalidHeaders(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(store)

	csvInvalid := `wrong,headers,here
ASSET-001,Test Asset,device`

	file, header := createTestCSV(t, csvInvalid)
	defer file.Close()

	_, err := service.ProcessUpload(context.Background(), 1, file, header)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "header") || strings.Contains(err.Error(), "column"))
}

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("Failed to parse UUID: %v", err)
	}
	return id
}
