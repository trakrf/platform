package bulkimport

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	"github.com/trakrf/platform/backend/internal/storage"
)

// Service handles bulk import business logic
type Service struct {
	storage   *storage.Storage
	validator *Validator
}

// NewService creates a new bulk import service
func NewService(storage *storage.Storage) *Service {
	return &Service{
		storage:   storage,
		validator: NewValidator(),
	}
}

// ProcessUpload handles the entire CSV upload workflow:
// 1. Validates file (size, type, format)
// 2. Parses and validates CSV content
// 3. Creates job in database
// 4. Returns immediate response (async processing in Phase 2B)
func (s *Service) ProcessUpload(
	ctx context.Context,
	orgID int,
	file multipart.File,
	header *multipart.FileHeader,
) (*bulkimport.UploadResponse, error) {
	// 1. Validate file
	if err := s.validator.ValidateFile(file, header); err != nil {
		return nil, err
	}

	// 2. Parse and validate CSV
	records, headers, err := s.validator.ParseAndValidateCSV(file)
	if err != nil {
		return nil, err
	}

	totalRows := len(records) - 1

	// 3. Create job in database
	job, err := s.storage.CreateBulkImportJob(ctx, orgID, totalRows)
	if err != nil {
		return nil, fmt.Errorf("failed to create import job: %w", err)
	}

	// 4. Return immediate response
	response := &bulkimport.UploadResponse{
		Status:    "accepted",
		JobID:     job.ID.String(),
		StatusURL: fmt.Sprintf("/api/v1/assets/bulk/%s", job.ID.String()),
		Message:   fmt.Sprintf("CSV upload accepted. Processing %d rows asynchronously.", totalRows),
	}

	// TODO Phase 2B: Launch goroutine here to process CSV rows
	// go s.processCSVAsync(context.Background(), job.ID, orgID, records, headers)
	_ = records // Suppress unused variable warning for now
	_ = headers // Suppress unused variable warning for now

	return response, nil
}

// processCSVAsync will be implemented in Phase 2B
// It will process CSV rows asynchronously and create assets
func (s *Service) processCSVAsync(
	ctx context.Context,
	jobID uuid.UUID,
	orgID int,
	records [][]string,
	headers []string,
) {
	// Phase 2B implementation:
	// 1. Parse each row
	// 2. Validate asset data
	// 3. Create assets
	// 4. Update job progress
	// 5. Handle errors
}
