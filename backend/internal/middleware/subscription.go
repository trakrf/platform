package middleware

import (
	"context"
	"net/http"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// EntitlementChecker reports whether an org may perform paid mutations.
// Satisfied by *storage.Storage (OrgIsEntitled).
type EntitlementChecker interface {
	OrgIsEntitled(ctx context.Context, orgID int) (bool, error)
}

// isMutation reports whether the method writes. Reads stay open regardless of
// entitlement (TRA-946: lapsed orgs keep read-only visibility).
func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// SubscriptionRequired gates paid mutations behind org entitlement (TRA-947).
// Apply it to route groups / routes that carry paid mutations. It:
//   - passes through all non-mutating methods (GET/HEAD/OPTIONS),
//   - passes through when no org context is resolvable (lets the auth layer 401),
//   - rejects a not-entitled mutation with 402 before the handler runs.
func SubscriptionRequired(checker EntitlementChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutation(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			orgID, err := GetRequestOrgID(r)
			if err != nil {
				// No org context — defer to the auth layer's 401.
				next.ServeHTTP(w, r)
				return
			}
			entitled, err := checker.OrgIsEntitled(r.Context(), orgID)
			if err != nil {
				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					apierrors.ErrInternal, "Failed to verify subscription entitlement",
					GetRequestID(r.Context()))
				return
			}
			if !entitled {
				httputil.Respond402PaymentRequired(w, r,
					"Organization subscription is not active or has expired",
					GetRequestID(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
