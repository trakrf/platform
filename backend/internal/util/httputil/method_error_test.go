package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestRespond405_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/api/v1/assets", nil)
	httputil.Respond405(w, r, "req-3")

	if w.Code != 405 {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Title != "Method not allowed" {
		t.Errorf("title = %q, want Method not allowed", resp.Error.Title)
	}
	if resp.Error.Type != string(apierrors.ErrMethodNotAllowed) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrMethodNotAllowed)
	}
	if resp.Error.Status != 405 {
		t.Errorf("status field = %d, want 405", resp.Error.Status)
	}
	if resp.Error.Detail != "" {
		t.Errorf("detail = %q, want empty", resp.Error.Detail)
	}
}
