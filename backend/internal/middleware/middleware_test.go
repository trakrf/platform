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
		path           string
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
			name:           "POST to /api/v1/assets/bulk with multipart/form-data",
			method:         http.MethodPost,
			path:           "/api/v1/assets/bulk",
			contentType:    "multipart/form-data; boundary=----WebKitFormBoundary",
			expectedStatus: http.StatusOK,
			description:    "Internal CSV-upload endpoint accepts multipart",
		},
		// PUT requests with valid Content-Types
		{
			name:           "PUT with application/json",
			method:         http.MethodPut,
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			description:    "PUT with JSON",
		},
		// PATCH on the global middleware accepts both application/json and
		// application/merge-patch+json. Strict RFC 7396 single-CT enforcement
		// for PATCH lives in RequireMergePatchCT (per-route), so undeclared
		// PATCH probes against POST-only paths get chi's 405 instead of 415.
		{
			name:           "PATCH with application/merge-patch+json",
			method:         http.MethodPatch,
			contentType:    "application/merge-patch+json",
			expectedStatus: http.StatusOK,
			description:    "PATCH with merge-patch+json (RFC 7396)",
		},
		{
			name:           "PATCH with application/json (global middleware lets it through)",
			method:         http.MethodPatch,
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			description:    "Global middleware passes PATCH+json; per-route RequireMergePatchCT enforces strict",
		},
		// TRA-703 / BB32 D4: empty Content-Type rejected on every write method.
		{
			name:           "POST with empty Content-Type rejected (TRA-703)",
			method:         http.MethodPost,
			contentType:    "",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "Missing CT is a wrong CT per docs",
		},
		{
			name:           "PUT with empty Content-Type rejected (TRA-703)",
			method:         http.MethodPut,
			contentType:    "",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "Missing CT is a wrong CT per docs",
		},
		{
			name:           "PATCH with empty Content-Type rejected (TRA-703)",
			method:         http.MethodPatch,
			contentType:    "",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "Missing CT is a wrong CT per docs",
		},
		// TRA-703 / BB32 D4: merge-patch+json is PATCH-only on the public surface.
		{
			name:           "POST with application/merge-patch+json rejected (TRA-703)",
			method:         http.MethodPost,
			contentType:    "application/merge-patch+json",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "merge-patch+json is PATCH-only per public docs",
		},
		{
			name:           "PUT with application/merge-patch+json rejected (TRA-703)",
			method:         http.MethodPut,
			contentType:    "application/merge-patch+json",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "merge-patch+json is PATCH-only per public docs",
		},
		// TRA-703 / BB32 D4: multipart on a non-bulk POST path now 415.
		{
			name:           "POST to non-bulk path with multipart/form-data rejected (TRA-703)",
			method:         http.MethodPost,
			path:           "/api/v1/assets",
			contentType:    "multipart/form-data; boundary=----WebKitFormBoundary",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "Only /api/v1/assets/bulk accepts multipart",
		},
		{
			name:           "PUT with multipart/form-data rejected (TRA-703)",
			method:         http.MethodPut,
			contentType:    "multipart/form-data; boundary=----WebKitFormBoundary",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "PUT never accepts multipart",
		},
		// Bulk endpoint must reject non-multipart CT.
		{
			name:           "POST to /api/v1/assets/bulk with application/json rejected (TRA-703)",
			method:         http.MethodPost,
			path:           "/api/v1/assets/bulk",
			contentType:    "application/json",
			expectedStatus: http.StatusUnsupportedMediaType,
			description:    "Bulk endpoint requires multipart",
		},
		// Other invalid Content-Types
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
			description:    "CSV not allowed as Content-Type (multipart/form-data only on /api/v1/assets/bulk)",
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
			path := tt.path
			if path == "" {
				path = "/test"
			}
			req := httptest.NewRequest(tt.method, path, strings.NewReader("{}"))
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
			if tt.expectedStatus == http.StatusUnsupportedMediaType {
				var resp apierrors.ErrorResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if resp.Error.Type != string(apierrors.ErrUnsupportedMedia) {
					t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrUnsupportedMedia)
				}
				if resp.Error.Title != "Unsupported media type" {
					t.Errorf("title = %q, want Unsupported media type", resp.Error.Title)
				}
				if strings.Contains(resp.Error.Detail, "multipart") {
					t.Errorf("detail = %q, must not mention multipart", resp.Error.Detail)
				}
			}
		})
	}
}

