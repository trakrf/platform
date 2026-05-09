package httputil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"time"

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

// DecodeJSONStrictWithPresence is DecodeJSONStrict that additionally returns
// the set of top-level keys that appeared in the request body. Pair with
// RespondValidationErrorWithPresence so missing required fields are reported
// as code=required while present-but-empty values stay as code=too_short
// (TRA-641 / BB21 §2.2).
//
// A non-object body produces an empty key set and the usual strict-decode
// failure.
func DecodeJSONStrictWithPresence(r *http.Request, dst any) (map[string]struct{}, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, &JSONDecodeError{Cause: err}
	}
	present := map[string]struct{}{}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) == nil {
		for k := range raw {
			present[k] = struct{}{}
		}
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return present, &JSONDecodeError{Cause: err}
	}
	return present, nil
}

// DecodeJSONStrictWithNulls is DecodeJSONStrict that additionally reports
// which top-level JSON keys held the explicit literal `null`. Use on
// PATCH / PUT endpoints where `null` has semantic meaning distinct from
// "field omitted" (e.g., clear the field in the database).
//
// A non-object body (array, string, number) yields the usual strict-decode
// failure and an empty null set.
func DecodeJSONStrictWithNulls(r *http.Request, dst any) (map[string]struct{}, error) {
	return DecodeJSONStrictWithNullsTolerant(r, dst, nil)
}

// DecodeJSONStrictWithNullsTolerant is DecodeJSONStrictWithNulls that
// additionally drops the named top-level keys from the body before strict
// decoding. Use on PUT / PATCH endpoints to allow a verbatim GET → PUT
// round-trip: read-only response fields (id, created_at, updated_at, tags,
// …) are silently ignored, while typo'd or otherwise unknown fields still
// produce a 400 (TRA-608 / BB18 §1.7).
//
// The drop set should mirror the readOnly fields on the corresponding
// PublicXxxView in the OpenAPI spec; the asset and location packages export
// PublicReadOnlyFields for this purpose.
//
// Returns the set of explicit-null keys (after the drop set is applied).
// Use DecodeJSONStrictWithNullsTolerantAndPresence when you also need the
// full set of keys present in the body (e.g. for TRA-641 required vs
// too_short discrimination on PUT bodies).
func DecodeJSONStrictWithNullsTolerant(r *http.Request, dst any, drop []string) (map[string]struct{}, error) {
	nulls, _, err := decodeStrictWithNullsTolerant(r, dst, drop)
	return nulls, err
}

// DecodeJSONStrictWithNullsTolerantAndPresence is DecodeJSONStrictWithNullsTolerant
// that additionally returns the full set of top-level keys present in the
// request body (after the drop set is applied). Pair with
// RespondValidationErrorWithPresence so missing required fields surface as
// code=required while present-but-empty values keep code=too_short
// (TRA-641 / BB21 §2.2).
func DecodeJSONStrictWithNullsTolerantAndPresence(r *http.Request, dst any, drop []string) (nulls, present map[string]struct{}, err error) {
	return decodeStrictWithNullsTolerant(r, dst, drop)
}

func decodeStrictWithNullsTolerant(r *http.Request, dst any, drop []string) (map[string]struct{}, map[string]struct{}, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, nil, &JSONDecodeError{Cause: err}
	}

	explicitNulls := map[string]struct{}{}
	present := map[string]struct{}{}
	var raw map[string]json.RawMessage
	objectBody := json.Unmarshal(body, &raw) == nil

	if objectBody {
		for k, v := range raw {
			if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				explicitNulls[k] = struct{}{}
			}
		}
		if len(drop) > 0 {
			mutated := false
			for _, k := range drop {
				if _, ok := raw[k]; ok {
					delete(raw, k)
					delete(explicitNulls, k)
					mutated = true
				}
			}
			if mutated {
				body, err = json.Marshal(raw)
				if err != nil {
					return nil, nil, &JSONDecodeError{Cause: err}
				}
			}
		}
		for k := range raw {
			present[k] = struct{}{}
		}
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if decErr := dec.Decode(dst); decErr != nil {
		return nil, present, &JSONDecodeError{Cause: decErr}
	}
	return explicitNulls, present, nil
}

// RespondDecodeError writes a 400 with a stable, human-safe detail string.
// Use this as the failure branch partner of DecodeJSON. An unknown-field
// error surfaces as a validation_error keyed on the offending field so
// clients can branch on type+fields[].code like any other body failure.
// Other decode failures (syntax, truncated input) stay as bad_request
// because there is no field name to attach.
func RespondDecodeError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	if err != nil {
		re := regexp.MustCompile(`unknown field "([^"]+)"`)
		if matches := re.FindStringSubmatch(err.Error()); len(matches) > 1 {
			fieldName := matches[1]
			msg := fmt.Sprintf("unknown field %q in request body", fieldName)
			WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
				msg, requestID,
				[]apierrors.FieldError{{
					Field:   fieldName,
					Code:    "invalid_value",
					Message: msg,
				}})
			return
		}

		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			// Format-validation failures from custom UnmarshalJSON on date types
			// reach us as *json.UnmarshalTypeError with Type == time.Time
			// (TRA-641 / BB21 §2.1). Surface those as validation_error with
			// fields[] populated so clients branch on type=validation_error +
			// fields[].field, like every other field-level body failure. The
			// scalar-type-mismatch case (e.g. {"count":"x"} when count is int)
			// stays as bad_request because no per-field validation pass would
			// have caught it either.
			if isTimeTarget(typeErr.Type) {
				field := typeErr.Field
				if field == "" {
					field = "(body)"
				}
				msg := fmt.Sprintf("%s must be an RFC3339 date or datetime string", field)
				WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
					msg, requestID,
					[]apierrors.FieldError{{
						Field:   field,
						Code:    "invalid_value",
						Message: msg,
					}})
				return
			}
			detail := typeMismatchDetail(typeErr)
			WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
				detail, requestID)
			return
		}
	}
	WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
		"Request body is not valid JSON", requestID)
}

// typeMismatchDetail renders a stable detail string for a json type-mismatch
// failure: the body parsed as JSON, but a value did not fit its destination
// Go field. Returns a generic message when no field name is available
// (e.g., the entire body was a JSON array where an object was expected).
func typeMismatchDetail(e *json.UnmarshalTypeError) string {
	if e.Field != "" {
		return fmt.Sprintf("Body field %q could not be decoded as the expected type", e.Field)
	}
	return "Request body could not be decoded as the expected type"
}

// isTimeTarget reports whether t is time.Time or *time.Time, including
// embedded variants such as shared.FlexibleDate which wraps time.Time.
// Used by RespondDecodeError to detect format-validation failures that
// originate from a custom UnmarshalJSON on a date type so the response can
// be rendered as a validation_error rather than a generic bad_request.
func isTimeTarget(t reflect.Type) bool {
	if t == nil {
		return false
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t == reflect.TypeOf(time.Time{})
}
