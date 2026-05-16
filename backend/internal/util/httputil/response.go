package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

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
//
// TRA-739 (BB42 F1): a match preceded by "://" is part of a legitimate URL
// (e.g. https://docs.trakrf.id/api/data-model) and is preserved verbatim;
// only bare host/path runs collapse to [internal]. Previously the regex
// collided with any cited documentation URL.
func sanitizeDetail(detail string) string {
	matches := modulePathPattern.FindAllStringIndex(detail, -1)
	if len(matches) == 0 {
		return detail
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		b.WriteString(detail[last:start])
		if start >= 3 && detail[start-3:start] == "://" {
			b.WriteString(detail[start:end])
		} else {
			b.WriteString("[internal]")
		}
		last = end
	}
	b.WriteString(detail[last:])
	return b.String()
}

// genericServerErrorDetail is the only detail string a 5xx response is
// allowed to expose to the client. TRA-673 / BB27 F1: pgx and other DB
// driver errors carry implementation details (column types, OIDs, binary
// encoding diagnostics) that fingerprint the stack and fail security
// review. The raw cause is still slog'd server-side with the request_id
// for correlation.
const genericServerErrorDetail = "An unexpected error occurred"

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
// Contract:
//   - title is derived from errType via errors.TitleForType — fixed per type,
//     never per call. Per-call context belongs in detail.
//   - detail is the specific, human-readable cause of this particular failure
//     (e.g. "asset id 999 is invalid", err.Error() text). May be empty when
//     the type alone fully describes the condition.
//
// Module paths in detail are scrubbed before the response is written so that
// internal package structure cannot leak through wrapped errors. 5xx
// responses additionally replace detail with a fixed generic message
// (TRA-673) so DB driver internals — pgx int4-encoding diagnostics, OIDs,
// SQLSTATE chatter — never reach the client. The original detail is
// retained in the server-side slog record for debugging.
func WriteJSONError(w http.ResponseWriter, r *http.Request, status int, errType errors.ErrorType, detail, requestID string) {
	rawDetail := detail
	detail = sanitizeDetail(detail)

	resp := ErrorResponse{}
	resp.Error.Type = string(errType)
	resp.Error.Title = errors.TitleForType(errType)
	resp.Error.Status = status
	resp.Error.Instance = r.URL.Path
	resp.Error.RequestID = requestID

	if status >= 500 {
		slog.Error("Error response",
			"status", status,
			"type", errType,
			"detail", rawDetail,
			"request_id", requestID,
			"path", r.URL.Path)
		resp.Error.Detail = genericServerErrorDetail
	} else {
		resp.Error.Detail = detail
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
func WriteJSONErrorWithFields(w http.ResponseWriter, r *http.Request, status int, errType errors.ErrorType, detail, requestID string, fields []errors.FieldError) {
	detail = sanitizeDetail(detail)
	resp := ErrorResponse{}
	resp.Error.Type = string(errType)
	resp.Error.Title = errors.TitleForType(errType)
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
