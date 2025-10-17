package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthzHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "GET returns 200 ok",
			method:     "GET",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "POST returns 405",
			method:     "POST",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   "",
		},
		{
			name:       "PUT returns 405",
			method:     "PUT",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/healthz", nil)
			w := httptest.NewRecorder()

			healthzHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestReadyzHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "GET returns 200 ok",
			method:     "GET",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "POST returns 405",
			method:     "POST",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/readyz", nil)
			w := httptest.NewRecorder()

			readyzHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHealthHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantStatus int
	}{
		{
			name:       "GET returns 200",
			method:     "GET",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST returns 405",
			method:     "POST",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "DELETE returns 405",
			method:     "DELETE",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/health", nil)
			w := httptest.NewRecorder()

			healthHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHealthResponse(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}

	if resp.Version != version {
		t.Errorf("version = %q, want %q", resp.Version, version)
	}

	if resp.Timestamp.IsZero() {
		t.Error("timestamp is zero")
	}

	// Verify timestamp is recent (within 1 second)
	if time.Since(resp.Timestamp) > time.Second {
		t.Errorf("timestamp too old: %v", resp.Timestamp)
	}
}
