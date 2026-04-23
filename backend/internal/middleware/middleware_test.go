package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

func TestContentType(t *testing.T) {
	// Handler that just returns 200 OK if middleware passes
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name           string
		method         string
		contentType    string
		expectedStatus int
		description    string
	}{
		// GET requests - Content-Type not checked
		{
			name:           "GET request with no Content-Type",
			method:         http.MethodGet,
			contentType:    "",
			expectedStatus: http.StatusOK,
			description:    "GET requests should not check Content-Type",
		},
		{
			name:           "GET request with application/json",
			method:         http.MethodGet,
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			description:    "GET requests should ignore Content-Type",
		},
		// POST requests with valid Content-Types
		{
			name:           "POST with application/json",
			method:         http.MethodPost,
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			description:    "Standard JSON API request",
		},
		{
			name:           "POST with application/json; charset=utf-8",
			method:         http.MethodPost,
			contentType:    "application/json; charset=utf-8",
			expectedStatus: http.StatusOK,
			description:    "JSON with charset parameter",
		},
		{
			name:           "POST with multipart/form-data",
			method:         http.MethodPost,
			contentType:    "multipart/form-data; boundary=----WebKitFormBoundary",
			expectedStatus: http.StatusOK,
			description:    "File upload request",
		},
		{
			name:           "POST with empty Content-Type",
			method:         http.MethodPost,
			contentType:    "",
			expectedStatus: http.StatusOK,
			description:    "Empty Content-Type for backwards compatibility",
		},
		// PUT requests with valid Content-Types
		{
			name:           "PUT with application/json",
			method:         http.MethodPut,
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			description:    "PUT with JSON",
		},
		{
			name:           "PUT with multipart/form-data",
			method:         http.MethodPut,
			contentType:    "multipart/form-data; boundary=----WebKitFormBoundary",
			expectedStatus: http.StatusOK,
			description:    "PUT with file upload",
		},
		// PATCH requests with valid Content-Types
		{
			name:           "PATCH with application/json",
			method:         http.MethodPatch,
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			description:    "PATCH with JSON",
		},
		// Invalid Content-Types
		{
			name:           "POST with text/plain",
			method:         http.MethodPost,
			contentType:    "text/plain",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "Text plain not allowed",
		},
		{
			name:           "POST with application/xml",
			method:         http.MethodPost,
			contentType:    "application/xml",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "XML not allowed",
		},
		{
			name:           "POST with text/csv",
			method:         http.MethodPost,
			contentType:    "text/csv",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "CSV not allowed as Content-Type (must use multipart/form-data)",
		},
		{
			name:           "PUT with application/x-www-form-urlencoded",
			method:         http.MethodPut,
			contentType:    "application/x-www-form-urlencoded",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "Form-encoded not allowed",
		},
		// DELETE requests - Content-Type not checked
		{
			name:           "DELETE request with any Content-Type",
			method:         http.MethodDelete,
			contentType:    "text/plain",
			expectedStatus: http.StatusOK,
			description:    "DELETE requests should not check Content-Type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest(tt.method, "/test", strings.NewReader("{}"))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Wrap handler with middleware
			handler := ContentType(nextHandler)
			handler.ServeHTTP(rr, req)

			// Check status code
			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("ContentType middleware test '%s' failed:\n"+
					"  Expected status: %d\n"+
					"  Got status:      %d\n"+
					"  Description:     %s\n"+
					"  Method:          %s\n"+
					"  Content-Type:    %s",
					tt.name,
					tt.expectedStatus,
					status,
					tt.description,
					tt.method,
					tt.contentType)
			}
		})
	}
}

func TestGenerateRequestID_ULIDFormat(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(w, r)

	got := w.Header().Get("X-Request-ID")
	ulidRE := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
	if !ulidRE.MatchString(got) {
		t.Fatalf("X-Request-ID = %q, want ULID (26-char Crockford base32)", got)
	}
}

func TestContentType_MultipartBoundary(t *testing.T) {
	// Test that multipart/form-data works with various boundary formats
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	boundaries := []string{
		"multipart/form-data",
		"multipart/form-data; boundary=----WebKitFormBoundary",
		"multipart/form-data; boundary=something123",
		"multipart/form-data;boundary=no-space",
	}

	handler := ContentType(nextHandler)

	for _, boundary := range boundaries {
		t.Run(boundary, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("Content-Type", boundary)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != http.StatusOK {
				t.Errorf("Expected status 200 for Content-Type '%s', got %d", boundary, status)
			}
		})
	}
}

func TestAuth_MissingHeader_Respond401(t *testing.T) {
	h := Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	h.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") != `Bearer realm="trakrf-api"` {
		t.Errorf("missing/wrong WWW-Authenticate header: %q", w.Header().Get("WWW-Authenticate"))
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Title != "Authentication required" {
		t.Errorf("title = %q, want normalized", resp.Error.Title)
	}
	if resp.Error.Detail != "Authorization header is required" {
		t.Errorf("detail = %q, want canonical missing-header string", resp.Error.Detail)
	}
}

