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
	"sort"
	"strings"
	"time"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/shared"
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

// JSONNullBodyError signals that the request body was the literal JSON
// `null`. The downstream renderer uses this to produce a wording that
// names RFC 7396 instead of the generic "not valid JSON" fallback (TRA-707
// / BB32 C3). `null` is structurally valid JSON; the rejection itself is
// correct, only the wording misdiagnosed.
type JSONNullBodyError struct{}

func (e *JSONNullBodyError) Error() string {
	return "request body must be a JSON object (RFC 7396), not null"
}

// JSONUnknownFieldsError carries every unknown top-level key found in a
// strict-decode request body. encoding/json's DisallowUnknownFields stops at
// the first unknown field, but the public API's docs commit to one fields[]
// entry per invalid field (TRA-702 / BB32 D3) — so the strict-decode
// helpers do the enumeration up-front via reflection on the destination
// struct and surface every offending key here.
//
// Fields is sorted lexically so test assertions and client-side branching
// see a deterministic order.
type JSONUnknownFieldsError struct {
	Fields []string
}

func (e *JSONUnknownFieldsError) Error() string {
	if len(e.Fields) == 1 {
		return fmt.Sprintf("json: unknown field %q", e.Fields[0])
	}
	return fmt.Sprintf("json: unknown fields %v", e.Fields)
}

// knownJSONTags returns the set of top-level JSON tag names declared on the
// destination struct (or *struct). Embedded anonymous structs are walked
// because encoding/json promotes their fields to the parent level. Unexported
// fields and json:"-" tags are skipped. Returns an empty map when dst is not
// a struct kind — callers treat that as "no precheck possible" and let the
// downstream strict decoder fail naturally.
func knownJSONTags(dst any) map[string]struct{} {
	out := map[string]struct{}{}
	t := reflect.TypeOf(dst)
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return out
	}
	collectKnownJSONTags(t, out)
	return out
}

func collectKnownJSONTags(t reflect.Type, out map[string]struct{}) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() && !f.Anonymous {
			continue
		}
		if f.Anonymous {
			inner := f.Type
			for inner.Kind() == reflect.Ptr {
				inner = inner.Elem()
			}
			if inner.Kind() == reflect.Struct {
				collectKnownJSONTags(inner, out)
				continue
			}
		}
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.SplitN(tag, ",", 2)[0]
		if name != "" && name != "-" {
			out[name] = struct{}{}
		}
	}
}

