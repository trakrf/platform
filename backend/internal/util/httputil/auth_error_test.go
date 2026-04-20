package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestRespond401_SetsWWWAuthenticateAndNormalizedTitle(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/protected", nil)
	httputil.Respond401(w, r, "Bearer token is invalid or expired", "req-1")

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if got := w.Header().Get("WWW-Authenticate"); got != `Bearer realm="trakrf-api"` {
		t.Errorf("WWW-Authenticate = %q, want Bearer realm=\"trakrf-api\"", got)
	}

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Title != "Authentication required" {
		t.Errorf("title = %q, want Authentication required", resp.Error.Title)
	}
	if resp.Error.Type != string(apierrors.ErrUnauthorized) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrUnauthorized)
	}
	if resp.Error.Detail != "Bearer token is invalid or expired" {
		t.Errorf("detail = %q, want caller-supplied string", resp.Error.Detail)
	}
}
