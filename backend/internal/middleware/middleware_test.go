package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
