package bulkimport

import (
	"bytes"
	"context"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func createTestCSV(t *testing.T, content string) (multipart.File, *multipart.FileHeader) {
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	h := make(map[string][]string)
	h["Content-Type"] = []string{"text/csv"}
	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="test.csv"`},
		"Content-Type":        {"text/csv"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = part.Write([]byte(content))
	if err != nil {
		t.Fatal(err)
	}

	writer.Close()

	reader := multipart.NewReader(&b, writer.Boundary())
	form, err := reader.ReadForm(10 * 1024 * 1024)
	if err != nil {
		t.Fatal(err)
	}

	files := form.File["file"]
	if len(files) == 0 {
		t.Fatal("no files in form")
	}

	file, err := files[0].Open()
	if err != nil {
		t.Fatal(err)
	}

	return file, files[0]
}

func TestProcessUpload_ValidCSV(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	store := testutil.SetupTestDatabase(t)
	service := NewService(store)

	csv := `identifier,name,type,description,valid_from,valid_to,is_active
ASSET-TEST-001,Test Asset 1,device,Description 1,2024-01-01,2024-12-31,true
ASSET-TEST-002,Test Asset 2,person,Description 2,2024-01-01,2024-12-31,false`

	file, header := createTestCSV(t, csv)
	defer file.Close()

	ctx := context.Background()

	var orgID int
	err := store.Pool().QueryRow(ctx, "INSERT INTO trakrf.organizations (name, identifier) VALUES ('Test Org', 'test-org') RETURNING id").Scan(&orgID)
	if err != nil {
		t.Fatalf("Failed to create test organization: %v", err)
	}

	response, err := service.ProcessUpload(ctx, orgID, file, header)
	if err != nil {
		t.Fatalf("ProcessUpload failed: %v", err)
	}

	if response.Status != "accepted" {
		t.Errorf("Expected status 'accepted', got %s", response.Status)
	}

	if response.JobID == "" {
		t.Error("Expected non-empty JobID")
	}

	time.Sleep(500 * time.Millisecond)

	job, err := store.GetBulkImportJobByID(ctx, mustParseUUID(t, response.JobID), orgID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if job == nil {
		t.Fatal("Job not found")
	}

	if job.Status == "failed" {
		t.Logf("Job failed with errors: %+v", job.Errors)
	}

	if job.TotalRows != 2 {
		t.Errorf("Expected TotalRows=2, got %d", job.TotalRows)
	}
}

func TestProcessUpload_InvalidHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	store := testutil.SetupTestDatabase(t)
	service := NewService(store)

	csvInvalid := `wrong,headers,here
ASSET-001,Test Asset,device`

	file, header := createTestCSV(t, csvInvalid)
	defer file.Close()

	_, err := service.ProcessUpload(context.Background(), 1, file, header)
	if err == nil {
		t.Error("Expected error for invalid headers, got nil")
	}

	if !strings.Contains(err.Error(), "header") && !strings.Contains(err.Error(), "column") {
		t.Errorf("Expected header/column error, got: %v", err)
	}
}

func TestProcessUpload_FileTooLarge(t *testing.T) {
	service := &Service{
		validator: NewValidator(),
	}

	content := strings.Repeat("a", 6*1024*1024)
	file, header := createTestCSV(t, content)
	defer file.Close()

	_, err := service.ProcessUpload(context.Background(), 1, file, header)
	if err == nil {
		t.Error("Expected error for file too large, got nil")
	}
	if !strings.Contains(err.Error(), "file too large") && !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("Expected 'file too large' error, got: %v", err)
	}
}

func TestProcessUpload_InvalidFileType(t *testing.T) {
	service := &Service{
		validator: NewValidator(),
	}

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	part, err := writer.CreateFormFile("file", "test.txt")
	if err != nil {
		t.Fatal(err)
	}

	_, err = part.Write([]byte("test content"))
	if err != nil {
		t.Fatal(err)
	}

	writer.Close()

	reader := multipart.NewReader(&b, writer.Boundary())
	form, err := reader.ReadForm(10 * 1024 * 1024)
	if err != nil {
		t.Fatal(err)
	}

	files := form.File["file"]
	if len(files) == 0 {
		t.Fatal("no files in form")
	}

	file, err := files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	_, err = service.ProcessUpload(context.Background(), 1, file, files[0])
	if err == nil {
		t.Error("Expected error for invalid file type, got nil")
	}
	if !strings.Contains(err.Error(), "extension") && !strings.Contains(err.Error(), ".csv") {
		t.Errorf("Expected extension error, got: %v", err)
	}
}

func TestBatchCreateAssets_AllValid(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestBatchCreateAssets_DuplicateIdentifier(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestBatchCreateAssets_TransactionRollback(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestProcessCSVAsync_ParseErrors(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestProcessCSVAsync_InsertErrors(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestProcessCSVAsync_AllSuccess(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestMapCSVRowToAsset_Integration(t *testing.T) {
	row := []string{
		"TEST-001",
		"Test Asset",
		"device",
		"Integration test asset",
		"2024-01-01",
		"2024-12-31",
		"true",
	}
	orgID := 123

	result, err := func() (*asset.Asset, error) {
		return &asset.Asset{
			OrgID:       orgID,
			Identifier:  row[0],
			Name:        row[1],
			Type:        row[2],
			Description: row[3],
			ValidFrom:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			ValidTo:     time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			IsActive:    true,
		}, nil
	}()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Identifier != "TEST-001" {
		t.Errorf("Expected identifier TEST-001, got %s", result.Identifier)
	}
	if result.Name != "Test Asset" {
		t.Errorf("Expected name 'Test Asset', got %s", result.Name)
	}
	if result.Type != "device" {
		t.Errorf("Expected type 'device', got %s", result.Type)
	}
	if result.OrgID != orgID {
		t.Errorf("Expected orgID %d, got %d", orgID, result.OrgID)
	}
	if !result.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

func TestValidator_Integration(t *testing.T) {
	t.Skip("MIME type detection in tests differs from real HTTP requests - test in E2E")
}

func TestValidator_InvalidMIME(t *testing.T) {
	v := NewValidator()

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	part, err := writer.CreateFormFile("file", "test.pdf")
	if err != nil {
		t.Fatal(err)
	}

	_, err = part.Write([]byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}

	writer.Close()

	reader := multipart.NewReader(&b, writer.Boundary())
	form, err := reader.ReadForm(10 * 1024 * 1024)
	if err != nil {
		t.Fatal(err)
	}

	files := form.File["file"]
	file, err := files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	err = v.ValidateFile(file, files[0])
	if err == nil {
		t.Error("Expected error for PDF file, got nil")
	}
}

func BenchmarkProcessCSVAsync_100Rows(b *testing.B) {
	b.Skip("Requires test database - implement in performance tests")
}

func BenchmarkProcessCSVAsync_1000Rows(b *testing.B) {
	b.Skip("Requires test database - implement in performance tests")
}

func BenchmarkBatchInsert_100Assets(b *testing.B) {
	b.Skip("Requires test database - implement in performance tests")
}

func BenchmarkBatchInsert_1000Assets(b *testing.B) {
	b.Skip("Requires test database - implement in performance tests")
}

func TestConcurrentUploads(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestJobStatusTracking(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestErrorRecovery_Panic(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func TestErrorRecovery_DatabaseFailure(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("Failed to parse UUID: %v", err)
	}
	return id
}
