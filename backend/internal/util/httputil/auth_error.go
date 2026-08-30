package httputil

import (
	"net/http"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// AuthRealm is the WWW-Authenticate realm returned with 401 responses.
const AuthRealm = "trakrf-api"

// Respond401 writes a normalized unauthorized response. All 401 call sites
// in public and internal handlers should route through this helper so the
// envelope, WWW-Authenticate header, and title are consistent.
//
// detail is caller-supplied from a short canonical set — see TRA-407.
func Respond401(w http.ResponseWriter, r *http.Request, detail, requestID string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="`+AuthRealm+`"`)
	WriteJSONError(w, r, http.StatusUnauthorized, apierrors.ErrUnauthorized,
		detail, requestID)
}

// Respond404 writes a normalized not-found response. All 404 call sites in
// public and internal handlers should route through this helper so the
// envelope and title are consistent and the variable explanation lives in
// detail.
//
// detail is caller-supplied, e.g. apierrors.AssetNotFound.
func Respond404(w http.ResponseWriter, r *http.Request, detail, requestID string) {
	WriteJSONError(w, r, http.StatusNotFound, apierrors.ErrNotFound,
		detail, requestID)
}

// Respond402PaymentRequired writes a normalized 402 for a not-entitled org
// (TRA-947 subscriptions lite). Distinct type/title from 401 (auth) and 403
// (RBAC) so the frontend branches to a renew/contact prompt rather than a
// login or permission prompt.
func Respond402PaymentRequired(w http.ResponseWriter, r *http.Request, detail, requestID string) {
	WriteJSONError(w, r, http.StatusPaymentRequired, apierrors.ErrPaymentRequired,
		detail, requestID)
}

// Respond403CapabilityRequired writes the uniform denial for a surface the org
// has not been granted (ADR 0002 / TRA-1025). 403 with a type distinct from
// role/scope `forbidden`: the principal's permissions are irrelevant, the
// organization has not licensed the surface, and the remedy is a sales
// conversation rather than a role change.
//
// There is deliberately no hide-404 variant. The platform is source-available,
// so route existence is confirmable from the repository and a 404 would conceal
// nothing a 403 reveals — one denial contract for integrators and test cycles.
func Respond403CapabilityRequired(w http.ResponseWriter, r *http.Request, detail, requestID string) {
	WriteJSONError(w, r, http.StatusForbidden, apierrors.ErrCapabilityRequired,
		detail, requestID)
}
