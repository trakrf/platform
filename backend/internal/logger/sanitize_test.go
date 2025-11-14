package logger

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeHeaders(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		expected map[string]string
	}{
		{
			name: "Redacts Bearer token",
			headers: http.Header{
				"Authorization": []string{"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.secret"},
			},
			expected: map[string]string{
				"Authorization": "Bearer <redacted>",
			},
		},
		{
			name: "Redacts non-Bearer authorization",
			headers: http.Header{
				"Authorization": []string{"Basic dXNlcjpwYXNzd29yZA=="},
			},
			expected: map[string]string{
				"Authorization": "<redacted>",
			},
		},
		{
			name: "Redacts empty authorization header",
			headers: http.Header{
				"Authorization": []string{""},
			},
			expected: map[string]string{
				"Authorization": "<redacted>",
			},
		},
		{
			name: "Preserves non-sensitive headers",
			headers: http.Header{
				"Content-Type":   []string{"application/json"},
				"Accept":         []string{"application/json"},
				"User-Agent":     []string{"Mozilla/5.0"},
				"X-Request-ID":   []string{"test-id-123"},
				"Content-Length": []string{"1024"},
			},
			expected: map[string]string{
				"Content-Type":   "application/json",
				"Accept":         "application/json",
				"User-Agent":     "Mozilla/5.0",
				"X-Request-ID":   "test-id-123",
				"Content-Length": "1024",
			},
		},
		{
			name: "Handles mixed sensitive and non-sensitive headers",
			headers: http.Header{
				"Authorization": []string{"Bearer secret-token"},
				"Content-Type":  []string{"application/json"},
				"Accept":        []string{"*/*"},
			},
			expected: map[string]string{
				"Authorization": "Bearer <redacted>",
				"Content-Type":  "application/json",
				"Accept":        "*/*",
			},
		},
		{
			name: "Case-insensitive authorization header",
			headers: http.Header{
				"authorization": []string{"Bearer secret"},
				"AUTHORIZATION": []string{"Bearer secret2"},
			},
			expected: map[string]string{
				"authorization": "Bearer <redacted>",
				"AUTHORIZATION": "Bearer <redacted>",
			},
		},
		{
			name:     "Empty headers",
			headers:  http.Header{},
			expected: map[string]string{},
		},
		{
			name: "Headers with multiple values (only first is kept)",
			headers: http.Header{
				"Accept": []string{"application/json", "text/html"},
			},
			expected: map[string]string{
				"Accept": "application/json",
			},
		},
		{
			name: "Headers with empty value array",
			headers: http.Header{
				"X-Empty-Header": []string{},
			},
			expected: map[string]string{},
		},
		{
			name: "Real-world request headers",
			headers: http.Header{
				"Host":            []string{"api.example.com"},
				"User-Agent":      []string{"curl/7.68.0"},
				"Accept":          []string{"*/*"},
				"Content-Type":    []string{"application/json"},
				"Content-Length":  []string{"256"},
				"Authorization":   []string{"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.secret.signature"},
				"X-Request-ID":    []string{"req-12345"},
				"X-Forwarded-For": []string{"203.0.113.42"},
			},
			expected: map[string]string{
				"Host":            "api.example.com",
				"User-Agent":      "curl/7.68.0",
				"Accept":          "*/*",
				"Content-Type":    "application/json",
				"Content-Length":  "256",
				"Authorization":   "Bearer <redacted>",
				"X-Request-ID":    "req-12345",
				"X-Forwarded-For": "203.0.113.42",
			},
		},
		{
			name: "API Key in Authorization header",
			headers: http.Header{
				"Authorization": []string{"ApiKey sk_live_1234567890abcdef"},
			},
			expected: map[string]string{
				"Authorization": "<redacted>",
			},
		},
		{
			name: "Digest authentication",
			headers: http.Header{
				"Authorization": []string{"Digest username=\"user\", realm=\"example.com\""},
			},
			expected: map[string]string{
				"Authorization": "<redacted>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeHeaders(tt.headers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeHeadersCaseInsensitivity(t *testing.T) {
	t.Run("Lowercase authorization", func(t *testing.T) {
		headers := http.Header{
			"authorization": []string{"Bearer token"},
		}
		result := SanitizeHeaders(headers)
		assert.Equal(t, "Bearer <redacted>", result["authorization"])
	})

	t.Run("Uppercase AUTHORIZATION", func(t *testing.T) {
		headers := http.Header{
			"AUTHORIZATION": []string{"Bearer token"},
		}
		result := SanitizeHeaders(headers)
		assert.Equal(t, "Bearer <redacted>", result["AUTHORIZATION"])
	})

	t.Run("Mixed case Authorization", func(t *testing.T) {
		headers := http.Header{
			"Authorization": []string{"Bearer token"},
		}
		result := SanitizeHeaders(headers)
		assert.Equal(t, "Bearer <redacted>", result["Authorization"])
	})
}

func TestSanitizeHeadersEdgeCases(t *testing.T) {
	t.Run("Authorization with only 'Bearer'", func(t *testing.T) {
		headers := http.Header{
			"Authorization": []string{"Bearer"},
		}
		result := SanitizeHeaders(headers)
		// "Bearer" without a space is not a Bearer token
		assert.Equal(t, "<redacted>", result["Authorization"])
	})

	t.Run("Authorization with 'Bearer ' and space but no token", func(t *testing.T) {
		headers := http.Header{
			"Authorization": []string{"Bearer "},
		}
		result := SanitizeHeaders(headers)
		assert.Equal(t, "Bearer <redacted>", result["Authorization"])
	})

	t.Run("Authorization with whitespace", func(t *testing.T) {
		headers := http.Header{
			"Authorization": []string{"  Bearer token  "},
		}
		result := SanitizeHeaders(headers)
		// Exact string matching, so this won't start with "Bearer "
		assert.Equal(t, "<redacted>", result["Authorization"])
	})

	t.Run("Empty string in values array", func(t *testing.T) {
		headers := http.Header{
			"Content-Type": []string{""},
		}
		result := SanitizeHeaders(headers)
		assert.Equal(t, "", result["Content-Type"])
	})
}

func TestSanitizeHeadersPreservesOtherHeaders(t *testing.T) {
	commonHeaders := []string{
		"Accept",
		"Accept-Encoding",
		"Accept-Language",
		"Cache-Control",
		"Connection",
		"Content-Length",
		"Content-Type",
		"Cookie",
		"Host",
		"Origin",
		"Referer",
		"User-Agent",
		"X-Request-ID",
		"X-Forwarded-For",
		"X-Real-IP",
	}

	for _, headerName := range commonHeaders {
		t.Run("Preserves "+headerName, func(t *testing.T) {
			headers := http.Header{
				headerName: []string{"test-value"},
			}
			result := SanitizeHeaders(headers)
			assert.Equal(t, "test-value", result[headerName])
		})
	}
}
