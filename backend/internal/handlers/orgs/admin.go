package orgs

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// @Summary List all organizations (superadmin)
// @Description Superadmin-only cross-org list (TRA-949). Returns every org with
// @Description its entitlement state and member count, regardless of membership.
// @Tags orgs,internal
// @ID orgs.admin.list
// @Accept json
// @Produce json
// @Success 200 {object} map[string]any "data: []organization.AdminOrgListItem"
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security SessionAuth
// @Router /api/v1/admin/orgs [get]
// ListAllOrgs returns every organization for the superadmin all-orgs list.
// Authorization is enforced upstream by RequireSuperadmin.
func (h *Handler) ListAllOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.storage.ListAllOrgs(r.Context())
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgListFailed, middleware.GetRequestID(r.Context()))

		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": orgs})
}

// @Summary Set an organization's manual entitlement (superadmin)
// @Description Superadmin-only (TRA-949). Sets subscription_enabled and
// @Description subscription_expires_at (null = never expires). Takes effect on
// @Description the next entitlement check. Regular org admins are rejected (403).
// @Tags orgs,internal
// @ID orgs.admin.entitlement
// @Accept json
// @Produce json
// @Param id path int true "Organization id" minimum(1) format(int64)
// @Param request body organization.UpdateEntitlementRequest true "Entitlement payload"
// @Success 200 {object} map[string]any "data: organization.Organization"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security SessionAuth
// @Router /api/v1/orgs/{id}/entitlement [patch]
// UpdateEntitlement sets an org's manual entitlement kill switch and expiry.
// Authorization is enforced upstream by RequireSuperadmin.
func (h *Handler) UpdateEntitlement(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.UpdateEntitlementRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	org, err := h.storage.UpdateOrgEntitlement(r.Context(), id,
		*request.SubscriptionEnabled, request.SubscriptionExpiresAt)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgUpdateFailed, middleware.GetRequestID(r.Context()))

		return
	}

	if org == nil {
		httputil.Respond404(w, r, apierrors.OrgUpdateNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}
