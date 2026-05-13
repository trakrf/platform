package httputil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

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
