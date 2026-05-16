package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

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
	if resp.SpecRefreshedAt != info.BuildTime {
		t.Errorf("spec_refreshed_at = %q, want %q (mirror of build_time per TRA-743)", resp.SpecRefreshedAt, info.BuildTime)
	}
}

// TestHealth_RouteRegistration verifies /health and /health.json both resolve
// to the same handler. The .json route is the canonical curl-able surface for
// BB tooling (and bypasses the SPA catch-all that would otherwise serve
// index.html). The bare /health path is preserved for K8s probes and existing
// operator muscle memory.
func TestHealth_RouteRegistration(t *testing.T) {
	info := buildinfo.Info{
		BuildTime: "2026-04-24T15:30:00Z",
	}
	h := NewHandler(nil, info, time.Now())

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	for _, path := range []string{"/health", "/health.json"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("GET %s: content-type = %q, want application/json", path, ct)
		}
		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Errorf("GET %s: decode: %v", path, err)
			continue
		}
		if resp.SpecRefreshedAt != info.BuildTime {
			t.Errorf("GET %s: spec_refreshed_at = %q, want %q", path, resp.SpecRefreshedAt, info.BuildTime)
		}
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