// precheckUnknownFields enumerates every top-level key in raw that does not
// map to a json tag on dst, ignoring keys named in skip (the PATCH drop set
// for round-trip-safe read-only fields). Returns a *JSONUnknownFieldsError
// when one or more unknowns are present; nil otherwise. The strict decoder
// would catch one of them, but only one, hence the explicit precheck. TRA-702
// / BB32 D3.
func precheckUnknownFields(raw map[string]json.RawMessage, dst any, skip []string) *JSONUnknownFieldsError {
	if len(raw) == 0 {
		return nil
	}
	known := knownJSONTags(dst)
	if len(known) == 0 {
		return nil
	}
	skipSet := map[string]struct{}{}
	for _, s := range skip {
		skipSet[s] = struct{}{}
	}
	var unknowns []string
	for k := range raw {
		if _, ok := known[k]; ok {
			continue
		}
		if _, ok := skipSet[k]; ok {
			continue
		}
		unknowns = append(unknowns, k)
	}
	if len(unknowns) == 0 {
		return nil
	}
	sort.Strings(unknowns)
	return &JSONUnknownFieldsError{Fields: unknowns}
}

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
//
// TRA-702 / BB32 D3: a body with multiple unknown top-level keys returns a
// *JSONUnknownFieldsError carrying every offending key (sorted), so the
// caller can render one fields[] entry per invalid field. The strict
// decoder only reports the first unknown key on its own.
func DecodeJSONStrict(r *http.Request, dst any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return &JSONDecodeError{Cause: err}
	}
	if err := rejectNULByteBody(body); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) == nil {
		if ufe := precheckUnknownFields(raw, dst, nil); ufe != nil {
			return ufe
		}
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
	objectBody := json.Unmarshal(body, &raw) == nil
	if objectBody {
		for k := range raw {
			present[k] = struct{}{}
		}
		if ufe := precheckUnknownFields(raw, dst, nil); ufe != nil {
			return present, ufe
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

// PeekJSONFields reads the request body once, restores r.Body so downstream
// decoders can re-consume it, and returns the raw json values of any
// top-level keys named in `fields` that appeared in the body. Keys absent
// from the body are absent from the result.
//
// A non-object body or read failure returns a nil map and no error — the
// downstream decoder will surface the structural failure. Use this when a
// handler needs to compare submitted values against current resource state
// before the decoder strips read-only fields (TRA-710 / BB33 F2).
func PeekJSONFields(r *http.Request, fields []string) map[string]json.RawMessage {
	if len(fields) == 0 {
		return nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return nil
	}
	out := map[string]json.RawMessage{}
	for _, f := range fields {
		if v, ok := raw[f]; ok {
			out[f] = v
		}
	}
	return out
}

// SameJSON reports whether a peeked raw JSON value matches the JSON
// serialization of an expected value. Both sides are normalized through
// unmarshal/marshal so whitespace and map-key order do not produce
// spurious mismatches. Array element order is preserved — the caller must
// canonicalize order on fields where element order is not semantically
// significant.
//
// Used by the PATCH read-only echo check (TRA-710 / BB33 F2) to compare a
// submitted read-only field against the current resource value: matched →
// silent strip; differed → 400 read_only.
func SameJSON(submitted json.RawMessage, expected any) bool {
	if submitted == nil {
		return false
	}
	canonicalize := func(b []byte) ([]byte, error) {
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		return json.Marshal(v)
	}
	expectedBytes, err := json.Marshal(expected)
	if err != nil {
		return false
	}
	a, err := canonicalize(submitted)
	if err != nil {
		return false
	}
	b, err := canonicalize(expectedBytes)
	if err != nil {
		return false
	}
	return bytes.Equal(a, b)
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
// TRA-686 / BB29 F7+F8 generalized it. TRA-699 (natural-keys) and TRA-710
// (server-managed read-onlys + tags) subsequently moved fields off this
// map onto the post-decode echo check, so the policy map is intended for
// fields whose mere presence is invalid regardless of value. The exported
// asset/location PublicRejectPatchFields maps are currently empty.
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

	// TRA-702 / BB32 D2+D3: route through the central validation_error
	// helper so detail echoes violations[0].Message and gains the
	// "(and N more validation errors)" suffix when more than one field
	// rejected. Detail computation is identical to every other
	// validation_error emit-site.
	WriteValidationError(w, r, requestID, violations)
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
	//
	// TRA-707 / BB32 C3: surface a typed *JSONNullBodyError so
	// RespondDecodeError can render the RFC 7396 wording — `null` is
	// structurally valid JSON (it is a defined merge-patch directive), so
	// the generic "Request body is not valid JSON" fallback misdiagnoses
	// the failure.
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, nil, &JSONNullBodyError{}
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
		// TRA-702 / BB32 D3: surface every unknown top-level key, not just
		// the first one the strict decoder would catch. The drop set is
		// already applied above so dropped keys never look unknown.
		if ufe := precheckUnknownFields(raw, dst, nil); ufe != nil {
			return explicitNulls, present, ufe
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
//
// TRA-702 / BB32 D3: a *JSONUnknownFieldsError carrying multiple keys
// produces one fields[] entry per offending key with detail computed by
// WriteValidationError (echoes fields[0].Message + "(and N more ...)" suffix).
func RespondDecodeError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	if err != nil {
		// TRA-707 / BB32 C3: literal `null` body — surface RFC 7396 wording
		// rather than the generic "not valid JSON" fallback. `null` is
		// structurally valid JSON, so the parse-error wording misdiagnoses
		// the failure.
		var nbe *JSONNullBodyError
		if errors.As(err, &nbe) {
			WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
				"Request body must be a JSON object (RFC 7396)", requestID)
			return
		}

		var ufe *JSONUnknownFieldsError
		if errors.As(err, &ufe) && len(ufe.Fields) > 0 {
			fields := make([]apierrors.FieldError, 0, len(ufe.Fields))
			for _, name := range ufe.Fields {
				fields = append(fields, apierrors.FieldError{
					Field:   name,
					Code:    "unknown_field",
					Message: fmt.Sprintf("unknown field %q in request body", name),
				})
			}
			WriteValidationError(w, r, requestID, fields)
			return
		}

		// Defensive fallback: if a *JSONDecodeError arrives carrying the raw
		// encoding/json "unknown field" string (e.g. a code path that
		// bypasses the strict-decode precheck), still surface a
		// validation_error keyed on that single field. Pre-TRA-702 this was
		// the only emit path.
		re := regexp.MustCompile(`unknown field "([^"]+)"`)
		if matches := re.FindStringSubmatch(err.Error()); len(matches) > 1 {
			fieldName := matches[1]
			msg := fmt.Sprintf("unknown field %q in request body", fieldName)
			WriteValidationError(w, r, requestID, []apierrors.FieldError{{
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
			//
			// A free-form object field (Go `map[...]any`, e.g. `metadata`) is
			// declared `type: object` in the public spec, so a non-object
			// value is a schema violation rather than a parse error. Surface
			// it as validation_error / invalid_value with the JSON-leaf field
			// name — same shape as the date-format branch below. Without this
			// the TRA-678 tightening of `Metadata` from `*any` to
			// `*map[string]any` would route schema-violating bodies through
			// the generic bad_request fallback and lose `fields[]`.
			if isMapTarget(typeErr.Type) {
				field := typeErr.Field
				if i := strings.LastIndex(field, "."); i >= 0 {
					field = field[i+1:]
				}
				if field == "" {
					field = "(body)"
				}
				WriteValidationError(w, r, requestID, []apierrors.FieldError{{
					Field:   field,
					Code:    "invalid_value",
					Message: fmt.Sprintf("%s must be a JSON object", field),
				}})
				return
			}
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
				// TRA-704 / BB32 C4: the two default-value sentinels (Go
				// zero time, Unix epoch) reach this branch via the same
				// *json.UnmarshalTypeError path as any other format failure,
				// but the per-field guidance differs — the integrator did
				// produce a valid RFC 3339 string, they just supplied a
				// language-default marker where they meant "unset". Point
				// them at JSON null explicitly so the next request is
				// correct instead of swapping one sentinel for the other.
				if raw := strings.Trim(typeErr.Value, "\""); shared.IsSentinelTimestamp(raw) {
					msg = fmt.Sprintf("%s must not be a default-value sentinel (%s); use JSON null to leave the field unset", field, raw)
				}
				WriteValidationError(w, r, requestID, []apierrors.FieldError{{
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
//
// TRA-707 / BB32 D6: encoding/json prefixes the field path with the Go
// struct name when an embedded struct is in play (e.g. POST asset bodies
// land here as "CreateAssetRequest.name" because CreateAssetWithTagsRequest
// embeds CreateAssetRequest). The wire-facing field is the JSON-tag leaf —
// strip the struct qualifier so the response matches the request body shape
// integrators see, mirroring the same handling on the time-target branch
// in RespondDecodeError.
func typeMismatchDetail(e *json.UnmarshalTypeError) string {
	field := e.Field
	if i := strings.LastIndex(field, "."); i >= 0 {
		field = field[i+1:]
	}
	if field != "" {
		return fmt.Sprintf("Body field %q could not be decoded as the expected type", field)
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

// isMapTarget reports whether t resolves to a Go map (the destination type
// for spec-declared `type: object` free-form fields like `metadata`).
// Pointer wrappers (`*map[...]any`) are unwrapped first.
func isMapTarget(t reflect.Type) bool {
	if t == nil {
		return false
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Map
}
