package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/trakrf/platform/backend/internal/logger"
)

// statusRecorder intercepts the response status without buffering the body.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// WriteAudit logs one structured line per write request: principal, org, method,
// path, status, request_id. Intended to be mounted only on the public write
// route group — does not itself enforce any auth or scope.
func WriteAudit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		principal := "anonymous"
		orgID := 0

		if p := GetAPIKeyPrincipal(r); p != nil {
			principal = "api_key:" + p.JTI
			orgID = p.OrgID
		} else if c := GetUserClaims(r); c != nil {
			principal = "user:" + strconv.Itoa(c.UserID)
			if c.CurrentOrgID != nil {
				orgID = *c.CurrentOrgID
			}
		}

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		logger.Get().Info().
			Str("event", "api.write").
			Str("principal", principal).
			Int("org_id", orgID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", status).
			Str("request_id", GetRequestID(r.Context())).
			Msg(fmt.Sprintf("%s %s", r.Method, r.URL.Path))
	})
}
