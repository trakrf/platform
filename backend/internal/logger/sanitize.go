package logger

import (
	"net/http"
	"strings"
)

func SanitizeHeaders(headers http.Header) map[string]string {
	sanitized := make(map[string]string)

	for key, values := range headers {
		lowerKey := strings.ToLower(key)

		if lowerKey == "authorization" {
			if len(values) > 0 && strings.HasPrefix(values[0], "Bearer ") {
				sanitized[key] = "Bearer <redacted>"
			} else {
				sanitized[key] = "<redacted>"
			}
			continue
		}

		if len(values) > 0 {
			sanitized[key] = values[0]
		}
	}

	return sanitized
}
