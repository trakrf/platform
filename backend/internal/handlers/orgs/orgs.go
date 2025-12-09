package orgs

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = validator.New()

type Handler struct {
	storage *storage.Storage
	service *orgsservice.Service
}

func NewHandler(storage *storage.Storage, service *orgsservice.Service) *Handler {
	return &Handler{storage: storage, service: service}
}

// List returns all organizations the authenticated user belongs to.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", middleware.GetRequestID(r.Context()))
		return
	}

	orgs, err := h.storage.ListUserOrgs(r.Context(), claims.UserID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgListFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": orgs})
}

// Create creates a new team organization with the creator as admin.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.CreateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgCreateInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.OrgCreateValidationFail, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	org, err := h.service.CreateOrgWithAdmin(r.Context(), request.Name, claims.UserID)
	if err != nil {
		if err.Error() == "organization identifier already taken" {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				"Organization identifier already taken", "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgCreateFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	w.Header().Set("Location", "/api/v1/orgs/"+strconv.Itoa(org.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": org})
}

// Get returns a single organization by ID.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	org, err := h.storage.GetOrganizationByID(r.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgGetFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	if org == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.OrgNotFound, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// Update updates an organization's name.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgUpdateInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.UpdateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgUpdateInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.OrgUpdateValidationFail, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	org, err := h.storage.UpdateOrganization(r.Context(), id, request)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgUpdateFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	if org == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.OrgUpdateNotFound, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// Delete soft-deletes an organization after confirming the name matches.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgDeleteInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.DeleteOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgDeleteInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	err = h.service.DeleteOrgWithConfirmation(r.Context(), id, request.ConfirmName)
	if err != nil {
		if err.Error() == "organization name does not match" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.OrgDeleteNameMismatch, "", middleware.GetRequestID(r.Context()))
			return
		}
		if err.Error() == "organization not found" {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				apierrors.OrgDeleteNotFound, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgDeleteFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Organization deleted"})
}

// RegisterRoutes registers org endpoints on the given router.
func (h *Handler) RegisterRoutes(r chi.Router, store middleware.OrgRoleStore) {
	// Public routes (any authenticated user)
	r.Get("/api/v1/orgs", h.List)
	r.Post("/api/v1/orgs", h.Create)

	// Protected routes (require org membership/admin)
	r.Route("/api/v1/orgs/{id}", func(r chi.Router) {
		r.With(middleware.RequireOrgMember(store)).Get("/", h.Get)
		r.With(middleware.RequireOrgAdmin(store)).Put("/", h.Update)
		r.With(middleware.RequireOrgAdmin(store)).Delete("/", h.Delete)

		// Member management routes
		r.With(middleware.RequireOrgMember(store)).Get("/members", h.ListMembers)
		r.With(middleware.RequireOrgAdmin(store)).Put("/members/{userId}", h.UpdateMemberRole)
		r.With(middleware.RequireOrgAdmin(store)).Delete("/members/{userId}", h.RemoveMember)
	})
}
