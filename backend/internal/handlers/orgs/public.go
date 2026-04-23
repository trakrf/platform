package orgs

import (
	"net/http"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// GetOrgMe returns the org that the authenticated API key belongs to.
// Scoped to API-key auth (not session auth); serves as the canary endpoint
// customers hit to verify a key works end-to-end before TRA-396 lands.
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
