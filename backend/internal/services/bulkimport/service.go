package bulkimport

import (
	"context"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	"github.com/trakrf/platform/backend/internal/storage"
	csvutil "github.com/trakrf/platform/backend/internal/util/csv"
)

const ProgressUpdateInterval = 10

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

	go s.processCSVAsync(context.Background(), job.ID, orgID, records, headers)

	return response, nil
}

func (s *Service) processCSVAsync(
	ctx context.Context,
	jobID uuid.UUID,
	orgID int,
	records [][]string,
	headers []string,
) {
	defer func() {
		if r := recover(); r != nil {
			s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		}
	}()

	s.storage.UpdateBulkImportJobStatus(ctx, jobID, "processing")

	var processedRows int
	var failedRows int
	var errors []bulkimport.ErrorDetail

	dataRows := records[1:]

	for rowIdx, row := range dataRows {
		rowNumber := rowIdx + 2

		asset, err := csvutil.MapCSVRowToAsset(row, headers, orgID)
		if err != nil {
			failedRows++
			errors = append(errors, bulkimport.ErrorDetail{
				Row:   rowNumber,
				Field: "",
				Error: err.Error(),
			})
			continue
		}

		_, err = s.storage.CreateAsset(ctx, *asset)
		if err != nil {
			failedRows++
			errorMsg := err.Error()
			field := ""

			if strings.Contains(errorMsg, "identifier") {
				field = "identifier"
			}

			errors = append(errors, bulkimport.ErrorDetail{
				Row:   rowNumber,
				Field: field,
				Error: errorMsg,
			})
		} else {
			processedRows++
		}

		if (rowIdx+1)%ProgressUpdateInterval == 0 {
			s.storage.UpdateBulkImportJobProgress(ctx, jobID, processedRows, failedRows, errors)
		}
	}

	s.storage.UpdateBulkImportJobProgress(ctx, jobID, processedRows, failedRows, errors)

	finalStatus := "completed"
	if processedRows == 0 {
		finalStatus = "failed"
	}

	s.storage.UpdateBulkImportJobStatus(ctx, jobID, finalStatus)
}
