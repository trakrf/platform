package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
)

// CreateBulkImportJob creates a new job record
func (s *Storage) CreateBulkImportJob(ctx context.Context, orgID int, totalRows int) (*bulkimport.BulkImportJob, error) {
	query := `
		INSERT INTO trakrf.bulk_import_jobs (org_id, status, total_rows)
		VALUES ($1, 'pending', $2)
		RETURNING id, org_id, status, total_rows, processed_rows, failed_rows, tags_created, errors, created_at, completed_at
	`

	var job bulkimport.BulkImportJob
	var errorsJSON []byte

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, totalRows).Scan(
			&job.ID, &job.OrgID, &job.Status, &job.TotalRows,
			&job.ProcessedRows, &job.FailedRows, &job.TagsCreated, &errorsJSON,
			&job.CreatedAt, &job.CompletedAt,
		)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create bulk import job: %w", err)
	}

	// Parse errors JSONB
	if err := json.Unmarshal(errorsJSON, &job.Errors); err != nil {
		return nil, fmt.Errorf("failed to parse job errors: %w", err)
	}

	return &job, nil
}

// GetBulkImportJobByID retrieves a job by ID and org_id (tenant isolation)
// Note: parameter order is (jobID, orgID) — inconsistent with other methods; left as-is.
func (s *Storage) GetBulkImportJobByID(ctx context.Context, jobID int, orgID int) (*bulkimport.BulkImportJob, error) {
	query := `
		SELECT id, org_id, status, total_rows, processed_rows, failed_rows, tags_created, errors, created_at, completed_at
		FROM trakrf.bulk_import_jobs
		WHERE id = $1 AND org_id = $2
	`

	var job bulkimport.BulkImportJob
	var errorsJSON []byte

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, jobID, orgID).Scan(
			&job.ID, &job.OrgID, &job.Status, &job.TotalRows,
			&job.ProcessedRows, &job.FailedRows, &job.TagsCreated, &errorsJSON,
			&job.CreatedAt, &job.CompletedAt,
		)
	})

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Job not found or doesn't belong to org
		}
		return nil, fmt.Errorf("failed to get bulk import job: %w", err)
	}

	// Parse errors JSONB
	if err := json.Unmarshal(errorsJSON, &job.Errors); err != nil {
		return nil, fmt.Errorf("failed to parse job errors: %w", err)
	}

	return &job, nil
}

// UpdateBulkImportJobProgress updates job progress, tags created, and errors
func (s *Storage) UpdateBulkImportJobProgress(ctx context.Context, orgID int, jobID int, processedRows, failedRows, tagsCreated int, errors []bulkimport.ErrorDetail) error {
	errorsJSON, err := json.Marshal(errors)
	if err != nil {
		return fmt.Errorf("failed to marshal errors: %w", err)
	}

	fmt.Printf("UpdateBulkImportJobProgress called for job %d: processedRows=%d, failedRows=%d, tagsCreated=%d, errors=%d\n",
		jobID, processedRows, failedRows, tagsCreated, len(errors))

	query := `
		UPDATE trakrf.bulk_import_jobs
		SET processed_rows = $3, failed_rows = $4, tags_created = $5, errors = $6
		WHERE id = $1 AND org_id = $2
	`

	var rowsAffected int64
	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, query, jobID, orgID, processedRows, failedRows, tagsCreated, errorsJSON)
		if err != nil {
			fmt.Printf("UpdateBulkImportJobProgress FAILED for job %d: %v\n", jobID, err)
			return err
		}
		rowsAffected = result.RowsAffected()
		fmt.Printf("UpdateBulkImportJobProgress affected %d rows for job %d\n", rowsAffected, jobID)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update job progress: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("job not found: %d", jobID)
	}

	return nil
}

// UpdateBulkImportJobStatus updates job status and optionally sets completed_at
func (s *Storage) UpdateBulkImportJobStatus(ctx context.Context, orgID int, jobID int, status string) error {
	query := `
		UPDATE trakrf.bulk_import_jobs
		SET status = $3, completed_at = CASE WHEN $3 IN ('completed', 'failed') THEN NOW() ELSE completed_at END
		WHERE id = $1 AND org_id = $2
	`

	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, query, jobID, orgID, status)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("job not found: %d", jobID)
	}

	return nil
}
