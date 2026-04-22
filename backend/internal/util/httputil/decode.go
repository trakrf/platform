package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// JSONDecodeError wraps any encoding/json decode failure so callers can
// render a stable response without leaking parser internals.
type JSONDecodeError struct {
	Cause error
}

func (e *JSONDecodeError) Error() string {
	return fmt.Sprintf("json decode: %v", e.Cause)
}

func (e *JSONDecodeError) Unwrap() error { return e.Cause }

// DecodeJSON decodes the request body into dst. Wraps any decode failure
// in *JSONDecodeError so the caller does not surface encoding/json
// internals to the client.
func DecodeJSON(r *http.Request, dst any) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return &JSONDecodeError{Cause: err}
	}
	return nil
}

// DecodeJSONStrict is DecodeJSON with DisallowUnknownFields. Use on
// public API endpoints where unrecognised body fields should produce a
// 400 rather than being silently ignored.
func DecodeJSONStrict(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &JSONDecodeError{Cause: err}
	}
	return nil
}

// RespondDecodeError writes a 400 with a stable, human-safe detail string.
// Use this as the failure branch partner of DecodeJSON.
func RespondDecodeError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
		"Bad Request", "Request body is not valid JSON", requestID)
}
