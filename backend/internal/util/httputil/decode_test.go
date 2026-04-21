package httputil_test

import (
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
