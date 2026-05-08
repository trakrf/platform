package httputil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
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
	if resp.Error.Fields[0].Code != "invalid_value" {
		t.Fatalf("fields[0].code = %q, want %q", resp.Error.Fields[0].Code, "invalid_value")
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
	var decErr *httputil.JSONDecodeError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected *httputil.JSONDecodeError, got %T", err)
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
