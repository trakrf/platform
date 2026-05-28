package auth

import (
	"net/http"
	"time"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/apisecret"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// Token is the OAuth2 token endpoint for the public API.
//
// @Summary OAuth2 token grant
// @Description Exchange API credentials for a short-lived (15 min) access token + a rotating 30-day refresh token. Two grants are supported. `client_credentials`: supply `client_id` and `client_secret` (the opaque {client_id, client_secret} pair returned once at API-key creation). `refresh_token`: supply a current `refresh_token` to rotate the pair; presenting an already-used refresh token revokes the whole chain and returns 401.
// @Tags oauth,public
// @Accept json
// @Produce json
// @Param request body auth.TokenRequest true "Token grant request"
// @Success 200 {object} auth.TokenResponse
// @Failure 400 {object} errors.ErrorResponse "Validation error / unsupported grant_type"
// @Failure 401 {object} errors.ErrorResponse "Invalid client credentials or refresh token"
// @Failure 404 {object} errors.ErrorResponse "not_found"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Failure 429 {object} errors.ErrorResponse "rate_limited"
// @Router /api/v1/oauth/token [post]
func (handler *Handler) Token(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var request auth.TokenRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}

	switch request.GrantType {
	case "client_credentials":
		handler.tokenClientCredentials(w, r, request)
	case "refresh_token":
		handler.tokenRefresh(w, r, request)
	default:
		// Unreachable: validator constrains grant_type via oneof. Defensive.
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			"Unsupported grant_type", reqID)
	}
}

func (handler *Handler) tokenClientCredentials(w http.ResponseWriter, r *http.Request, request auth.TokenRequest) {
	reqID := middleware.GetRequestID(r.Context())

	if request.ClientID == "" || request.ClientSecret == "" {
		httputil.Respond401(w, r, "client_id and client_secret are required for client_credentials", reqID)
		return
	}

	// Authenticate the client: client_id is the api_keys.jti, client_secret is
	// the opaque secret shown once at creation and stored only as a hash.
	key, err := handler.store.GetAPIKeyByJTI(r.Context(), request.ClientID)
	if err != nil || key == nil {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}
	if !apisecret.Verify(request.ClientSecret, key.SecretHash) {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}
	if key.RevokedAt != nil || (key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now())) {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}

	access, refresh, expiresIn, err := handler.service.MintAPITokenPair(
		r.Context(), key.JTI, key.Scopes, key.OrgID, int64(key.ID), r.UserAgent(), clientIP(r),
	)
	if err != nil {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, auth.TokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
	})
}

func (handler *Handler) tokenRefresh(w http.ResponseWriter, r *http.Request, request auth.TokenRequest) {
	reqID := middleware.GetRequestID(r.Context())

	if request.RefreshToken == "" {
		httputil.Respond401(w, r, "refresh_token is required for refresh_token grant", reqID)
		return
	}

	resp, err := handler.service.RefreshAPIToken(r.Context(), request.RefreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		httputil.Respond401(w, r, "Invalid or expired refresh token", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, auth.TokenResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    resp.ExpiresIn,
	})
}
