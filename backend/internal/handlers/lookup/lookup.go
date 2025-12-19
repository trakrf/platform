package lookup

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

// @Summary Lookup entity by tag
// @Description Find an asset or location by tag identifier value
// @Tags lookup
// @Accept json
// @Produce json
// @Param type query string true "Tag type (rfid, ble, barcode)"
// @Param value query string true "Tag value to search for"
// @Success 200 {object} map[string]any "data: storage.LookupResult"
// @Failure 400 {object} modelerrors.ErrorResponse "Missing required parameters"
// @Failure 404 {object} modelerrors.ErrorResponse "No entity found with this tag"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/lookup/tag [get]
func (h *Handler) LookupByTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LookupFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	tagType := r.URL.Query().Get("type")
	value := r.URL.Query().Get("value")

	if tagType == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LookupFailed, "type parameter is required", requestID)
		return
	}

	if value == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LookupFailed, "value parameter is required", requestID)
		return
	}

	result, err := h.storage.LookupByTagValue(r.Context(), orgID, tagType, value)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LookupFailed, err.Error(), requestID)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LookupNotFound, "", requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/lookup/tag", h.LookupByTag)
}
