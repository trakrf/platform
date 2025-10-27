package bulkimport

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	"github.com/trakrf/platform/backend/internal/storage"
)

type Service struct {
	storage   *storage.Storage
	validator *Validator
}

func NewService(storage *storage.Storage) *Service {
	return &Service{
		storage:   storage,
		validator: NewValidator(),
	}
}

func (s *Service) ProcessUpload(
	ctx context.Context,
	orgID int,
	file multipart.File,
	header *multipart.FileHeader,
) (*bulkimport.UploadResponse, error) {
	if err := s.validator.ValidateFile(file, header); err != nil {
		return nil, err
	}

	records, headers, err := s.validator.ParseAndValidateCSV(file)
	if err != nil {
		return nil, err
	}

	totalRows := len(records) - 1

	job, err := s.storage.CreateBulkImportJob(ctx, orgID, totalRows)
	if err != nil {
		return nil, fmt.Errorf("failed to create import job: %w", err)
	}

	response := &bulkimport.UploadResponse{
		Status:    "accepted",
		JobID:     job.ID.String(),
		StatusURL: fmt.Sprintf("/api/v1/assets/bulk/%s", job.ID.String()),
		Message:   fmt.Sprintf("CSV upload accepted. Processing %d rows asynchronously.", totalRows),
	}

	// TODO Phase 2B: Launch goroutine to process CSV rows
	// go s.processCSVAsync(context.Background(), job.ID, orgID, records, headers)
	_ = records
	_ = headers

	return response, nil
}

func (s *Service) processCSVAsync(
	ctx context.Context,
	jobID uuid.UUID,
	orgID int,
	records [][]string,
	headers []string,
) {
	// Phase 2B implementation
}
