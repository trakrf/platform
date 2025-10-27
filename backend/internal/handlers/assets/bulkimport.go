package assets

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

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

	// Delegate to service for processing
	response, err := handler.bulkImportService.ProcessUpload(r.Context(), orgID, file, header)
	if err != nil {
		// Map service errors to appropriate HTTP status codes
		statusCode := http.StatusBadRequest
		errorType := modelerrors.ErrBadRequest

		// Check for specific error types
		errMsg := err.Error()
		if strings.Contains(errMsg, "file too large") {
			statusCode = http.StatusRequestEntityTooLarge
		} else if strings.Contains(errMsg, "failed to create import job") {
			statusCode = http.StatusInternalServerError
			errorType = modelerrors.ErrInternal
		}

		httputil.WriteJSONError(w, r, statusCode, errorType, "Upload failed", err.Error(), requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, response)
}
