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
	"strings"
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return &JSONDecodeError{Cause: err}
	}
	if err := rejectNULByteBody(body); err != nil {
		return err
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(dst); err != nil {
		return &JSONDecodeError{Cause: err}
	}
	return nil
}

// DecodeJSONStrict is DecodeJSON with DisallowUnknownFields. Use on
// public API endpoints where unrecognised body fields should produce a
// 400 rather than being silently ignored.
func DecodeJSONStrict(r *http.Request, dst any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return &JSONDecodeError{Cause: err}
	}
	if err := rejectNULByteBody(body); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &JSONDecodeError{Cause: err}
	}
	return nil
}

// rejectNULByteBody returns a JSONDecodeError when the raw body bytes
// contain a NUL byte. Postgres TEXT columns reject NUL outright (SQLSTATE
// 22021); a NUL anywhere in the body — including nested JSON strings
// inside `metadata` or other free-form objects — would land in pgx as a
// 5xx (TRA-678). Pre-screening at the boundary turns this into a
// deterministic 400 before any decode work happens.
func rejectNULByteBody(body []byte) error {
	if bytes.IndexByte(body, 0x00) >= 0 {
		return &JSONDecodeError{Cause: errors.New("request body must not contain NUL bytes")}
	}
	return nil
}

// DecodeJSONStrictWithPresence is DecodeJSONStrict that additionally returns
// the set of top-level keys that appeared in the request body. Callers use
// the presence map for per-field branching that encoding/json cannot signal
// on its own — e.g. distinguishing an absent optional natural-key field
// from an explicitly empty one on Create. The validation envelope itself no
// longer needs presence: length-bearing required fields report too_short
// (with min_length=1) whether the field was empty or omitted (TRA-675).
//
// A non-object body produces an empty key set and the usual strict-decode
// failure.
func DecodeJSONStrictWithPresence(r *http.Request, dst any) (map[string]struct{}, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, &JSONDecodeError{Cause: err}
	}
	if err := rejectNULByteBody(body); err != nil {
		return nil, err
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
// full set of keys present in the body for non-validator per-field
// branching.
func DecodeJSONStrictWithNullsTolerant(r *http.Request, dst any, drop []string) (map[string]struct{}, error) {
	nulls, _, err := decodeStrictWithNullsTolerant(r, dst, drop)
	return nulls, err
}

// DecodeJSONStrictWithNullsTolerantAndPresence is DecodeJSONStrictWithNullsTolerant
// that additionally returns the full set of top-level keys present in the
// request body (after the drop set is applied). Use the presence map for
// per-field branching where the validator's tag-driven path cannot
// distinguish absent from explicit-zero (e.g. optional alternate-natural-key
// fields).
func DecodeJSONStrictWithNullsTolerantAndPresence(r *http.Request, dst any, drop []string) (nulls, present map[string]struct{}, err error) {
	return decodeStrictWithNullsTolerant(r, dst, drop)
}

// FieldRejectPolicy is the per-field rule used by RejectFields: if the
// field is present in the PATCH body, emit a 400 validation_error with
// the configured code and message.
//
// TRA-686 / BB29 F7+F8: PATCH validators distinguish three categories of
// fields the body might carry — round-trip-safe read-onlys (silent drop),
// managed-via-subresource (reject with invalid_value), and
// managed-via-rename (reject with read_only). The first stays on the
// strip list (PublicReadOnlyFields); the other two each use a
// FieldRejectPolicy with the appropriate code and a message naming the
// dedicated endpoint.
type FieldRejectPolicy struct {
	Code    string
	Message string
}

// RejectFields peeks at the request body for any of the named top-level
// keys and, if any are present, writes a 400 validation_error with the
// per-field code/message from the policy map. Returns true if the response
// was written and the caller should return; false if the body is clean
// (and r.Body has been replaced with a fresh reader that the downstream
// decoder can still consume).
//
// A non-object body is left to the downstream decoder. An empty `policies`
// map is a no-op.
//
// TRA-664 / BB26 D7 introduced the pre-decode reject for external_key;
// TRA-686 / BB29 F7+F8 generalized it so PATCH validators can reject
// `tags` (managed via subresource → invalid_value) alongside
// `external_key` / `parent_external_key` (managed via rename → read_only)
// without falling back to the strip-and-silently-ignore default.
func RejectFields(w http.ResponseWriter, r *http.Request, requestID string, policies map[string]FieldRejectPolicy) bool {
	if len(policies) == 0 {
		return false
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
			"Request body could not be read", requestID)
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		// Not an object body — downstream decoder will surface the parse error.
		return false
	}

	var violations []apierrors.FieldError
	for field, policy := range policies {
		if _, present := raw[field]; !present {
			continue
		}
		violations = append(violations, apierrors.FieldError{
			Field:   field,
			Code:    policy.Code,
			Message: policy.Message,
		})
	}
	if len(violations) == 0 {
		return false
	}

	// Detail: take the first violation's message. Most callers only ever
	// hit one rejected field per resource, and even when several appear,
	// branching happens on fields[].code, not the detail string.
	WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
		violations[0].Message, requestID, violations)
	return true
}

func decodeStrictWithNullsTolerant(r *http.Request, dst any, drop []string) (map[string]struct{}, map[string]struct{}, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, nil, &JSONDecodeError{Cause: err}
	}
	if err := rejectNULByteBody(body); err != nil {
		return nil, nil, err
	}

	// A literal `null` parses successfully into any struct destination as a
	// silent no-op — every field stays at the Go zero value and the handler
	// has no signal that the body was structurally invalid. Schemathesis
	// flags this as "API accepted schema-violating request" on PATCH because
	// the spec declares the body as type:object (TRA-678). Reject upfront so
	// the response is a 400 bad_request, not a no-op 200.
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, nil, &JSONDecodeError{Cause: errors.New("request body must be a JSON object, not null")}
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
					Code:    "unknown_field",
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
				// encoding/json prefixes the field path with the struct
				// name when an embedded struct is in play (e.g.
				// CreateAssetWithTagsRequest embeds CreateAssetRequest, so
				// a date failure on `valid_from` arrives as
				// "CreateAssetRequest.valid_from"). The wire-facing field
				// is the JSON-tag leaf — keep only the segment after the
				// last "." so the response matches the request body
				// shape integrators see.
				if i := strings.LastIndex(field, "."); i >= 0 {
					field = field[i+1:]
				}
				if field == "" {
					field = "(body)"
				}
				msg := fmt.Sprintf("%s must be an RFC 3339 timestamp", field)
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
