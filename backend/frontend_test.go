package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCacheControlMiddleware(t *testing.T) {
	// Create a test handler that just returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with cache control middleware
	handler := cacheControlMiddleware(testHandler)

	tests := []struct {
		name           string
		path           string
		expectedCache  string
		expectedPragma string
	}{
		{
			name:           "index.html has no-cache",
			path:           "/",
			expectedCache:  "no-cache, no-store, must-revalidate",
			expectedPragma: "no-cache",
		},
		{
			name:           "explicit index.html has no-cache",
			path:           "/index.html",
			expectedCache:  "no-cache, no-store, must-revalidate",
			expectedPragma: "no-cache",
		},
		{
			name:           "SPA route has no-cache",
			path:           "/dashboard",
			expectedCache:  "no-cache, no-store, must-revalidate",
			expectedPragma: "no-cache",
		},
		{
			name:          "hashed JS asset has long cache",
			path:          "/assets/index-abc123.js",
			expectedCache: "public, max-age=31536000, immutable",
		},
		{
			name:          "hashed CSS asset has long cache",
			path:          "/assets/index-xyz789.css",
			expectedCache: "public, max-age=31536000, immutable",
		},
		{
			name:          "favicon has moderate cache",
			path:          "/favicon.ico",
			expectedCache: "public, max-age=3600",
		},
		{
			name:          "icon has moderate cache",
			path:          "/icon-192.png",
			expectedCache: "public, max-age=3600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			cacheControl := rec.Header().Get("Cache-Control")
			if cacheControl != tt.expectedCache {
				t.Errorf("Expected Cache-Control: %s, got: %s", tt.expectedCache, cacheControl)
			}

			// Only check Pragma for no-cache scenarios
			if tt.expectedPragma != "" {
				pragma := rec.Header().Get("Pragma")
				if pragma != tt.expectedPragma {
					t.Errorf("Expected Pragma: %s, got: %s", tt.expectedPragma, pragma)
				}
			}
		})
	}
}

func TestSPAHandler(t *testing.T) {
	// Note: This test requires frontend/dist/index.html to exist
	// The build.sh script ensures this is copied before building

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	spaHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got: %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type: text/html; charset=utf-8, got: %s", contentType)
	}

	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "no-cache, no-store, must-revalidate" {
		t.Errorf("Expected no-cache headers, got: %s", cacheControl)
	}

	// Verify we got HTML content
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("Expected HTML content, got empty body")
	}

	// Basic sanity check - should contain HTML tags
	if body[:15] != "<!DOCTYPE html>" {
		t.Errorf("Expected HTML doctype, got: %s", body[:min(50, len(body))])
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
