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

// Flush makes the wrapper transparent to streaming responses (SSE, TRA-924):
// it delegates to the underlying writer when that supports flushing. Without
// this, the sentry fancy-writer above us asserts its wrapped writer is an
// http.Flusher and panics when streaming.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying writer so http.ResponseController can reach
// capabilities we don't proxy (e.g. SetWriteDeadline, needed to keep a
// long-lived SSE stream alive past the server WriteTimeout).
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
