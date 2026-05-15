package middleware

import (
	"net/http"
	"strings"

	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// EitherAuth dispatches a request to APIKeyAuth or session Auth based on a
// signature-verified classification of the JWT. Public read routes use this
// so the frontend (session) and external API-key callers share one handler
// registration.
//
// Classification verifies the HMAC signature against the shared secret before
// reading the "iss" claim. The dispatched chain then runs full claim
// validation (expiry, issuer, audience) and — for API-key tokens — the DB
// checks for revocation, expiry, and last-used bump.
func EitherAuth(store *storage.Storage) func(http.Handler) http.Handler {
	apiChain := APIKeyAuth(store)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httputil.Respond401(w, r, missingAuthDetail(r, Detail401MissingAuthHeader), reqID)
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httputil.Respond401(w, r, Detail401InvalidAuthFormat, reqID)
				return
			}

			kind, err := jwt.ClassifyToken(parts[1])
			if err != nil {
				httputil.Respond401(w, r, Detail401InvalidOrExpiredToken, reqID)
				return
			}

			switch kind {
			case jwt.TokenKindAPIKey:
				apiChain(next).ServeHTTP(w, r)
			case jwt.TokenKindSession:
				Auth(next).ServeHTTP(w, r)
			default:
				httputil.Respond401(w, r, Detail401InvalidOrExpiredToken, reqID)
			}
		})
	}
}
