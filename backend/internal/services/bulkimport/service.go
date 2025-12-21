package bulkimport

import (
	"context"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/storage"
	csvutil "github.com/trakrf/platform/backend/internal/util/csv"
)

// isEmptyRow checks if a CSV row is empty (all fields are empty or whitespace)
func isEmptyRow(row []string) bool {
	for _, field := range row {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

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
			s.storage.UpdateBulkImportJobProgress(ctx, jobID, 0, 1, 0, []bulkimport.ErrorDetail{panicErr})
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
		s.storage.UpdateBulkImportJobProgress(ctx, jobID, 0, 1, 0, []bulkimport.ErrorDetail{panicErr})
		s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		return
	}

	dataRows := records[1:]
	fmt.Printf("Processing %d raw data rows for job %d\n", len(dataRows), jobID)

	// PHASE 1: Parse all rows with tags and collect ALL parse errors
	type parsedRow struct {
		rowNumber int
		asset     *asset.Asset
		tagValues []string
	}

	var allErrors []bulkimport.ErrorDetail
	validRows := make([]parsedRow, 0, len(dataRows))
	var emptyRowCount int

	for rowIdx, row := range dataRows {
		rowNumber := rowIdx + 2 // +1 for 0-index, +1 for header row

		// Skip empty rows silently
		if isEmptyRow(row) {
			emptyRowCount++
			continue
		}

		result, err := csvutil.MapCSVRowToAssetWithTags(row, headers, orgID)
		if err != nil {
			fmt.Printf("Parse error at row %d for job %d: %v\n", rowNumber, jobID, err)
			allErrors = append(allErrors, bulkimport.ErrorDetail{
				Row:   rowNumber,
				Field: "",
				Error: err.Error(),
			})
			continue // Continue to find ALL parse errors
		}

		validRows = append(validRows, parsedRow{
			rowNumber: rowNumber,
			asset:     result.Asset,
			tagValues: result.TagValues,
		})
	}

	// Calculate actual data rows (excluding empty rows)
	totalDataRows := len(dataRows) - emptyRowCount
	if emptyRowCount > 0 {
		fmt.Printf("Skipped %d empty rows for job %d\n", emptyRowCount, jobID)
	}
	fmt.Printf("Validating %d data rows for job %d\n", totalDataRows, jobID)

	// PHASE 2: Check for duplicate identifiers WITHIN the CSV batch
	identifierToRows := make(map[string][]int)
	for _, pr := range validRows {
		identifier := pr.asset.Identifier
		identifierToRows[identifier] = append(identifierToRows[identifier], pr.rowNumber)
	}

	for identifier, rowNumbers := range identifierToRows {
		if len(rowNumbers) > 1 {
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

	// PHASE 3: Check for duplicate tag values WITHIN the CSV batch
	tagToRows := make(map[string][]int) // tag value -> list of row numbers
	for _, pr := range validRows {
		for _, tag := range pr.tagValues {
			tagToRows[tag] = append(tagToRows[tag], pr.rowNumber)
		}
	}

	for tag, rowNumbers := range tagToRows {
		if len(rowNumbers) > 1 {
			for _, rowNum := range rowNumbers {
				fmt.Printf("Duplicate tag '%s' at row %d in CSV for job %d\n", tag, rowNum, jobID)
				allErrors = append(allErrors, bulkimport.ErrorDetail{
					Row:   rowNum,
					Field: "tags",
					Error: fmt.Sprintf("duplicate tag '%s' appears in rows %v within the CSV", tag, rowNumbers),
				})
			}
		}
	}

	// PHASE 4: If ANY errors found, report them all and fail
	if len(allErrors) > 0 {
		fmt.Printf("Found %d total errors for job %d, marking as failed\n", len(allErrors), jobID)
		// processed_rows = 0 (no successful inserts), failed_rows = total (all rows failed validation)
		s.storage.UpdateBulkImportJobProgress(ctx, jobID, 0, totalDataRows, 0, allErrors)
		s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		return
	}

	// PHASE 5: Insert assets with tags one at a time
	// Using per-row insertion to capture per-row errors for duplicate tags from DB
	fmt.Printf("All validations passed. Inserting %d assets for job %d\n", len(validRows), jobID)

	var successCount int
	var tagsCreated int
	var insertErrors []bulkimport.ErrorDetail

	for _, pr := range validRows {
		// Convert tag values to TagIdentifierRequest with type "rfid"
		identifiers := make([]shared.TagIdentifierRequest, len(pr.tagValues))
		for i, tag := range pr.tagValues {
			identifiers[i] = shared.TagIdentifierRequest{
				Type:  "rfid",
				Value: tag,
			}
		}

		// Build CreateAssetWithIdentifiersRequest
		request := asset.CreateAssetWithIdentifiersRequest{
			CreateAssetRequest: asset.CreateAssetRequest{
				OrgID:       pr.asset.OrgID,
				Identifier:  pr.asset.Identifier,
				Name:        pr.asset.Name,
				Type:        pr.asset.Type,
				Description: pr.asset.Description,
				ValidFrom:   shared.FlexibleDate{Time: pr.asset.ValidFrom},
				IsActive:    pr.asset.IsActive,
			},
			Identifiers: identifiers,
		}
		if pr.asset.ValidTo != nil {
			validTo := shared.FlexibleDate{Time: *pr.asset.ValidTo}
			request.ValidTo = &validTo
		}

		_, err := s.storage.CreateAssetWithIdentifiers(ctx, request)
		if err != nil {
			fmt.Printf("Insert error at row %d for job %d: %v\n", pr.rowNumber, jobID, err)
			insertErrors = append(insertErrors, bulkimport.ErrorDetail{
				Row:   pr.rowNumber,
				Field: "",
				Error: err.Error(),
			})
			continue
		}

		successCount++
		tagsCreated += len(pr.tagValues)
	}

	if len(insertErrors) > 0 {
		fmt.Printf("Insert completed with errors for job %d: %d success, %d failed\n", jobID, successCount, len(insertErrors))
		s.storage.UpdateBulkImportJobProgress(ctx, jobID, successCount, len(insertErrors), tagsCreated, insertErrors)
		if successCount == 0 {
			s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		} else {
			s.storage.UpdateBulkImportJobStatus(ctx, jobID, "completed")
		}
		return
	}

	fmt.Printf("Successfully completed job %d with %d assets and %d tags\n", jobID, successCount, tagsCreated)
	s.storage.UpdateBulkImportJobProgress(ctx, jobID, successCount, 0, tagsCreated, nil)
	s.storage.UpdateBulkImportJobStatus(ctx, jobID, "completed")
}
