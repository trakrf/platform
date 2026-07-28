package orgs

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/capability"
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

// @Summary Get an organization's capability grants (superadmin)
// @Description Superadmin-only (TRA-1027 / ADR 0002). Returns the org's granted
// @Description capability names alongside the full capability vocabulary, so a
// @Description grant UI can render every option from server truth.
// @Tags orgs,internal
// @ID orgs.admin.capabilities.get
// @Accept json
// @Produce json
// @Param id path int true "Organization id" minimum(1) format(int64)
// @Success 200 {object} map[string]any "data: organization.OrgCapabilitiesView"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security SessionAuth
// @Router /api/v1/orgs/{id}/capabilities [get]
// GetOrgCapabilities returns an org's capability grants for the superadmin
// grant surface. Authorization is enforced upstream by RequireSuperadmin.
func (h *Handler) GetOrgCapabilities(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	// An ungranted org and an org that does not exist both have an empty
	// capability set, so existence is checked explicitly rather than inferred
	// from the set — a superadmin editing a typo'd id must see 404, not an
	// empty grant list they can "save" into nothing.
	org, err := h.storage.GetOrganizationByID(r.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgCapabilitiesFailed, middleware.GetRequestID(r.Context()))

		return
	}
	if org == nil {
		httputil.Respond404(w, r, apierrors.OrgNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	caps, err := h.storage.OrgCapabilitySet(r.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgCapabilitiesFailed, middleware.GetRequestID(r.Context()))

		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": organization.OrgCapabilitiesView{Capabilities: caps, Available: capability.All},
	})
}

// @Summary Replace an organization's capability grants (superadmin)
// @Description Superadmin-only (TRA-1027 / ADR 0002). Replaces the org's grants
// @Description with the submitted set: names present are granted, names absent
// @Description are revoked. Idempotent. Takes effect on the org's next request —
// @Description grants are never baked into tokens, so no reissue is needed.
// @Tags orgs,internal
// @ID orgs.admin.capabilities.set
// @Accept json
// @Produce json
// @Param id path int true "Organization id" minimum(1) format(int64)
// @Param request body organization.SetOrgCapabilitiesRequest true "Capability set"
// @Success 200 {object} map[string]any "data: organization.OrgCapabilitiesView"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security SessionAuth
// @Router /api/v1/orgs/{id}/capabilities [put]
// SetOrgCapabilities replaces an org's capability grants.
// Authorization is enforced upstream by RequireSuperadmin.
func (h *Handler) SetOrgCapabilities(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.SetOrgCapabilitiesRequest
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

	// The lookup-table FK rejects an unknown name too, but as a 500-shaped
	// database error. Checking the code-owned registry first turns it into a
	// 400 that names the offending value.
	for _, name := range *request.Capabilities {
		if !capability.IsValid(name) {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
				fmt.Sprintf("unknown capability: %s", name), middleware.GetRequestID(r.Context()))

			return
		}
	}

	caps, err := h.storage.SetOrgCapabilities(r.Context(), id, *request.Capabilities)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.OrgCapabilitiesUpdateFailed, middleware.GetRequestID(r.Context()))

		return
	}

	if caps == nil {
		httputil.Respond404(w, r, apierrors.OrgNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": organization.OrgCapabilitiesView{Capabilities: caps, Available: capability.All},
	})
}
