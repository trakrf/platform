package bulkimport

import (
	"context"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/models/asset"
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

	dataRows := records[1:]
	assets := make([]asset.Asset, 0, len(dataRows))
	var parseErrors []bulkimport.ErrorDetail

	for rowIdx, row := range dataRows {
		rowNumber := rowIdx + 2

		parsedAsset, err := csvutil.MapCSVRowToAsset(row, headers, orgID)
		if err != nil {
			parseErrors = append(parseErrors, bulkimport.ErrorDetail{
				Row:   rowNumber,
				Field: "",
				Error: err.Error(),
			})
			continue
		}

		assets = append(assets, *parsedAsset)
	}

	if len(parseErrors) > 0 {
		s.storage.UpdateBulkImportJobProgress(ctx, jobID, 0, len(parseErrors), parseErrors)
		s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		return
	}

	successCount, insertErrors := s.storage.BatchCreateAssets(ctx, assets)

	if len(insertErrors) > 0 {
		var errorDetails []bulkimport.ErrorDetail
		for _, err := range insertErrors {
			errorMsg := err.Error()
			var rowNum int
			var field string

			if n, parseErr := fmt.Sscanf(errorMsg, "row %d:", &rowNum); n == 1 && parseErr == nil {
				if strings.Contains(errorMsg, "identifier") {
					field = "identifier"
				}
			}

			errorDetails = append(errorDetails, bulkimport.ErrorDetail{
				Row:   rowNum + 2,
				Field: field,
				Error: errorMsg,
			})
		}

		s.storage.UpdateBulkImportJobProgress(ctx, jobID, 0, len(insertErrors), errorDetails)
		s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		return
	}

	s.storage.UpdateBulkImportJobProgress(ctx, jobID, successCount, 0, nil)
	s.storage.UpdateBulkImportJobStatus(ctx, jobID, "completed")
}
