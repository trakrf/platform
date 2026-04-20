package middleware

import (
	"net/http"
	"strings"

	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

const apiKeyIssuer = "trakrf-api-key"

// EitherAuth dispatches a request to APIKeyAuth or session Auth based on the
// JWT's "iss" claim. Public read routes use this so the frontend (session) and
// external API-key callers share one handler registration.
//
// The peek at iss is unverified; the delegated chain runs full signature +
// expiry + revocation validation. Peek authorizes nothing on its own.
func EitherAuth(store *storage.Storage) func(http.Handler) http.Handler {
	apiChain := APIKeyAuth(store)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Missing authorization header", "", reqID)
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Invalid authorization header format", "", reqID)
				return
			}

			iss, err := jwt.PeekIssuer(parts[1])
			if err != nil {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Invalid or malformed token", "", reqID)
				return
			}

			switch iss {
			case apiKeyIssuer:
				apiChain(next).ServeHTTP(w, r)
			case "":
				Auth(next).ServeHTTP(w, r)
			default:
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Invalid or expired token", "", reqID)
			}
		})
	}
}
