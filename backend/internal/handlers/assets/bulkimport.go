package assets

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// @Summary Get bulk import job status
// @Description Retrieve the status of a bulk import job by ID
// @Tags bulk-import,internal
// @Accept json
// @Produce json
// @Param jobId path int true "Job ID" minimum(1) maximum(2147483647) format(int32)
// @Success 200 {object} bulkimport.JobStatusResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid job ID"
// @Failure 404 {object} modelerrors.ErrorResponse "Job not found or access denied"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security SessionAuth
// @Router /api/v1/assets/bulk/{jobId} [get]
func (handler *Handler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())
	jobID, err := httputil.ParseSurrogateID("jobId", chi.URLParam(r, "jobId"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, requestID)
		return
	}

	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.Respond401(w, r, "Session authentication required", requestID)
		return
	}
	if claims.CurrentOrgID == nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	job, err := handler.storage.GetBulkImportJobByID(r.Context(), jobID, orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), requestID)

		return
	}

	if job == nil {
		httputil.Respond404(w, r, apierrors.BulkImportJobNotFound, requestID)
		return
	}

	response := bulkimport.JobStatusResponse{
		JobID:         fmt.Sprintf("%d", job.ID),
		Status:        job.Status,
		TotalRows:     job.TotalRows,
		ProcessedRows: job.ProcessedRows,
		FailedRows:    job.FailedRows,
		TagsCreated:   job.TagsCreated,
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
// @Tags bulk-import,internal
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "CSV file with assets"
// @Success 202 {object} bulkimport.UploadResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid file or headers"
// @Failure 413 {object} modelerrors.ErrorResponse "File too large"
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
// @Security SessionAuth
// @Router /api/v1/assets/bulk [post]
func (handler *Handler) UploadCSV(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.Respond401(w, r, "Session authentication required", requestID)
		return
	}
	if claims.CurrentOrgID == nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	err := r.ParseMultipartForm(6 * 1024 * 1024)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), requestID)

		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), requestID)

		return
	}
	defer file.Close()

	response, err := handler.bulkImportService.ProcessUpload(r.Context(), orgID, file, header)
	if err != nil {
		statusCode := http.StatusBadRequest
		errorType := modelerrors.ErrBadRequest

		errMsg := err.Error()
		if strings.Contains(errMsg, "file too large") {
			statusCode = http.StatusRequestEntityTooLarge
		} else if strings.Contains(errMsg, "failed to create import job") {
			statusCode = http.StatusInternalServerError
			errorType = modelerrors.ErrInternal
		}

		httputil.WriteJSONError(w, r, statusCode, errorType, err.Error(), requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, response)
}
