package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"

	"github.com/trakrf/platform/backend/internal/middleware"
)

// TRA-707 / BB32 D5: the middleware mirrors ParseListParams' rejection of
// unknown query keys for endpoints that do not have their own query-string
// parser. Bare /api/v1/orgs/me, single-resource GETs, write endpoints, and
// subresource POST/DELETEs each declare zero allowed keys; anything in the
// query string is a typo or a smuggled value and surfaces as 400
// validation_error so integrators can branch on it like any other field-
// level body failure.
func TestRejectQueryParams_RejectsUnknownKey(t *testing.T) {
	called := false
	handler := middleware.RejectQueryParams()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/assets/1?bogus=42", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatalf("downstream handler must not run on rejection")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Type != string(apierrors.ErrValidation) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrValidation)
	}
	if len(resp.Error.Fields) != 1 || resp.Error.Fields[0].Field != "bogus" {
		t.Fatalf("Fields = %+v, want one entry for 'bogus'", resp.Error.Fields)
	}
}

func TestRejectQueryParams_EmptyQueryPassesThrough(t *testing.T) {
	called := false
	handler := middleware.RejectQueryParams()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest("GET", "/api/v1/assets/1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatalf("downstream handler should run when no query is present")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
}
