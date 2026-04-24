package httputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

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

// DecodeJSONStrictWithNulls is DecodeJSONStrict that additionally reports
// which top-level JSON keys held the explicit literal `null`. Use on
// PATCH / PUT endpoints where `null` has semantic meaning distinct from
// "field omitted" (e.g., clear the field in the database).
//
// A non-object body (array, string, number) yields the usual strict-decode
// failure and an empty null set.
func DecodeJSONStrictWithNulls(r *http.Request, dst any) (map[string]struct{}, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, &JSONDecodeError{Cause: err}
	}

	explicitNulls := map[string]struct{}{}
	var raw map[string]json.RawMessage
	if jsonErr := json.Unmarshal(body, &raw); jsonErr == nil {
		for k, v := range raw {
			if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				explicitNulls[k] = struct{}{}
			}
		}
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if decErr := dec.Decode(dst); decErr != nil {
		return nil, &JSONDecodeError{Cause: decErr}
	}
	return explicitNulls, nil
}

// RespondDecodeError writes a 400 with a stable, human-safe detail string.
// Use this as the failure branch partner of DecodeJSON.
// If the error is a DisallowUnknownFields error, the detail includes the field name.
func RespondDecodeError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	detail := "Request body is not valid JSON"
	// Extract field name from json.SyntaxError for unknown field errors
	if err != nil {
		errStr := err.Error()
		// Match "json: unknown field "fieldname"" pattern
		re := regexp.MustCompile(`unknown field "([^"]+)"`)
		if matches := re.FindStringSubmatch(errStr); len(matches) > 1 {
			fieldName := matches[1]
			detail = fmt.Sprintf("Request body is not valid JSON: unknown field %q", fieldName)
		}
	}
	WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
		"Bad Request", detail, requestID)
}
