package orgs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// GetMeResponse is the typed envelope returned by GET /api/v1/users/me.
type GetMeResponse struct {
	Data organization.UserProfile `json:"data"`
}

// SetCurrentOrgResponse is returned by POST /api/v1/users/me/current-org.
type SetCurrentOrgResponse struct {
	Message string `json:"message" example:"Current organization updated"`
	Token   string `json:"token"   example:"eyJhbGciOiJIUzI1NiIsInR5cCI6..."`
}

// @Summary Get the authenticated user's profile with org memberships
// @Description Returns the caller's user record alongside the organizations they belong to. Used by the SPA to render the user menu and org picker.
// @Tags users,internal
// @ID users.me
// @Accept json
// @Produce json
// @Success 200 {object} orgs.GetMeResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/users/me [get]
// GetMe returns the authenticated user's profile with orgs.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", middleware.GetRequestID(r.Context()))
		return
	}

	profile, err := h.service.GetUserProfile(r.Context(), claims.UserID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to get user profile", "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// @Summary Switch the authenticated user's current organization
// @Description SPA org-switcher. Issues a fresh session JWT scoped to the selected org. API-key auth has a fixed org — no analog exists for integrators. Note: route is POST (not GET as some earlier docs suggested).
// @Tags users,internal
// @ID users.set_current_org
// @Accept json
// @Produce json
// @Param request body organization.SetCurrentOrgRequest true "Org to switch to"
// @Success 200 {object} orgs.SetCurrentOrgResponse
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse "Not a member of the target org"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/users/me/current-org [post]
// SetCurrentOrg updates the user's current organization.
func (h *Handler) SetCurrentOrg(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.SetCurrentOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid JSON", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			"Validation failed", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := h.service.SetCurrentOrg(r.Context(), claims.UserID, request.OrgID); err != nil {
		if errors.Is(err, storage.ErrOrgUserNotFound) {
			httputil.WriteJSONError(w, r, http.StatusForbidden, modelerrors.ErrForbidden,
				apierrors.OrgNotMember, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgNotMember, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	// Generate new JWT with updated org_id
	token, err := jwt.Generate(claims.UserID, claims.Email, &request.OrgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to generate token", "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"message": "Current organization updated",
		"token":   token,
	})
}

// RegisterMeRoutes registers /users/me endpoints.
func (h *Handler) RegisterMeRoutes(r chi.Router) {
	r.Get("/api/v1/users/me", h.GetMe)
	r.Post("/api/v1/users/me/current-org", h.SetCurrentOrg)
}
