package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// APIKeyPrincipal is the authenticated-call identity for the public API.
type APIKeyPrincipal struct {
	OrgID  int
	Scopes []string
	JTI    string
}

const APIKeyPrincipalKey contextKey = "api_key_principal"

// GetAPIKeyPrincipal returns the principal if this request was authenticated via APIKeyAuth.
func GetAPIKeyPrincipal(r *http.Request) *APIKeyPrincipal {
	p, _ := r.Context().Value(APIKeyPrincipalKey).(*APIKeyPrincipal)
	return p
}

// APIKeyAuth validates an API-key JWT, looks up its DB record, and sets the principal on context.
func APIKeyAuth(store *storage.Storage) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httputil.Respond401(w, r, missingAuthDetail(r, "Authorization header is required"), reqID)
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				httputil.Respond401(w, r, "Authorization header must be Bearer <token>", reqID)
				return
			}

			claims, err := jwt.ValidateAPIKey(parts[1])
			if err != nil {
				logger.Get().Warn().Err(err).Str("request_id", reqID).Msg("api key jwt validation failed")
				httputil.Respond401(w, r, "Bearer token is invalid or expired", reqID)
				return
			}

			key, err := store.GetAPIKeyByJTI(r.Context(), claims.Subject)
			if err != nil {
				logger.Get().Warn().Err(err).Str("jti", claims.Subject).Str("request_id", reqID).
					Msg("api key lookup failed")
				httputil.Respond401(w, r, "Bearer token is invalid or expired", reqID)
				return
			}
			if key.RevokedAt != nil {
				logger.Get().Warn().Str("jti", key.JTI).Str("reason", "revoked").Str("request_id", reqID).
					Msg("api key rejected")
				httputil.Respond401(w, r, "API key has been revoked", reqID)
				return
			}
			if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
				logger.Get().Warn().Str("jti", key.JTI).Str("reason", "expired").Str("request_id", reqID).
					Msg("api key rejected")
				httputil.Respond401(w, r, "API key has expired", reqID)
				return
			}

			// Fire-and-forget last_used_at bump. Logs but doesn't fail the request.
			go func(jti string) {
				if err := store.UpdateAPIKeyLastUsed(context.Background(), jti); err != nil {
					logger.Get().Error().Err(err).Str("jti", jti).Msg("last_used_at update failed")
				}
			}(key.JTI)

			principal := &APIKeyPrincipal{
				OrgID:  key.OrgID,
				Scopes: key.Scopes,
				JTI:    key.JTI,
			}
			ctx := context.WithValue(r.Context(), APIKeyPrincipalKey, principal)
			logger.Get().Info().
				Int("org_id", principal.OrgID).
				Str("jti", principal.JTI).
				Str("request_id", reqID).
				Msg("api key auth success")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireScope rejects API-key requests whose principal lacks the given scope.
// Session-auth requests (UserClaims present) pass through; their access is
// governed elsewhere. Must be chained after EitherAuth or APIKeyAuth.
func RequireScope(required string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			// Session principal → pass through.
			if GetUserClaims(r) != nil {
				next.ServeHTTP(w, r)
				return
			}

			p := GetAPIKeyPrincipal(r)
			if p == nil {
				httputil.Respond401(w, r, "Authorization header is required", reqID)
				return
			}
			for _, s := range p.Scopes {
				if s == required {
					next.ServeHTTP(w, r)
					return
				}
			}
			httputil.WriteJSONError(w, r, http.StatusForbidden,
				errors.ErrForbidden, "Forbidden",
				"Missing required scope: "+required, reqID)
		})
	}
}

// WithAPIKeyPrincipalForTest attaches an APIKey principal to the context.
// Exported for tests only.
func WithAPIKeyPrincipalForTest(ctx context.Context, p *APIKeyPrincipal) context.Context {
	return context.WithValue(ctx, APIKeyPrincipalKey, p)
}
