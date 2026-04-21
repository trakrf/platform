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
// detail is caller-supplied from a short canonical set — see
// docs/superpowers/specs/2026-04-20-tra-407-contract-bugs-design.md.
func Respond401(w http.ResponseWriter, r *http.Request, detail, requestID string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="`+AuthRealm+`"`)
	WriteJSONError(w, r, http.StatusUnauthorized, apierrors.ErrUnauthorized,
		"Authentication required", detail, requestID)
}
