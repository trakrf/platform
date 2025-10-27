package assets

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	csvutil "github.com/trakrf/platform/backend/internal/util/csv"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

const (
	MaxFileSize = 5 * 1024 * 1024 // 5MB
	MaxRows     = 1000            // Maximum rows per CSV
)

// Allowed MIME types for CSV files
var allowedMIMETypes = map[string]bool{
	"text/csv":                 true,
	"application/vnd.ms-excel": true,
	"application/csv":          true,
	"text/plain":               true, // Some systems send CSV as text/plain
}

// @Summary Get bulk import job status
// @Description Retrieve the status of a bulk import job by ID
// @Tags bulk-import
// @Accept json
// @Produce json
// @Param jobId path string true "Job ID (UUID)"
// @Success 200 {object} bulkimport.JobStatusResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid job ID"
// @Failure 404 {object} modelerrors.ErrorResponse "Job not found or access denied"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets/bulk/{jobId} [get]
func (handler *Handler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())
	jobIDParam := chi.URLParam(r, "jobId")

	// Parse job ID as UUID
	jobID, err := uuid.Parse(jobIDParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid job ID format", err.Error(), requestID)
		return
	}

	// Extract org_id from JWT claims for tenant isolation
	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Missing org context", "", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	// Retrieve job from storage (with tenant isolation)
	job, err := handler.storage.GetBulkImportJobByID(r.Context(), jobID, orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to retrieve job", err.Error(), requestID)
		return
	}

	if job == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			"Job not found or does not belong to your org", "", requestID)
		return
	}

	// Build response
	response := bulkimport.JobStatusResponse{
		JobID:         job.ID.String(),
		Status:        job.Status,
		TotalRows:     job.TotalRows,
		ProcessedRows: job.ProcessedRows,
		FailedRows:    job.FailedRows,
		CreatedAt:     job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Errors:        job.Errors,
	}

	if job.Status == "completed" {
		response.SuccessfulRows = job.ProcessedRows - job.FailedRows
	}

	if job.CompletedAt != nil {
		response.CompletedAt = job.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	httputil.WriteJSON(w, http.StatusOK, response)
}

// @Summary Upload CSV for bulk asset creation
// @Description Accepts CSV file and creates async job. Returns immediately with job ID.
// @Tags bulk-import
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "CSV file with assets"
// @Success 202 {object} bulkimport.UploadResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid file or headers"
// @Failure 413 {object} modelerrors.ErrorResponse "File too large"
// @Security BearerAuth
// @Router /api/v1/assets/bulk [post]
func (handler *Handler) UploadCSV(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	// Extract org_id from JWT claims
	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Missing org context", "", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	// Parse multipart form (max 6MB to account for overhead beyond 5MB file)
	err := r.ParseMultipartForm(6 * 1024 * 1024)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Failed to parse multipart form", err.Error(), requestID)
		return
	}

	// Get the file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Missing or invalid 'file' field", err.Error(), requestID)
		return
	}
	defer file.Close()

	// Validate file size
	if header.Size > MaxFileSize {
		httputil.WriteJSONError(w, r, http.StatusRequestEntityTooLarge, modelerrors.ErrBadRequest,
			fmt.Sprintf("File too large: %d bytes (max %d bytes / 5MB)", header.Size, MaxFileSize),
			"", requestID)
		return
	}

	// Validate file extension
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".csv") {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid file extension: must be .csv", "", requestID)
		return
	}

	// Validate MIME type (use Content-Type from header as fallback)
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		// Try to detect from file
		buffer := make([]byte, 512)
		_, err := file.Read(buffer)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				"Failed to read file for type detection", err.Error(), requestID)
			return
		}
		contentType = http.DetectContentType(buffer)
		// Reset file pointer
		file.Seek(0, 0)
	}

	if !allowedMIMETypes[contentType] {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf("Invalid MIME type: %s (expected text/csv or application/vnd.ms-excel)", contentType),
			"", requestID)
		return
	}

	// Read all CSV content into memory (safe due to 5MB limit)
	csvContent, err := io.ReadAll(file)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to read CSV file", err.Error(), requestID)
		return
	}

	// Parse CSV
	csvReader := csv.NewReader(bytes.NewReader(csvContent))
	records, err := csvReader.ReadAll()
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid CSV format", err.Error(), requestID)
		return
	}

	// Validate minimum content
	if len(records) < 1 {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"CSV file is empty", "", requestID)
		return
	}

	// Validate headers (using csvutil.ValidateCSVHeaders from util/csv package)
	headers := records[0]
	if err := csvutil.ValidateCSVHeaders(headers); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid CSV headers", err.Error(), requestID)
		return
	}

	// Count data rows (exclude header)
	totalRows := len(records) - 1
	if totalRows == 0 {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"CSV has headers but no data rows", "", requestID)
		return
	}

	// Validate row limit
	if totalRows > MaxRows {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf("Too many rows: %d (max %d)", totalRows, MaxRows),
			"", requestID)
		return
	}

	// Create job in database
	job, err := handler.storage.CreateBulkImportJob(r.Context(), orgID, totalRows)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to create import job", err.Error(), requestID)
		return
	}

	// Build response
	response := bulkimport.UploadResponse{
		Status:    "accepted",
		JobID:     job.ID.String(),
		StatusURL: fmt.Sprintf("/api/v1/assets/bulk/%s", job.ID.String()),
		Message:   fmt.Sprintf("CSV upload accepted. Processing %d rows asynchronously.", totalRows),
	}

	httputil.WriteJSON(w, http.StatusAccepted, response)

	// TODO Phase 2B: Launch goroutine here to process CSV rows
	// go handler.processCSVAsync(context.Background(), job.ID, orgID, csvContent, headers)
	_ = csvContent // Suppress unused variable warning for now
	_ = headers    // Suppress unused variable warning for now
}
