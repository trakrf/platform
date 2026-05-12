package httputil

import (
	"net/http"
	"strings"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// Respond405 writes a normalized method-not-allowed response. Registered as
// chi's MethodNotAllowed handler on the root mux so unknown method/path
// combinations return the standard envelope instead of an empty body.
//
// allowed is the set of HTTP methods the route accepts, in canonical order.
// When non-empty, it is emitted as both the Allow response header (RFC 7231
// §6.5.5) and the human-readable error.detail. An empty allowed slice
// signals a programming error in the caller — at 405 time at least one
// method must match — but is handled defensively: the envelope is written
// without the Allow header and with empty detail.
func Respond405(w http.ResponseWriter, r *http.Request, allowed []string, requestID string) {
	detail := ""
	if len(allowed) > 0 {
		joined := strings.Join(allowed, ", ")
		w.Header().Set("Allow", joined)
		detail = "Allowed methods: " + joined
	}
	WriteJSONError(w, r, http.StatusMethodNotAllowed, apierrors.ErrMethodNotAllowed,
		detail, requestID)
}

// Respond415 writes a normalized unsupported-media-type response. Used by the
// ContentType middleware when a write request arrives with a content-type the
// public API spec does not declare.
//
// Detail is method-aware: PATCH operations declare only
// application/merge-patch+json per RFC 7396; POST and PUT declare
// application/json. The middleware still accepts multipart/form-data
// internally for the session-only bulk CSV endpoint, but that detail is
// not surfaced in the public-facing error.
func Respond415(w http.ResponseWriter, r *http.Request, requestID string) {
	detail := "Content-Type must be application/json"
	if r.Method == http.MethodPatch {
		detail = "Content-Type must be application/merge-patch+json on PATCH operations"
	}
	WriteJSONError(w, r, http.StatusUnsupportedMediaType, apierrors.ErrUnsupportedMedia,
		detail, requestID)
}

// RespondMissingOrgContext writes the canonical 422 envelope used when
// auth has succeeded but the request lacks an active organization context.
//
// Two real-world causes:
//   - Session-authenticated user has no current org (just signed up,
//     deleted last org, cleared client state). Frontend should route to
//     the org picker.
//   - API-key request with no org bound (shouldn't happen in production:
//     keys are minted per-org). Integrator should re-mint with the
//     correct org.
//
// Title and detail are both fixed; the variable cause does not surface
// per-call to keep the contract clean for client-side branching.
func RespondMissingOrgContext(w http.ResponseWriter, r *http.Request, requestID string) {
	WriteJSONError(w, r, http.StatusUnprocessableEntity, apierrors.ErrMissingOrgContext,
		"This request requires an active organization context. Select an organization or re-authenticate.",
		requestID)
}
