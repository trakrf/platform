package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestRespond405_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/api/v1/assets", nil)
	httputil.Respond405(w, r, []string{"GET", "POST"}, "req-3")

	if w.Code != 405 {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}
	if got := w.Header().Get("Allow"); got != "GET, POST" {
		t.Errorf("Allow = %q, want GET, POST", got)
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
	if resp.Error.Detail != "Allowed methods: GET, POST" {
		t.Errorf("detail = %q, want Allowed methods: GET, POST", resp.Error.Detail)
	}
	if resp.Error.RequestID != "req-3" {
		t.Errorf("request_id = %q, want req-3", resp.Error.RequestID)
	}
}

// TestRespond405_EmptyAllowed verifies the defensive path: when allowed is
// empty (a caller bug — at 405 time at least one method must match) the
// envelope still writes cleanly with no Allow header and empty detail.
func TestRespond405_EmptyAllowed(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/api/v1/assets", nil)
	httputil.Respond405(w, r, nil, "req-3b")

	if w.Code != 405 {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if got := w.Header().Get("Allow"); got != "" {
		t.Errorf("Allow = %q, want empty", got)
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Detail != "" {
		t.Errorf("detail = %q, want empty", resp.Error.Detail)
	}
}

func TestRespond415_DropsMultipartWording(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/assets", nil)
	httputil.Respond415(w, r, "req-4")
	assertRespond415Envelope(t, w, "Content-Type must be application/json", "req-4")
}

// On PATCH, the public spec declares application/merge-patch+json only
// (RFC 7396); the 415 detail must name that exact content type so
// integrators can repair the request without re-reading the spec.
func TestRespond415_PatchNamesMergePatch(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/api/v1/assets/1", nil)
	httputil.Respond415(w, r, "req-4p")
	assertRespond415Envelope(t, w, "Content-Type must be application/merge-patch+json on PATCH operations", "req-4p")
}

func assertRespond415Envelope(t *testing.T, w *httptest.ResponseRecorder, wantDetail, wantReqID string) {
	t.Helper()

	if w.Code != 415 {
		t.Fatalf("status = %d, want 415", w.Code)
	}

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Title != "Unsupported media type" {
		t.Errorf("title = %q, want Unsupported media type", resp.Error.Title)
	}
	if resp.Error.Type != string(apierrors.ErrUnsupportedMedia) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrUnsupportedMedia)
	}
	// POLS: AI integrators work from openapi.public.yaml, which never names
	// multipart/form-data. The public 415 message must not either.
	if strings.Contains(resp.Error.Detail, "multipart") {
		t.Errorf("detail = %q, must not mention multipart", resp.Error.Detail)
	}
	if resp.Error.Detail != wantDetail {
		t.Errorf("detail = %q, want %q", resp.Error.Detail, wantDetail)
	}
	if resp.Error.RequestID != wantReqID {
		t.Errorf("request_id = %q, want %q", resp.Error.RequestID, wantReqID)
	}
}

func TestRespondMissingOrgContext_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/assets", nil)
	httputil.RespondMissingOrgContext(w, r, "req-mo")

	if w.Code != 422 {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Type != string(apierrors.ErrMissingOrgContext) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrMissingOrgContext)
	}
	if resp.Error.Title != "Missing org context" {
		t.Errorf("title = %q, want Missing org context", resp.Error.Title)
	}
	if resp.Error.Status != 422 {
		t.Errorf("status field = %d, want 422", resp.Error.Status)
	}
	if resp.Error.Detail != "This request requires an active organization context. Select an organization or re-authenticate." {
		t.Errorf("detail = %q, want canonical message", resp.Error.Detail)
	}
	if resp.Error.RequestID != "req-mo" {
		t.Errorf("request_id = %q, want req-mo", resp.Error.RequestID)
	}
}
