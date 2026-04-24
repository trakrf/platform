package orgs

import (
	"net/http"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// OrgMeView is the minimal org identity returned by GET /api/v1/orgs/me.
// Customer-facing: just the fields integrators need to confirm the key is
// bound to the right org.
type OrgMeView struct {
	ID   int    `json:"id"   example:"42"`
	Name string `json:"name" example:"Acme Logistics"`
}

// GetOrgMeResponse is the typed envelope returned by GET /api/v1/orgs/me.
type GetOrgMeResponse struct {
	Data OrgMeView `json:"data"`
}

// @Summary Get the org associated with the authenticated API key
// @Description Returns the organization scoped by the API key presented in Authorization. Intended as a lightweight health-check primitive for integrators verifying a key works end-to-end.
// @Tags orgs,public
// @ID orgs.me
// @Accept json
// @Produce json
// @Success 200 {object} orgs.GetOrgMeResponse
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security APIKey
// @Router /api/v1/orgs/me [get]
// GetOrgMe returns the org that the authenticated API key belongs to.
// Scoped to API-key auth (not session auth); serves as the canary endpoint
// customers hit to verify a key works end-to-end.
func (h *Handler) GetOrgMe(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	principal := middleware.GetAPIKeyPrincipal(r)
	if principal == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", reqID)
		return
	}

	org, err := h.storage.GetOrganizationByID(r.Context(), principal.OrgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to get organization", "", reqID)
		return
	}
	if org == nil {
		// Org was deleted between key issuance and this request — treat the key as unauthorized.
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "Organization no longer exists", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":   org.ID,
			"name": org.Name,
		},
	})
}
