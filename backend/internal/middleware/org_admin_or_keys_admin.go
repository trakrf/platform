package middleware

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// RequireOrgAdminOrKeysAdmin accepts either a session admin of the target org
// OR an API-key principal with the "keys:admin" scope whose principal.OrgID
// matches {id} in the URL. Must be chained AFTER EitherAuth.
func RequireOrgAdminOrKeysAdmin(store OrgRoleStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			// 1. Session principal → delegate to existing org-admin check.
			if GetUserClaims(r) != nil {
				RequireOrgAdmin(store)(next).ServeHTTP(w, r)
				return
			}

			// 2. API-key principal → require keys:admin + matching org.
			if p := GetAPIKeyPrincipal(r); p != nil {
				orgIDStr := chi.URLParam(r, "id")
				orgID, err := strconv.Atoi(orgIDStr)
				if err != nil {
					httputil.WriteJSONError(w, r, http.StatusBadRequest,
						errors.ErrBadRequest, "Bad Request", "Invalid organization ID", reqID)
					return
				}
				if p.OrgID != orgID {
					httputil.WriteJSONError(w, r, http.StatusForbidden,
						errors.ErrForbidden, "Forbidden",
						"API key is not authorized for this organization", reqID)
					return
				}
				for _, s := range p.Scopes {
					if s == "keys:admin" {
						next.ServeHTTP(w, r)
						return
					}
				}
				httputil.WriteJSONError(w, r, http.StatusForbidden,
					errors.ErrForbidden, "Forbidden",
					"Missing required scope: keys:admin", reqID)
				return
			}

			// 3. No principal (defensive — EitherAuth should have rejected).
			httputil.Respond401(w, r, "Authorization required", reqID)
		})
	}
}
