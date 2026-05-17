package httputil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestDecodeJSON_ValidBody(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"abc"}`))
	var got payload
	if err := httputil.DecodeJSON(r, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "abc" {
		t.Fatalf("got name=%q, want abc", got.Name)
	}
}

func TestDecodeJSON_MalformedBody_ReturnsTypedError(t *testing.T) {
	type payload struct{}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`not json`))
	var got payload
	err := httputil.DecodeJSON(r, &got)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var jde *httputil.JSONDecodeError
	if !errors.As(err, &jde) {
		t.Fatalf("expected *JSONDecodeError, got %T", err)
	}
}

func TestRespondDecodeError_StableDetail(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: errors.New("anything")}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Detail != "Request body is not valid JSON" {
		t.Fatalf("detail = %q, want stable string", resp.Error.Detail)
	}
	if resp.Error.Type != string(apierrors.ErrBadRequest) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrBadRequest)
	}
}

func TestRespondDecodeError_UnknownField_EmitsValidationError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	// Simulate json.Decoder error for unknown field
	httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: errors.New("json: unknown field \"parent_path\"")}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Type != string(apierrors.ErrValidation) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrValidation)
	}
	if !strings.Contains(resp.Error.Detail, "unknown field") || !strings.Contains(resp.Error.Detail, "parent_path") {
		t.Fatalf("detail = %q, should describe the unknown field by name", resp.Error.Detail)
	}
	if strings.HasPrefix(resp.Error.Detail, "Request body is not valid JSON") {
		t.Fatalf("detail = %q, should not claim the body is invalid JSON — it is valid, the field is just unknown", resp.Error.Detail)
	}
	if len(resp.Error.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(resp.Error.Fields))
	}
	if resp.Error.Fields[0].Field != "parent_path" {
		t.Fatalf("fields[0].field = %q, want %q", resp.Error.Fields[0].Field, "parent_path")
	}
	if resp.Error.Fields[0].Code != "unknown_field" {
		t.Fatalf("fields[0].code = %q, want %q", resp.Error.Fields[0].Code, "unknown_field")
	}
}

// TRA-634: a body that parses as JSON but mismatches a Go field type must
// produce a wording that describes the type mismatch with the offending
// field name — not "Request body is not valid JSON", which is misleading
// because the body IS valid JSON.
func TestRespondDecodeError_TypeMismatch_NamesFieldAndAvoidsInvalidJSONWording(t *testing.T) {
	type target struct {
		Count int `json:"count"`
	}
	var dst target
	decErr := json.Unmarshal([]byte(`{"count":"not a number"}`), &dst)
	if decErr == nil {
		t.Fatalf("expected json.Unmarshal to return an UnmarshalTypeError, got nil")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: decErr}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Type != string(apierrors.ErrBadRequest) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrBadRequest)
	}
	if resp.Error.Detail == "Request body is not valid JSON" {
		t.Fatalf("detail = %q, must not claim the body is invalid JSON — it is valid, just wrong type", resp.Error.Detail)
	}
	if !strings.Contains(resp.Error.Detail, "count") {
		t.Fatalf("detail = %q, should name the offending field 'count'", resp.Error.Detail)
	}
	if !strings.Contains(resp.Error.Detail, "expected type") {
		t.Fatalf("detail = %q, should describe the type-mismatch nature of the failure", resp.Error.Detail)
	}
}

// TRA-641 / BB21 §2.1: a malformed RFC3339 string in a date field surfaces
// as validation_error with fields[].field naming the offending key — not
// bad_request. The body is valid JSON; only the field value fails the
// format check, so the failure belongs in the validation pass alongside
// other field-level failures (required, too_short, ...).
func TestRespondDecodeError_BadRFC3339_EmitsValidationError(t *testing.T) {
	type target struct {
		ValidFrom shared.FlexibleDate `json:"valid_from"`
	}
	var dst target
	decErr := json.Unmarshal([]byte(`{"valid_from":"not-a-date"}`), &dst)
	if decErr == nil {
		t.Fatalf("expected json.Unmarshal to return an error, got nil")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: decErr}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Type != string(apierrors.ErrValidation) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrValidation)
	}
	if len(resp.Error.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(resp.Error.Fields))
	}
	if resp.Error.Fields[0].Field != "valid_from" {
		t.Fatalf("fields[0].field = %q, want %q", resp.Error.Fields[0].Field, "valid_from")
	}
	if resp.Error.Fields[0].Code != "invalid_value" {
		t.Fatalf("fields[0].code = %q, want %q", resp.Error.Fields[0].Code, "invalid_value")
	}
}

// TRA-649: when the date field lives on an embedded struct, encoding/json
// reports the field path qualified by the struct name (e.g.
// "CreateAssetRequest.valid_from"). The wire-facing response must show the
// JSON-tag leaf only — the integrator's request body uses bare keys, not
// Go-struct-qualified names.
func TestRespondDecodeError_BadRFC3339_EmbeddedStruct_StripsStructPrefix(t *testing.T) {
	type inner struct {
		ValidFrom shared.FlexibleDate `json:"valid_from"`
	}
	type target struct {
		inner
	}
	var dst target
	decErr := json.Unmarshal([]byte(`{"valid_from":"not-a-date"}`), &dst)
	if decErr == nil {
		t.Fatalf("expected json.Unmarshal to return an error, got nil")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: decErr}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if len(resp.Error.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(resp.Error.Fields))
	}
	if resp.Error.Fields[0].Field != "valid_from" {
		t.Fatalf("fields[0].field = %q, want %q", resp.Error.Fields[0].Field, "valid_from")
	}
}

// TRA-704 / BB32 C4: when a request supplies one of the two default-value
// sentinels (Go zero, Unix epoch) on a timestamp field, the rejection
// message must point the integrator at JSON null rather than read as a
// generic format failure. Both sentinels share the same UnmarshalTypeError
// path as any other bad RFC 3339 string, so the per-field message is the
// only place the distinction can surface.
func TestRespondDecodeError_SentinelTimestamps_PointAtNull(t *testing.T) {
	type target struct {
		ValidTo shared.FlexibleDate `json:"valid_to"`
	}
	cases := []struct {
		name     string
		body     string
		sentinel string
	}{
		{"Go zero-time", `{"valid_to":"0001-01-01T00:00:00Z"}`, "0001-01-01T00:00:00Z"},
		{"Unix epoch", `{"valid_to":"1970-01-01T00:00:00Z"}`, "1970-01-01T00:00:00Z"},
		{"Unix epoch with offset", `{"valid_to":"1970-01-01T00:00:00+00:00"}`, "1970-01-01T00:00:00+00:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dst target
			decErr := json.Unmarshal([]byte(tc.body), &dst)
			if decErr == nil {
				t.Fatalf("expected json.Unmarshal to return an error, got nil")
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/", strings.NewReader(""))
			httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: decErr}, "req-1")

			if w.Code != 400 {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			var resp apierrors.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode resp: %v", err)
			}
			if resp.Error.Type != string(apierrors.ErrValidation) {
				t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrValidation)
			}
			if len(resp.Error.Fields) != 1 {
				t.Fatalf("fields = %d, want 1", len(resp.Error.Fields))
			}
			if resp.Error.Fields[0].Field != "valid_to" {
				t.Fatalf("fields[0].field = %q, want %q", resp.Error.Fields[0].Field, "valid_to")
			}
			if resp.Error.Fields[0].Code != "invalid_value" {
				t.Fatalf("fields[0].code = %q, want %q", resp.Error.Fields[0].Code, "invalid_value")
			}
			msg := resp.Error.Fields[0].Message
			if !strings.Contains(msg, tc.sentinel) {
				t.Fatalf("fields[0].message %q must echo offending sentinel %q", msg, tc.sentinel)
			}
			if !strings.Contains(msg, "null") {
				t.Fatalf("fields[0].message %q must point integrator at JSON null", msg)
			}
		})
	}
}

// TRA-767 / BB57 F1: the sentinel-rejection recommendation must match the
// null-rejection recommendation. valid_from is non-nullable: the handler
// rejects explicit null with "omit the field to use the server default" on
// POST and "omit the field to leave unchanged" on PATCH. The sentinel
// rejection path must point at the same omit hint instead of pointing at
// JSON null, which the null path would then reject.
func TestRespondDecodeError_SentinelTimestamps_NonNullableField_PointAtOmit(t *testing.T) {
	type target struct {
		ValidFrom shared.FlexibleDate `json:"valid_from"`
	}
	cases := []struct {
		name       string
		method     string
		wantPhrase string
	}{
		{"POST uses server-default hint", "POST", "omit the field to use the server default"},
		{"PATCH uses leave-unchanged hint", "PATCH", "omit the field to leave unchanged"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dst target
			decErr := json.Unmarshal([]byte(`{"valid_from":"0001-01-01T00:00:00Z"}`), &dst)
			if decErr == nil {
				t.Fatalf("expected json.Unmarshal to return an error, got nil")
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, "/", strings.NewReader(""))
			httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: decErr}, "req-1")

			if w.Code != 400 {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			var resp apierrors.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode resp: %v", err)
			}
			if len(resp.Error.Fields) != 1 {
				t.Fatalf("fields = %d, want 1", len(resp.Error.Fields))
			}
			msg := resp.Error.Fields[0].Message
			if !strings.Contains(msg, tc.wantPhrase) {
				t.Fatalf("fields[0].message %q must contain %q (matches null-rejection path on non-nullable timestamps)", msg, tc.wantPhrase)
			}
			if strings.Contains(msg, "use JSON null") {
				t.Fatalf("fields[0].message %q must NOT instruct integrator to use JSON null on non-nullable field — the null path rejects that", msg)
			}
			if !strings.Contains(msg, "or provide a real timestamp") {
				t.Fatalf("fields[0].message %q should include the provide-a-real-timestamp alternative", msg)
			}
		})
	}
}

// TRA-767 / BB57 F2: a type-mismatch detail must name the expected JSON
// type when the decoder knows it. The validation-stage envelope surfaces
// the expected type through params; the decode-stage envelope previously
// withheld it, forcing integrators to probe to find the expected type.
func TestRespondDecodeError_TypeMismatch_IncludesExpectedJSONType(t *testing.T) {
	type target struct {
		IsActive bool `json:"is_active"`
	}
	var dst target
	decErr := json.Unmarshal([]byte(`{"is_active":"true"}`), &dst)
	if decErr == nil {
		t.Fatalf("expected json.Unmarshal to return an UnmarshalTypeError, got nil")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: decErr}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Type != string(apierrors.ErrBadRequest) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrBadRequest)
	}
	if !strings.Contains(resp.Error.Detail, "is_active") {
		t.Fatalf("detail = %q, should name the offending field", resp.Error.Detail)
	}
	if !strings.Contains(resp.Error.Detail, "boolean") {
		t.Fatalf("detail = %q, should include the expected type 'boolean'", resp.Error.Detail)
	}
}

// Top-level type mismatch (no field name available) must still avoid the
// misleading "not valid JSON" wording.
func TestRespondDecodeError_TypeMismatch_TopLevel_GenericWording(t *testing.T) {
	type target struct {
		Name string `json:"name"`
	}
	var dst target
	decErr := json.Unmarshal([]byte(`[1,2,3]`), &dst)
	if decErr == nil {
		t.Fatalf("expected json.Unmarshal to return an UnmarshalTypeError, got nil")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: decErr}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Detail == "Request body is not valid JSON" {
		t.Fatalf("detail = %q, must not claim the body is invalid JSON", resp.Error.Detail)
	}
	if !strings.Contains(resp.Error.Detail, "expected type") {
		t.Fatalf("detail = %q, should describe a type-decoding failure", resp.Error.Detail)
	}
}

func TestDecodeJSONStrict_RejectsUnknownField(t *testing.T) {
	type target struct {
		Name string `json:"name"`
	}
	var got target
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"name":"x","extra":1}`))
	err := httputil.DecodeJSONStrict(r, &got)

	if err == nil {
		t.Fatalf("expected strict decode to reject unknown field, got nil")
	}
	// TRA-702: strict-decode helpers pre-detect every unknown top-level key
	// via reflection and return *JSONUnknownFieldsError so RespondDecodeError
	// can render one fields[] entry per unknown.
	var ufe *httputil.JSONUnknownFieldsError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *httputil.JSONUnknownFieldsError, got %T", err)
	}
	if len(ufe.Fields) != 1 || ufe.Fields[0] != "extra" {
		t.Fatalf("Fields = %v, want [extra]", ufe.Fields)
	}
}

