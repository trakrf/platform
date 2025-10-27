package bulkimport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
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
func (h *Handler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())
	jobIDParam := chi.URLParam(r, "jobId")

	// Parse job ID as UUID
	jobID, err := uuid.Parse(jobIDParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid job ID format", err.Error(), requestID)
		return
	}

	// Extract account_id from JWT claims for tenant isolation
	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Missing account context", "", requestID)
		return
	}
	accountID := *claims.CurrentOrgID

	// Retrieve job from storage (with tenant isolation)
	job, err := h.storage.GetBulkImportJobByID(r.Context(), jobID, accountID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to retrieve job", err.Error(), requestID)
		return
	}

	if job == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			"Job not found or does not belong to your account", "", requestID)
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

// RegisterRoutes registers all bulk import routes
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/assets/bulk/{jobId}", h.GetJobStatus)
}
