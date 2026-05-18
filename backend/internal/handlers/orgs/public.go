package orgs

import (
	"net/http"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// OrgMeView is the minimal org identity returned by GET /api/v1/orgs/me.
// Customer-facing: just the fields integrators need to confirm the key is
// bound to the right org, plus the bearer's effective scopes and api_key_id
// so they can debug 403s without decoding the JWT locally (TRA-719 / BB35 A5).
type OrgMeView struct {
	ID       int      `json:"id"          example:"42"`
	Name     string   `json:"name"        example:"Acme Logistics"`
	Scopes   []string `json:"scopes"      example:"assets:read,assets:write"`
	APIKeyID string   `json:"api_key_id"  example:"550e8400-e29b-41d4-a716-446655440000"`
}

// GetOrgMeResponse is the typed envelope returned by GET /api/v1/orgs/me.
type GetOrgMeResponse struct {
	Data OrgMeView `json:"data"`
}

// @Summary Get the org associated with the authenticated API key
// @Description Returns the organization scoped by the API key presented in Authorization. Intended as a lightweight health-check primitive for integrators verifying a key works end-to-end. Singleton endpoint; accepts no query parameters. Any query parameter returns `400 validation_error / unknown_field`.
// @Tags orgs,public
// @ID orgs.me
// @Accept json
// @Produce json
// @Success 200 {object} orgs.GetOrgMeResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 422 {object} modelerrors.ErrorResponse "missing_org_context — the API key authenticated but its org no longer exists"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Header  429 {integer} Retry-After "Seconds to wait before retrying"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/orgs/me [get]
// GetOrgMe returns the org that the authenticated API key belongs to.
// Scoped to API-key auth (not session auth); serves as the canary endpoint
// customers hit to verify a key works end-to-end.
func (h *Handler) GetOrgMe(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	principal := middleware.GetAPIKeyPrincipal(r)
	if principal == nil {
		httputil.Respond401(w, r, "API key authentication required", reqID)
		return
	}

	org, err := h.storage.GetOrganizationByID(r.Context(), principal.OrgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to get organization", reqID)

		return
	}
	if org == nil {
		// Org was deleted between key issuance and this request — the key is
		// authentic but has no org context. Matches the docs' 422
		// missing_org_context contract (TRA-646 / BB22 S2).
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}

	// TRA-719 / BB35 A5: include scopes and api_key_id so integrators can
	// inspect their bearer's effective grant without decoding the JWT.
	// api_key_id mirrors the JWT `sub` claim (the api_keys row's jti UUID).
	// scopes is always a JSON array — empty slice rather than null so typed
	// clients can iterate without a nil check.
	scopes := principal.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":         org.ID,
			"name":       org.Name,
			"scopes":     scopes,
			"api_key_id": principal.JTI,
		},
	})
}
