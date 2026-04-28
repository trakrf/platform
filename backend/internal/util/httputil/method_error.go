package httputil

import (
	"net/http"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// Respond405 writes a normalized method-not-allowed response. Registered as
// chi's MethodNotAllowed handler on the root mux so unknown method/path
// combinations return the standard envelope instead of an empty body.
//
// detail is intentionally empty — the path and method are already in the
// access log; no useful per-call variability remains.
func Respond405(w http.ResponseWriter, r *http.Request, requestID string) {
	WriteJSONError(w, r, http.StatusMethodNotAllowed, apierrors.ErrMethodNotAllowed,
		"Method not allowed", "", requestID)
}

// Respond415 writes a normalized unsupported-media-type response. Used by the
// ContentType middleware when a write request arrives with a content-type the
// public API spec does not declare.
//
// detail is fixed: the public OpenAPI spec only declares application/json on
// every endpoint, so the message names exactly that. The middleware still
// accepts multipart/form-data internally for the session-only bulk CSV
// endpoint, but that detail is not surfaced in the public-facing error.
func Respond415(w http.ResponseWriter, r *http.Request, requestID string) {
	WriteJSONError(w, r, http.StatusUnsupportedMediaType, apierrors.ErrUnsupportedMedia,
		"Unsupported media type", "Content-Type must be application/json", requestID)
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
		"Organization context required",
		"This request requires an active organization context. Select an organization or re-authenticate.",
		requestID)
}
