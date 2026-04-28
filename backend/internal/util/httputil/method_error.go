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