func TestDecodeJSONStrict_AcceptsKnownFieldsOnly(t *testing.T) {
	type target struct {
		Name string `json:"name"`
	}
	var got target
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"name":"x"}`))
	if err := httputil.DecodeJSONStrict(r, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "x" {
		t.Fatalf("Name = %q, want %q", got.Name, "x")
	}
}

// TRA-608 / BB18 §1.7: GET → PUT round-trip must succeed, with read-only
// fields silently stripped from the request body before strict decoding.
func TestDecodeJSONStrictWithNullsTolerant_DropsReadOnlyFields(t *testing.T) {
	type target struct {
		Name *string `json:"name"`
	}
	var got target
	body := `{"id":42,"name":"x","created_at":"2026-01-01T00:00:00Z","tags":[{"value":"abc"}]}`
	r := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))

	nulls, err := httputil.DecodeJSONStrictWithNullsTolerant(r, &got, []string{"id", "created_at", "updated_at", "tags"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name == nil || *got.Name != "x" {
		t.Fatalf("Name = %v, want \"x\"", got.Name)
	}
	if _, ok := nulls["id"]; ok {
		t.Fatalf("id should not appear in explicit-nulls (it was stripped)")
	}
}

// Strict-unknown-field still applies for fields that are not in the drop set.
func TestDecodeJSONStrictWithNullsTolerant_RejectsTypoFields(t *testing.T) {
	type target struct {
		Name *string `json:"name"`
	}
	var got target
	body := `{"id":42,"nme":"x"}` // "nme" is a typo, not in drop set
	r := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))

	_, err := httputil.DecodeJSONStrictWithNullsTolerant(r, &got, []string{"id"})
	if err == nil {
		t.Fatalf("expected strict decode to reject typo'd field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err = %v, want unknown-field error", err)
	}
}

// Explicit null on a kept field is still reported via the nulls map; explicit
// null on a stripped field is suppressed (we already removed the key).
func TestDecodeJSONStrictWithNullsTolerant_NullSemanticsOnKeptVsStripped(t *testing.T) {
	type target struct {
		ValidTo *string `json:"valid_to"`
		Name    *string `json:"name"`
	}
	var got target
	body := `{"valid_to":null,"name":null,"updated_at":null}`
	r := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))

	nulls, err := httputil.DecodeJSONStrictWithNullsTolerant(r, &got, []string{"updated_at"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := nulls["valid_to"]; !ok {
		t.Fatalf("valid_to null should be reported (kept field)")
	}
	if _, ok := nulls["name"]; !ok {
		t.Fatalf("name null should be reported (kept field)")
	}
	if _, ok := nulls["updated_at"]; ok {
		t.Fatalf("updated_at null should be suppressed (stripped field)")
	}
}

// TRA-702 / BB32 D3: a request body with multiple unknown top-level fields
// must surface every offending key in fields[]. encoding/json's
// DisallowUnknownFields stops at the first one, so the decoder helpers do
// the enumeration up-front via reflection on the destination struct.
func TestDecodeJSONStrict_MultipleUnknownFields_AllReported(t *testing.T) {
	type target struct {
		Name string `json:"name"`
	}
	var got target
	body := `{"name":"x","foo":1,"bar":2}`
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	err := httputil.DecodeJSONStrict(r, &got)

	if err == nil {
		t.Fatalf("expected strict decode to reject unknown fields, got nil")
	}
	var ufe *httputil.JSONUnknownFieldsError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *JSONUnknownFieldsError, got %T (%v)", err, err)
	}
	// Both unknown fields must be reported, in deterministic order.
	got2 := append([]string(nil), ufe.Fields...)
	if len(got2) != 2 || got2[0] != "bar" || got2[1] != "foo" {
		t.Fatalf("Fields = %v, want [bar foo] in sorted order", got2)
	}
}

// RespondDecodeError must translate a *JSONUnknownFieldsError into a
// multi-entry validation_error envelope, one fields[] entry per unknown key,
// with detail echoing the first field's message + the multi-field suffix.
func TestRespondDecodeError_MultipleUnknownFields_MultiEntryAndEchoesDetail(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	httputil.RespondDecodeError(w, r, &httputil.JSONUnknownFieldsError{Fields: []string{"bar", "foo"}}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}

	if resp.Error.Type != string(apierrors.ErrValidation) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrValidation)
	}
	if len(resp.Error.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(resp.Error.Fields))
	}
	if resp.Error.Fields[0].Field != "bar" || resp.Error.Fields[0].Code != "unknown_field" {
		t.Fatalf("fields[0] = %+v, want field=bar code=unknown_field", resp.Error.Fields[0])
	}
	if resp.Error.Fields[1].Field != "foo" || resp.Error.Fields[1].Code != "unknown_field" {
		t.Fatalf("fields[1] = %+v, want field=foo code=unknown_field", resp.Error.Fields[1])
	}
	// detail must echo fields[0].Message plus '(and 1 more validation error)' suffix.
	wantDetail := resp.Error.Fields[0].Message + " (and 1 more validation error)"
	if resp.Error.Detail != wantDetail {
		t.Fatalf("detail = %q, want %q", resp.Error.Detail, wantDetail)
	}
}

// TRA-707 / BB32 D6: a type-mismatch on a field declared via an embedded
// struct surfaces from encoding/json as "OuterType.field". The wire-facing
// detail string must show the JSON-tag leaf only — integrators see bare
// keys in their request body, not Go-struct-qualified names. Mirrors the
// embedded-struct stripping already applied on the time-target branch in
// TestRespondDecodeError_BadRFC3339_EmbeddedStruct_StripsStructPrefix.
func TestRespondDecodeError_TypeMismatch_EmbeddedStruct_StripsStructPrefix(t *testing.T) {
	type inner struct {
		Count int `json:"count"`
	}
	type target struct {
		inner
	}
	var dst target
	decErr := json.Unmarshal([]byte(`{"count":"not a number"}`), &dst)
	if decErr == nil {
		t.Fatalf("expected json.Unmarshal to return an UnmarshalTypeError, got nil")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: decErr}, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if !strings.Contains(resp.Error.Detail, `"count"`) {
		t.Fatalf("detail = %q, should name the offending field 'count'", resp.Error.Detail)
	}
	if strings.Contains(resp.Error.Detail, ".") {
		t.Fatalf("detail = %q, should not contain a struct-qualified field name", resp.Error.Detail)
	}
}

// TRA-707 / BB32 C3: a literal `null` request body is structurally valid
// JSON (RFC 7396 defines it as a merge-patch directive that empties the
// target object), so the rejection wording must name RFC 7396 rather than
// fall through to "Request body is not valid JSON" — that wording
// misdiagnoses the failure and sends integrators chasing a JSON syntax
// error that does not exist.
func TestRespondDecodeError_NullBody_NamesRFC7396(t *testing.T) {
	type target struct {
		Name *string `json:"name"`
	}
	var got target
	r := httptest.NewRequest("PATCH", "/", bytes.NewBufferString(`null`))
	_, err := httputil.DecodeJSONStrictWithNullsTolerant(r, &got, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var nbe *httputil.JSONNullBodyError
	if !errors.As(err, &nbe) {
		t.Fatalf("expected *httputil.JSONNullBodyError, got %T (%v)", err, err)
	}

	w := httptest.NewRecorder()
	httputil.RespondDecodeError(w, r, err, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if jerr := json.Unmarshal(w.Body.Bytes(), &resp); jerr != nil {
		t.Fatalf("decode resp: %v", jerr)
	}
	if resp.Error.Type != string(apierrors.ErrBadRequest) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrBadRequest)
	}
	if !strings.Contains(resp.Error.Detail, "RFC 7396") {
		t.Fatalf("detail = %q, must name RFC 7396 so the integrator knows the rejection is about merge-patch shape, not JSON syntax", resp.Error.Detail)
	}
	if strings.Contains(resp.Error.Detail, "not valid JSON") {
		t.Fatalf("detail = %q, must not claim the body is invalid JSON — `null` is structurally valid", resp.Error.Detail)
	}
}

// TRA-710 (BB33 F2): SameJSON compares a peeked raw body value against an
// expected current resource value. Used by the PATCH read-only echo check
// to silently strip matching values and reject differing ones.
func TestSameJSON(t *testing.T) {
	cases := []struct {
		name      string
		submitted string
		expected  any
		want      bool
	}{
		{"int matches", `42`, 42, true},
		{"int differs", `42`, 43, false},
		{"string matches", `"2026-05-14T16:51:02Z"`, "2026-05-14T16:51:02Z", true},
		{"null matches nil pointer", `null`, (*string)(nil), true},
		{"non-null vs nil pointer", `"x"`, (*string)(nil), false},
		{"empty array matches empty slice", `[]`, []int{}, true},
		{"empty array vs nil slice", `[]`, []int(nil), false},
		{"different whitespace canonical match", `[ 1 , 2 ]`, []int{1, 2}, true},
		{"object key order canonical match", `{"b":1,"a":2}`, map[string]int{"a": 2, "b": 1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := httputil.SameJSON(json.RawMessage(tc.submitted), tc.expected)
			if got != tc.want {
				t.Fatalf("SameJSON(%s, %v) = %v, want %v", tc.submitted, tc.expected, got, tc.want)
			}
		})
	}
}

// TRA-721: SameJSONInstant compares a peeked datetime JSON value against
// an expected time.Time / shared.PublicTime as instants, so byte-different
// RFC 3339 representations of the same moment compare equal. The four
// BB35 cycle 2 cases (literal Z, +00:00 offset, microsecond fractional,
// differing instant) ground the test.
func TestSameJSONInstant(t *testing.T) {
	instant := shared.NewPublicTime(time.Date(2026, 5, 14, 19, 42, 5, 121000000, time.UTC))
	cases := []struct {
		name      string
		submitted string
		expected  any
		want      bool
	}{
		// Wire shape the server emits today (PublicTime millisecond Z).
		{"emit shape literal Z matches", `"2026-05-14T19:42:05.121Z"`, instant, true},
		// Same instant, +00:00 offset form (Go time.Time MarshalJSON default).
		{"plus-offset matches", `"2026-05-14T19:42:05.121+00:00"`, instant, true},
		// Same instant, microsecond fractional (Pydantic-default).
		{"microsecond fractional matches", `"2026-05-14T19:42:05.121000+00:00"`, instant, true},
		// Same instant via a non-UTC offset.
		{"non-utc offset same instant matches", `"2026-05-14T14:42:05.121-05:00"`, instant, true},
		// Different instant — must still reject.
		{"different instant differs", `"2026-05-14T19:42:05.122Z"`, instant, false},
		// Malformed datetime → mismatch (handler escalates to read_only).
		{"unparseable submitted differs", `"not-a-date"`, instant, false},
		// Nullable case: nil *PublicTime vs JSON null → match.
		{"null matches nil PublicTime", `null`, (*shared.PublicTime)(nil), true},
		// Nullable case: nil *PublicTime vs JSON value → mismatch.
		{"value vs nil PublicTime differs", `"2026-05-14T19:42:05.121Z"`, (*shared.PublicTime)(nil), false},
		// Nullable case: non-nil *PublicTime vs JSON null → mismatch.
		{"null vs non-nil PublicTime differs", `null`, &instant, false},
		// Nullable case: non-nil *PublicTime vs same instant in alt form → match.
		{"alt form matches non-nil PublicTime", `"2026-05-14T19:42:05.121000+00:00"`, &instant, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := httputil.SameJSONInstant(json.RawMessage(tc.submitted), tc.expected)
			if got != tc.want {
				t.Fatalf("SameJSONInstant(%s, %v) = %v, want %v", tc.submitted, tc.expected, got, tc.want)
			}
		})
	}
}

// TRA-710 (BB33 F2): PeekJSONFields returns raw values for the requested
// top-level keys without consuming the request body — downstream decoders
// see the same byte stream.
func TestPeekJSONFields_ReturnsRawValuesAndRestoresBody(t *testing.T) {
	r := httptest.NewRequest("PATCH", "/",
		bytes.NewBufferString(`{"id":42,"name":"abc","tags":[{"t":"x"}]}`))
	got := httputil.PeekJSONFields(r, []string{"id", "tags", "missing"})
	if len(got) != 2 {
		t.Fatalf("expected 2 peeked keys, got %d (%v)", len(got), got)
	}
	if string(got["id"]) != "42" {
		t.Fatalf("id = %q, want 42", string(got["id"]))
	}
	if string(got["tags"]) != `[{"t":"x"}]` {
		t.Fatalf("tags = %q", string(got["tags"]))
	}

	// Body must still be readable for a downstream decoder.
	var rest struct {
		Name string `json:"name"`
	}
	if err := httputil.DecodeJSON(r, &rest); err != nil {
		t.Fatalf("downstream decode after peek failed: %v", err)
	}
	if rest.Name != "abc" {
		t.Fatalf("name = %q, want abc", rest.Name)
	}
}

// Empty drop list reduces to plain DecodeJSONStrictWithNulls behavior.
func TestDecodeJSONStrictWithNullsTolerant_EmptyDropListEquivalent(t *testing.T) {
	type target struct {
		Name *string `json:"name"`
	}
	var got target
	r := httptest.NewRequest("PUT", "/", bytes.NewBufferString(`{"id":1}`))

	_, err := httputil.DecodeJSONStrictWithNullsTolerant(r, &got, nil)
	if err == nil {
		t.Fatalf("expected unknown-field error with empty drop list, got nil")
	}
}
