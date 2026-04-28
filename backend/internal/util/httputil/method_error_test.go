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

func TestRespond415_DropsMultipartWording(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/assets", nil)
	httputil.Respond415(w, r, "req-4")

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
	if resp.Error.Detail != "Content-Type must be application/json" {
		t.Errorf("detail = %q, want Content-Type must be application/json", resp.Error.Detail)
	}
	if resp.Error.RequestID != "req-4" {
		t.Errorf("request_id = %q, want req-4", resp.Error.RequestID)
	}
}
