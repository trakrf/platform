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
