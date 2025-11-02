package logger

import (
	"context"
	"net/http"
	"time"
)

type contextKey string

const requestIDKey contextKey = "requestID"

func getRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		return reqID
	}
	return ""
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := getRequestID(r.Context())

		logger := Get().With().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_ip", r.RemoteAddr).
			Logger()

		logger.Debug().
			Interface("headers", SanitizeHeaders(r.Header)).
			Msg("Request received")

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)

		logEvent := logger.Info().
			Int("status", wrapped.statusCode).
			Dur("duration_ms", duration).
			Int64("duration_ms_int", duration.Milliseconds())

		if wrapped.statusCode >= 400 {
			logEvent = logger.Warn().
				Int("status", wrapped.statusCode).
				Dur("duration_ms", duration).
				Int64("duration_ms_int", duration.Milliseconds())
		}

		logEvent.Msg("Request completed")
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
