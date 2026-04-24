package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/buildinfo"
)

// TestHealth_ReturnsBuildInfo verifies /health exposes the ldflags-injected
// build metadata so operators can tell which commit is deployed without
// exec'ing into the pod. See TRA-481.
func TestHealth_ReturnsBuildInfo(t *testing.T) {
	info := buildinfo.Info{
		Version:   "0.1.0-test",
		Commit:    "abc1234",
		Tag:       "sha-abc1234",
		BuildTime: "2026-04-24T15:30:00Z",
		GoVersion: "go1.25.0",
	}
	start := time.Now().Add(-30 * time.Second)

	// nil db pool → handler skips the ping; dbStatus defaults to "unknown".
	h := NewHandler(nil, info, start)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if resp.Version != info.Version {
		t.Errorf("version = %q, want %q", resp.Version, info.Version)
	}
	if resp.Commit != info.Commit {
		t.Errorf("commit = %q, want %q", resp.Commit, info.Commit)
	}
	if resp.Tag != info.Tag {
		t.Errorf("tag = %q, want %q", resp.Tag, info.Tag)
	}
	if resp.BuildTime != info.BuildTime {
		t.Errorf("build_time = %q, want %q", resp.BuildTime, info.BuildTime)
	}
	if resp.GoVersion != info.GoVersion {
		t.Errorf("go_version = %q, want %q", resp.GoVersion, info.GoVersion)
	}
	if resp.Uptime == "" {
		t.Errorf("uptime missing")
	}
	if resp.Timestamp.IsZero() {
		t.Errorf("timestamp missing")
	}
}

// TestHealth_RejectsNonGET guards against accidental POST/DELETE regressions
// to a method still in wide use by K8s and operator curl.
func TestHealth_RejectsNonGET(t *testing.T) {
	h := NewHandler(nil, buildinfo.Info{}, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()
	h.Health(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// TestHealthz_ReturnsPlainOK confirms the K8s liveness probe stays tiny,
// plaintext, and unchanged by the TRA-481 build-info additions. K8s probes
// don't parse bodies — altering /healthz to JSON would be a silent breakage.
func TestHealthz_ReturnsPlainOK(t *testing.T) {
	h := NewHandler(nil, buildinfo.Info{}, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.Healthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("content-type = %q, want text/plain", ct)
	}
}
