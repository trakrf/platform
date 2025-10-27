package bulkimport

import (
	"time"

	"github.com/google/uuid"
)

// ErrorDetail represents a single row error during bulk import
type ErrorDetail struct {
	Row   int    `json:"row"`
	Field string `json:"field,omitempty"`
	Error string `json:"error"`
}

// BulkImportJob represents an async bulk import operation
type BulkImportJob struct {
	ID            uuid.UUID     `json:"job_id"`
	AccountID     int           `json:"account_id"`
	Status        string        `json:"status"` // pending, processing, completed, failed
	TotalRows     int           `json:"total_rows"`
	ProcessedRows int           `json:"processed_rows"`
	FailedRows    int           `json:"failed_rows"`
	Errors        []ErrorDetail `json:"errors,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
}

// CreateJobRequest is used when creating a new job (Phase 2 will use this)
type CreateJobRequest struct {
	AccountID int `json:"account_id" validate:"required,min=1"`
	TotalRows int `json:"total_rows" validate:"required,min=1,max=1000"`
}

// UpdateJobProgressRequest is used to update job progress
type UpdateJobProgressRequest struct {
	ProcessedRows int           `json:"processed_rows" validate:"required,min=0"`
	FailedRows    int           `json:"failed_rows" validate:"min=0"`
	Errors        []ErrorDetail `json:"errors,omitempty"`
}

// JobStatusResponse is returned by the status endpoint
type JobStatusResponse struct {
	JobID          string        `json:"job_id"`
	Status         string        `json:"status"`
	TotalRows      int           `json:"total_rows"`
	ProcessedRows  int           `json:"processed_rows"`
	FailedRows     int           `json:"failed_rows"`
	SuccessfulRows int           `json:"successful_rows,omitempty"` // Calculated: processed - failed
	CreatedAt      string        `json:"created_at"`
	CompletedAt    string        `json:"completed_at,omitempty"`
	Errors         []ErrorDetail `json:"errors,omitempty"`
}