// RequireMergePatchCT enforces RFC 7396 strict on PATCH-handling routes
// (BB28 W2/S4). Empty CT is allowed for backwards compatibility, matching
// the global ContentType policy. Per-route placement is deliberate: chi's
// 405 must fire for PATCH probes against POST-only paths.
func TestRequireMergePatchCT(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	tests := []struct {
		name           string
		contentType    string
		expectedStatus int
	}{
		{"merge-patch+json", "application/merge-patch+json", http.StatusOK},
		{"merge-patch+json with charset", "application/merge-patch+json; charset=utf-8", http.StatusOK},
		// TRA-703 / BB32 D4: missing CT is a wrong CT — 415 to match the
		// "every wrong Content-Type returns 415" docs promise.
		{"empty CT rejected", "", http.StatusUnsupportedMediaType},
		{"application/json rejected", "application/json", http.StatusUnsupportedMediaType},
		{"application/json with charset rejected", "application/json; charset=utf-8", http.StatusUnsupportedMediaType},
		{"multipart rejected", "multipart/form-data; boundary=----X", http.StatusUnsupportedMediaType},
		{"text/plain rejected", "text/plain", http.StatusUnsupportedMediaType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/assets/1", strings.NewReader("{}"))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rr := httptest.NewRecorder()
			RequireMergePatchCT(next).ServeHTTP(rr, req)
			if rr.Code != tt.expectedStatus {
				t.Errorf("got %d, want %d", rr.Code, tt.expectedStatus)
			}
			if tt.expectedStatus == http.StatusUnsupportedMediaType {
				var resp apierrors.ErrorResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if resp.Error.Detail != "Content-Type must be application/merge-patch+json on PATCH operations" {
					t.Errorf("detail = %q, want method-aware PATCH detail", resp.Error.Detail)
				}
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

// TestContentType_MultipartBoundary verifies the /api/v1/assets/bulk path
// accepts multipart with all the boundary forms a CSV-upload client emits.
// Probed at the bulk path because every other route now 415s multipart
// (TRA-703 / BB32 D4).
func TestContentType_MultipartBoundary(t *testing.T) {
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/bulk", nil)
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
	if resp.Error.Title != "Unauthorized" {
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
	if resp.Error.Title != "Unauthorized" {
		t.Errorf("title = %q, want %q", resp.Error.Title, "Unauthorized")
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

// TRA-685 F10: CORS-disabled deployments must NOT short-circuit OPTIONS to
// 204. With CORS disabled there is no preflight semantics to honor, and
// returning 204 with neither `Allow` nor `Access-Control-Allow-Methods` was
// worst-of-both. OPTIONS must fall through to the inner handler (which in
// production is chi's MethodNotAllowed → 405 with Allow).
func TestCORS_DisabledOriginPassesOptionsThrough(t *testing.T) {
	t.Setenv("BACKEND_CORS_ORIGIN", "disabled")
	reached := false
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/x", nil)
	h.ServeHTTP(w, r)

	if !reached {
		t.Fatalf("OPTIONS under disabled CORS must fall through to the next handler")
	}
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d (inner handler's response)", w.Code, http.StatusMethodNotAllowed)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty under disabled CORS", got)
	}
}

// TRA-685 F10: CORS-enabled deployments keep the preflight short-circuit —
// OPTIONS returns 204 with proper Access-Control-Allow-* headers and never
// reaches downstream middleware.
func TestCORS_EnabledOriginShortCircuitsOptions(t *testing.T) {
	t.Setenv("BACKEND_CORS_ORIGIN", "https://app.example.com")
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("CORS-enabled OPTIONS must not reach the next handler")
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/x", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Errorf("Access-Control-Allow-Methods must be set on preflight responses")
	}
}
