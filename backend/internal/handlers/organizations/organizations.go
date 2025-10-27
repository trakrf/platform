package organizations

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TODO(TRA-94): Implement full CRUD handlers for organizations
// These endpoints are not used by auth flows (auth uses direct SQL queries).
// Proper REST API implementation deferred to follow-up task.

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// @Summary List organizations
// @Description Get paginated organizations
// @Tags organizations
// @Produce json
// @Router /api/v1/organizations [get]
func (handler *Handler) List(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Organization list endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

// @Summary Get organization
// @Description Get organization by ID
// @Tags organizations
// @Produce json
// @Router /api/v1/organizations/{id} [get]
func (handler *Handler) Get(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Organization get endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

// @Summary Create organization
// @Description Create a new organization
// @Tags organizations
// @Produce json
// @Router /api/v1/organizations [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Organization create endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

// @Summary Update organization
// @Description Update an existing organization
// @Tags organizations
// @Produce json
// @Router /api/v1/organizations/{id} [put]
func (handler *Handler) Update(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Organization update endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

// @Summary Delete organization
// @Description Soft delete an organization
// @Tags organizations
// @Produce json
// @Router /api/v1/organizations/{id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Organization delete endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/organizations", handler.List)
	r.Get("/api/v1/organizations/{id}", handler.Get)
	r.Post("/api/v1/organizations", handler.Create)
	r.Put("/api/v1/organizations/{id}", handler.Update)
	r.Delete("/api/v1/organizations/{id}", handler.Delete)
}
