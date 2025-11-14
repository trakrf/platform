package logger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRequestID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "Returns request ID from context",
			ctx:      context.WithValue(context.Background(), requestIDKey, "test-request-id"),
			expected: "test-request-id",
		},
		{
			name:     "Returns empty string when not set",
			ctx:      context.Background(),
			expected: "",
		},
		{
			name:     "Returns empty string when wrong type",
			ctx:      context.WithValue(context.Background(), requestIDKey, 123),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRequestID(tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMiddleware(t *testing.T) {
	// Initialize logger for middleware tests
	cfg := &Config{
		Environment:   EnvDev,
		ServiceName:   "test-service",
		Level:         "debug",
		Format:        "json",
		IncludeCaller: false,
		IncludeStack:  false,
		Version:       "1.0.0",
	}
	Initialize(cfg)

	tests := []struct {
		name           string
		method         string
		path           string
		requestID      string
		handlerStatus  int
		expectedStatus int
	}{
		{
			name:           "Successful GET request",
			method:         "GET",
			path:           "/api/v1/test",
			requestID:      "test-id-1",
			handlerStatus:  http.StatusOK,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST request with 201",
			method:         "POST",
			path:           "/api/v1/test",
			requestID:      "test-id-2",
			handlerStatus:  http.StatusCreated,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Request with 400 error",
			method:         "GET",
			path:           "/api/v1/bad-request",
			requestID:      "test-id-3",
			handlerStatus:  http.StatusBadRequest,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Request with 500 error",
			method:         "GET",
			path:           "/api/v1/server-error",
			requestID:      "test-id-4",
			handlerStatus:  http.StatusInternalServerError,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Request without request ID",
			method:         "GET",
			path:           "/api/v1/test",
			requestID:      "",
			handlerStatus:  http.StatusOK,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler that returns specific status
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.handlerStatus)
				w.Write([]byte(`{"message":"test"}`))
			})

			// Wrap with middleware
			wrapped := Middleware(handler)

			// Create request
			req := httptest.NewRequest(tt.method, tt.path, nil)

			// Add request ID to context if provided
			if tt.requestID != "" {
				ctx := context.WithValue(req.Context(), requestIDKey, tt.requestID)
				req = req.WithContext(ctx)
			}

			// Create response recorder
			rec := httptest.NewRecorder()

			// Execute request
			wrapped.ServeHTTP(rec, req)

			// Verify status code
			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestResponseWriter(t *testing.T) {
	t.Run("Captures status code on WriteHeader", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &responseWriter{
			ResponseWriter: rec,
			statusCode:     http.StatusOK, // default
		}

		rw.WriteHeader(http.StatusCreated)

		assert.Equal(t, http.StatusCreated, rw.statusCode)
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("Defaults to 200 OK when not explicitly set", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &responseWriter{
			ResponseWriter: rec,
			statusCode:     http.StatusOK,
		}

		// Write without calling WriteHeader
		rw.Write([]byte("test"))

		assert.Equal(t, http.StatusOK, rw.statusCode)
	})

	t.Run("Captures different status codes", func(t *testing.T) {
		testCases := []int{
			http.StatusOK,
			http.StatusCreated,
			http.StatusNoContent,
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusNotFound,
			http.StatusInternalServerError,
			http.StatusBadGateway,
		}

		for _, status := range testCases {
			rec := httptest.NewRecorder()
			rw := &responseWriter{
				ResponseWriter: rec,
				statusCode:     http.StatusOK,
			}

			rw.WriteHeader(status)

			assert.Equal(t, status, rw.statusCode, "Should capture status code %d", status)
		}
	})
}

func TestMiddlewareWithHeaders(t *testing.T) {
	// Initialize logger
	cfg := &Config{
		Environment:   EnvDev,
		ServiceName:   "test-service",
		Level:         "debug",
		Format:        "json",
		IncludeCaller: false,
		IncludeStack:  false,
		Version:       "1.0.0",
	}
	Initialize(cfg)

	t.Run("Logs request with headers", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := Middleware(handler)
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer secret-token")

		ctx := context.WithValue(req.Context(), requestIDKey, "test-id")
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestMiddlewareChaining(t *testing.T) {
	// Initialize logger
	cfg := &Config{
		Environment:   EnvDev,
		ServiceName:   "test-service",
		Level:         "debug",
		Format:        "json",
		IncludeCaller: false,
		IncludeStack:  false,
		Version:       "1.0.0",
	}
	Initialize(cfg)

	t.Run("Can chain with other middleware", func(t *testing.T) {
		// Create a simple middleware that adds a header
		addHeaderMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Custom-Header", "test-value")
				next.ServeHTTP(w, r)
			})
		}

		// Final handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Chain: addHeader -> Middleware -> handler
		wrapped := addHeaderMiddleware(Middleware(handler))

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		ctx := context.WithValue(req.Context(), requestIDKey, "test-id")
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "test-value", rec.Header().Get("X-Custom-Header"))
	})
}
