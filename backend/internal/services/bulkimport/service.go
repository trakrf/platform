package bulkimport

import (
	"context"
	"fmt"
	"mime/multipart"

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
		JobID:     fmt.Sprintf("%d", job.ID),
		StatusURL: fmt.Sprintf("/api/v1/assets/bulk/%d", job.ID),
		Message:   fmt.Sprintf("CSV upload accepted. Processing %d rows asynchronously.", totalRows),
	}

	go s.processCSVAsync(context.Background(), job.ID, orgID, records, headers)

	return response, nil
}

func (s *Service) processCSVAsync(
	ctx context.Context,
	jobID int,
	orgID int,
	records [][]string,
	headers []string,
) {
	defer func() {
		if r := recover(); r != nil {
			panicErr := bulkimport.ErrorDetail{
				Row:   0,
				Field: "system",
				Error: fmt.Sprintf("Panic during processing: %v", r),
			}
			fmt.Printf("PANIC in processCSVAsync for job %d: %v\n", jobID, r)
			s.storage.UpdateBulkImportJobProgress(ctx, jobID, 0, 1, []bulkimport.ErrorDetail{panicErr})
			s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		}
	}()

	fmt.Printf("Starting processCSVAsync for job %d, orgID %d, records: %d\n", jobID, orgID, len(records))

	if err := s.storage.UpdateBulkImportJobStatus(ctx, jobID, "processing"); err != nil {
		fmt.Printf("Failed to update job status to processing for job %d: %v\n", jobID, err)
		panicErr := bulkimport.ErrorDetail{
			Row:   0,
			Field: "system",
			Error: fmt.Sprintf("Failed to update job status: %v", err),
		}
		s.storage.UpdateBulkImportJobProgress(ctx, jobID, 0, 1, []bulkimport.ErrorDetail{panicErr})
		s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		return
	}

	dataRows := records[1:]
	totalDataRows := len(dataRows)
	fmt.Printf("Validating %d data rows for job %d\n", totalDataRows, jobID)

	// PHASE 1: Parse all rows and collect ALL parse errors (don't stop on first error)
	type parsedRow struct {
		rowNumber int
		asset     *asset.Asset
	}

	var allErrors []bulkimport.ErrorDetail
	validRows := make([]parsedRow, 0, len(dataRows))

	for rowIdx, row := range dataRows {
		rowNumber := rowIdx + 2 // +1 for 0-index, +1 for header row

		parsedAsset, err := csvutil.MapCSVRowToAsset(row, headers, orgID)
		if err != nil {
			fmt.Printf("Parse error at row %d for job %d: %v\n", rowNumber, jobID, err)
			allErrors = append(allErrors, bulkimport.ErrorDetail{
				Row:   rowNumber,
				Field: "",
				Error: err.Error(),
			})
			continue // Continue to find ALL parse errors, not just the first one
		}

		validRows = append(validRows, parsedRow{
			rowNumber: rowNumber,
			asset:     parsedAsset,
		})
	}

	// PHASE 2: Check for duplicate identifiers WITHIN the CSV batch itself
	identifierToRows := make(map[string][]int) // identifier -> list of row numbers
	for _, pr := range validRows {
		identifier := pr.asset.Identifier
		identifierToRows[identifier] = append(identifierToRows[identifier], pr.rowNumber)
	}

	// Report duplicates within CSV
	for identifier, rowNumbers := range identifierToRows {
		if len(rowNumbers) > 1 {
			// Multiple rows have the same identifier
			for _, rowNum := range rowNumbers {
				fmt.Printf("Duplicate identifier '%s' at row %d in CSV for job %d\n", identifier, rowNum, jobID)
				allErrors = append(allErrors, bulkimport.ErrorDetail{
					Row:   rowNum,
					Field: "identifier",
					Error: fmt.Sprintf("duplicate identifier '%s' appears in rows %v within the CSV", identifier, rowNumbers),
				})
			}
		}
	}

	// PHASE 3: Database duplicate check REMOVED
	// With UPSERT (ON CONFLICT DO UPDATE), existing identifiers will be updated instead of rejected
	// This allows users to re-upload CSVs to update asset information and resurrect soft-deleted assets
	// Only duplicates WITHIN the same CSV batch (PHASE 2) are still rejected

	// PHASE 4: If ANY errors found, report them all and fail
	if len(allErrors) > 0 {
		fmt.Printf("Found %d total errors for job %d, marking as failed\n", len(allErrors), jobID)
		s.storage.UpdateBulkImportJobProgress(ctx, jobID, totalDataRows, totalDataRows, allErrors)
		s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		return
	}

	// PHASE 5: All validation passed - extract assets and insert
	fmt.Printf("All validations passed. Attempting to insert %d assets for job %d\n", len(validRows), jobID)
	assets := make([]asset.Asset, len(validRows))
	for i, pr := range validRows {
		assets[i] = *pr.asset
	}

	successCount, insertErrors := s.storage.BatchCreateAssets(ctx, assets)
	fmt.Printf("BatchCreateAssets returned: successCount=%d, errors=%d for job %d\n", successCount, len(insertErrors), jobID)

	if len(insertErrors) > 0 {
		// Unexpected database errors during insert (shouldn't happen since we pre-validated)
		var errorDetails []bulkimport.ErrorDetail
		for _, err := range insertErrors {
			fmt.Printf("Unexpected insert error for job %d: %s\n", jobID, err.Error())
			errorDetails = append(errorDetails, bulkimport.ErrorDetail{
				Row:   0,
				Field: "",
				Error: fmt.Sprintf("unexpected database error: %s", err.Error()),
			})
		}

		s.storage.UpdateBulkImportJobProgress(ctx, jobID, totalDataRows, totalDataRows, errorDetails)
		s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		return
	}

	fmt.Printf("Successfully completed job %d with %d assets\n", jobID, successCount)
	s.storage.UpdateBulkImportJobProgress(ctx, jobID, successCount, 0, nil)
	s.storage.UpdateBulkImportJobStatus(ctx, jobID, "completed")
}
