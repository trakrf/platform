package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/trakrf/platform/backend/internal/models/errors"
)

// modulePathPattern matches Go module paths (e.g. github.com/owner/repo/pkg.Func,
// gitlab.com/..., golang.org/x/...) that can leak into wrapped errors via
// fmt.Errorf("...%w", err) or stack traces. Public error responses must not
// expose internal package structure. Pattern matches host/path shape rather
// than a literal hostname so it covers any forge or vanity import path.
var modulePathPattern = regexp.MustCompile(`\b[a-z0-9][a-z0-9.-]*\.[a-z]{2,}/[A-Za-z0-9._/-]+`)

// sanitizeDetail scrubs internal module paths from a detail string before it
// reaches the client. The placeholder preserves message structure so callers
// can still read the surrounding cause text.
func sanitizeDetail(detail string) string {
	return modulePathPattern.ReplaceAllString(detail, "[internal]")
}

type ErrorResponse struct {
	Error struct {
		Type      string              `json:"type"`
		Title     string              `json:"title"`
		Status    int                 `json:"status"`
		Detail    string              `json:"detail"`
		Instance  string              `json:"instance"`
		RequestID string              `json:"request_id"`
		Fields    []errors.FieldError `json:"fields,omitempty"`
	} `json:"error"`
}

// WriteJSONError writes a standardized error response in RFC 7807 format.
//
// Contract for callers:
//   - title is a stable, machine-readable summary of what went wrong, suitable
//     for client-side branching (e.g. apierrors.AssetNotFound). Should not vary
//     between calls for the same condition.
//   - detail is the specific, human-readable cause of this particular failure
//     (e.g. err.Error() text or a templated message with the offending value).
//     May be empty when the title alone fully describes the condition.
//
// Module paths in detail are scrubbed before the response is written so that
// internal package structure cannot leak through wrapped errors.
func WriteJSONError(w http.ResponseWriter, r *http.Request, status int, errType errors.ErrorType, title, detail, requestID string) {
	detail = sanitizeDetail(detail)
	resp := ErrorResponse{}
	resp.Error.Type = string(errType)
	resp.Error.Title = title
	resp.Error.Status = status
	resp.Error.Detail = detail
	resp.Error.Instance = r.URL.Path
	resp.Error.RequestID = requestID

	if status >= 500 {
		slog.Error("Error response",
			"status", status,
			"type", errType,
			"detail", detail,
			"request_id", requestID,
			"path", r.URL.Path)
	} else {
		slog.Info("Client error",
			"status", status,
			"type", errType,
			"request_id", requestID,
			"path", r.URL.Path)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// WriteJSON writes a successful JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}

// WriteJSONErrorWithFields is WriteJSONError plus a populated fields[]
// array. Used by RespondValidationError.
func WriteJSONErrorWithFields(w http.ResponseWriter, r *http.Request, status int, errType errors.ErrorType, title, detail, requestID string, fields []errors.FieldError) {
	detail = sanitizeDetail(detail)
	resp := ErrorResponse{}
	resp.Error.Type = string(errType)
	resp.Error.Title = title
	resp.Error.Status = status
	resp.Error.Detail = detail
	resp.Error.Instance = r.URL.Path
	resp.Error.RequestID = requestID
	resp.Error.Fields = fields

	slog.Info("Validation error",
		"status", status,
		"type", errType,
		"request_id", requestID,
		"path", r.URL.Path,
		"field_count", len(fields))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
