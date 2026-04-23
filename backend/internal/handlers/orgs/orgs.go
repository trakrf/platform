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

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	return v
}()

type Handler struct {
	storage *storage.Storage
	service *orgsservice.Service
}

func NewHandler(storage *storage.Storage, service *orgsservice.Service) *Handler {
	return &Handler{storage: storage, service: service}
}

// @Summary List organizations the authenticated user belongs to
// @Tags orgs,internal
// @ID orgs.list
// @Accept json
// @Produce json
// @Success 200 {object} map[string]any "data: []organization.Organization"
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs [get]
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

// @Summary Create a new organization
// @Description Creates a team organization with the caller as admin. SPA-only — integrators have a fixed org scoped to their API key.
// @Tags orgs,internal
// @ID orgs.create
// @Accept json
// @Produce json
// @Param request body organization.CreateOrganizationRequest true "Organization to create"
// @Success 201 {object} map[string]any "data: organization.Organization"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 409 {object} modelerrors.ErrorResponse "Identifier already taken"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs [post]
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

// @Summary Get an organization by id
// @Tags orgs,internal
// @ID orgs.get
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Success 200 {object} map[string]any "data: organization.Organization"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id} [get]
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

// @Summary Update an organization's name
// @Tags orgs,internal
// @ID orgs.update
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param request body organization.UpdateOrganizationRequest true "Update payload"
// @Success 200 {object} map[string]any "data: organization.Organization"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id} [put]
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

// @Summary Soft-delete an organization
// @Description Requires the caller to repeat the organization name as a confirmation in the request body.
// @Tags orgs,internal
// @ID orgs.delete
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param request body organization.DeleteOrganizationRequest true "Confirmation payload"
// @Success 200 {object} map[string]any "message: Organization deleted"
// @Failure 400 {object} modelerrors.ErrorResponse "Name mismatch or invalid id"
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id} [delete]
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

		// Invitation routes (admin only)
		r.With(middleware.RequireOrgAdmin(store)).Get("/invitations", h.ListInvitations)
		r.With(middleware.RequireOrgAdmin(store)).Post("/invitations", h.CreateInvitation)
		r.With(middleware.RequireOrgAdmin(store)).Delete("/invitations/{inviteId}", h.CancelInvitation)
		r.With(middleware.RequireOrgAdmin(store)).Post("/invitations/{inviteId}/resend", h.ResendInvitation)
	})
}

// RegisterAPIKeyRoutes registers the /api/v1/orgs/{id}/api-keys endpoints.
// Registered SEPARATELY from RegisterRoutes because these routes accept api-key
// auth via keys:admin scope — they must live under an EitherAuth group, not
// the session-only middleware.Auth group used by the rest of the org subtree.
func (h *Handler) RegisterAPIKeyRoutes(r chi.Router, store middleware.OrgRoleStore) {
	r.Route("/api/v1/orgs/{id}/api-keys", func(r chi.Router) {
		r.Use(middleware.RequireOrgAdminOrKeysAdmin(store))
		r.Post("/", h.CreateAPIKey)
		r.Get("/", h.ListAPIKeys)
		r.Delete("/{keyID}", h.RevokeAPIKey)
	})
}