func TestAuth_MalformedHeader_Respond401(t *testing.T) {
	h := Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Basic abc123")
	h.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Detail != "Authorization header must be Bearer <token>" {
		t.Errorf("detail = %q, want canonical malformed-header string", resp.Error.Detail)
	}
}

func TestAuth_InvalidToken_Respond401(t *testing.T) {
	h := Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer not-a-valid-jwt")
	h.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Detail != "Bearer token is invalid or expired" {
		t.Errorf("detail = %q, want canonical invalid-token string", resp.Error.Detail)
	}
}

func TestAuth_BearerSchemeCaseInsensitive(t *testing.T) {
	cases := []string{"Bearer", "bearer", "BEARER", "BeArEr"}
	for _, scheme := range cases {
		t.Run(scheme, func(t *testing.T) {
			h := Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not reach handler for invalid token")
			}))
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/x", nil)
			r.Header.Set("Authorization", scheme+" not-a-valid-jwt")
			h.ServeHTTP(w, r)

			if w.Code != 401 {
				t.Fatalf("status = %d, want 401", w.Code)
			}
			var resp apierrors.ErrorResponse
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			// Must reach the token-validation branch, not the scheme-rejection branch.
			if resp.Error.Detail != "Bearer token is invalid or expired" {
				t.Errorf("detail = %q, want token-validation detail (scheme should have been accepted)", resp.Error.Detail)
			}
		})
	}
}

// APIKeyAuth unit tests (no DB required for early-exit branches).

func TestAPIKey_MissingHeader_Respond401(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	h := APIKeyAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	h.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") != `Bearer realm="trakrf-api"` {
		t.Errorf("missing/wrong WWW-Authenticate header: %q", w.Header().Get("WWW-Authenticate"))
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Title != "Authentication required" {
		t.Errorf("title = %q, want %q", resp.Error.Title, "Authentication required")
	}
	if resp.Error.Detail != "Authorization header is required" {
		t.Errorf("detail = %q, want canonical missing-header string", resp.Error.Detail)
	}
}

func TestAPIKey_MalformedHeader_Respond401(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	h := APIKeyAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Basic abc123")
	h.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") != `Bearer realm="trakrf-api"` {
		t.Errorf("missing/wrong WWW-Authenticate header: %q", w.Header().Get("WWW-Authenticate"))
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Detail != "Authorization header must be Bearer <token>" {
		t.Errorf("detail = %q, want canonical malformed-header string", resp.Error.Detail)
	}
}

func TestAPIKey_InvalidJWT_Respond401(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	h := APIKeyAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer not-a-valid-jwt")
	h.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") != `Bearer realm="trakrf-api"` {
		t.Errorf("missing/wrong WWW-Authenticate header: %q", w.Header().Get("WWW-Authenticate"))
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Detail != "Bearer token is invalid or expired" {
		t.Errorf("detail = %q, want canonical invalid-token string", resp.Error.Detail)
	}
}

func TestAPIKey_BearerSchemeCaseInsensitive(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	cases := []string{"Bearer", "bearer", "BEARER", "BeArEr"}
	for _, scheme := range cases {
		t.Run(scheme, func(t *testing.T) {
			h := APIKeyAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not reach handler for invalid token")
			}))
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/x", nil)
			r.Header.Set("Authorization", scheme+" not-a-valid-jwt")
			h.ServeHTTP(w, r)

			if w.Code != 401 {
				t.Fatalf("status = %d, want 401", w.Code)
			}
			var resp apierrors.ErrorResponse
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			// Must reach the token-validation branch, not the scheme-rejection branch.
			if resp.Error.Detail != "Bearer token is invalid or expired" {
				t.Errorf("detail = %q, want token-validation detail (scheme should have been accepted)", resp.Error.Detail)
			}
		})
	}
}

// TRA-449 D10: requests that send X-API-Key without Authorization should see
// a 401 detail hinting at the correct header format, not the generic
// missing-header string that sends integrators chasing credential issues.

func TestAuth_XAPIKeyWithoutAuthorization_HintsBearer(t *testing.T) {
	h := Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-API-Key", "some-token-value")
	h.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp.Error.Detail, "Authorization: Bearer") {
		t.Errorf("detail = %q, want a hint containing %q", resp.Error.Detail, "Authorization: Bearer")
	}
}

func TestAPIKey_XAPIKeyWithoutAuthorization_HintsBearer(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	h := APIKeyAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-API-Key", "some-token-value")
	h.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp.Error.Detail, "Authorization: Bearer") {
		t.Errorf("detail = %q, want a hint containing %q", resp.Error.Detail, "Authorization: Bearer")
	}
}
