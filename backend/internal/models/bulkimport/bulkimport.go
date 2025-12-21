package bulkimport

import (
	"time"
)

// ErrorDetail represents a single row error during bulk import
type ErrorDetail struct {
	Row   int    `json:"row"`
	Field string `json:"field,omitempty"`
	Error string `json:"error"`
}

// BulkImportJob represents an async bulk import operation
type BulkImportJob struct {
	ID            int           `json:"job_id"`
	OrgID         int           `json:"org_id"`
	Status        string        `json:"status"` // pending, processing, completed, failed
	TotalRows     int           `json:"total_rows"`
	ProcessedRows int           `json:"processed_rows"`
	FailedRows    int           `json:"failed_rows"`
	TagsCreated   int           `json:"tags_created"`
	Errors        []ErrorDetail `json:"errors,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
}

// CreateJobRequest is used when creating a new job (Phase 2 will use this)
type CreateJobRequest struct {
	OrgID     int `json:"org_id" validate:"required,min=1"`
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
	TagsCreated    int           `json:"tags_created,omitempty"`
	CreatedAt      string        `json:"created_at"`
	CompletedAt    string        `json:"completed_at,omitempty"`
	Errors         []ErrorDetail `json:"errors,omitempty"`
}

// UploadResponse is returned when a CSV file is successfully accepted
type UploadResponse struct {
	Status    string `json:"status"`     // "accepted"
	JobID     string `json:"job_id"`     // UUID string
	StatusURL string `json:"status_url"` // "/api/v1/assets/bulk/{jobId}"
	Message   string `json:"message"`    // User-friendly message
}
