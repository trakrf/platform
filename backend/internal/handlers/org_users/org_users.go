package org_users

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TODO(TRA-94): Implement full CRUD handlers for org_users
// These endpoints are not used by auth flows (auth uses direct SQL queries).
// Proper REST API implementation deferred to follow-up task.

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// @Summary List org users
// @Description Get users in an organization
// @Tags org_users
// @Produce json
// @Router /api/v1/org_users [get]
func (handler *Handler) List(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Org users list endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

// @Summary Get org user
// @Description Get a specific user-org relationship
// @Tags org_users
// @Produce json
// @Router /api/v1/org_users/{orgId}/{userId} [get]
func (handler *Handler) Get(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Org user get endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

// @Summary Add user to organization
// @Description Create user-org relationship
// @Tags org_users
// @Produce json
// @Router /api/v1/org_users [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Org user create endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

// @Summary Update org user
// @Description Update user-org relationship
// @Tags org_users
// @Produce json
// @Router /api/v1/org_users/{orgId}/{userId} [put]
func (handler *Handler) Update(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Org user update endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

// @Summary Remove user from organization
// @Description Delete user-org relationship
// @Tags org_users
// @Produce json
// @Router /api/v1/org_users/{orgId}/{userId} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	httputil.WriteJSONError(w, r, http.StatusNotImplemented, errors.ErrInternal,
		"Org user delete endpoint not implemented", "", middleware.GetRequestID(r.Context()))
}

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/org_users", handler.List)
	r.Get("/api/v1/org_users/{orgId}/{userId}", handler.Get)
	r.Post("/api/v1/org_users", handler.Create)
	r.Put("/api/v1/org_users/{orgId}/{userId}", handler.Update)
	r.Delete("/api/v1/org_users/{orgId}/{userId}", handler.Delete)
}
