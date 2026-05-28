package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// apiAccessTokenTTL is the lifetime of an API access JWT. Short by design: a
// leaked access token self-expires quickly; the integrator silently refreshes.
const apiAccessTokenTTL = 15 * time.Minute

// APITokenResponse is the result of an OAuth2 token grant (client_credentials
// or refresh_token) for the public API.
type APITokenResponse struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// MintAPITokenPair issues a short-lived API access JWT + a rotating refresh
// token for an authenticated client_credentials request. The caller (handler)
// is responsible for authenticating the client first.
func (s *Service) MintAPITokenPair(ctx context.Context, jti string, scopes []string, orgID int, apiKeyID int64, userAgent, ip string) (accessToken, refreshSecret string, expiresIn int, err error) {
	exp := time.Now().Add(apiAccessTokenTTL)
	accessToken, err = jwt.GenerateAPIKey(jti, orgID, scopes, &exp)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to generate api access JWT: %w", err)
	}

	refreshSecret, err = generateRefreshSecret()
	if err != nil {
		return "", "", 0, err
	}

	orgIDVal := orgID
	_, err = s.storage.CreateAPIRefreshToken(
		ctx, apiKeyID, &orgIDVal, hashRefreshSecret(refreshSecret),
		time.Now().Add(refreshTokenTTL), userAgent, ip,
	)
	if err != nil {
		return "", "", 0, err
	}

	return accessToken, refreshSecret, int(apiAccessTokenTTL.Seconds()), nil
}

// RefreshAPIToken exchanges a current api refresh token for a new pair. Reuses
// the TRA-843 rotation + replay-detection semantics. Scopes are re-read from
// the api_keys row at mint time (single source of truth), so a scope change on
// the key takes effect on the next refresh.
func (s *Service) RefreshAPIToken(ctx context.Context, presentedSecret, userAgent, ip string) (*APITokenResponse, error) {
	hash := hashRefreshSecret(presentedSecret)
	row, err := s.storage.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("lookup refresh token: %w", err)
	}
	if row == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	// Only api-type tokens may be exchanged here; a session token is invalid.
	if row.TokenType != "api" || row.APIKeyID == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	if row.RevokedAt != nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	if row.UsedAt != nil {
		// Replay of an already-rotated token → chain compromise.
		if revokeErr := s.storage.RevokeRefreshTokenChain(ctx, row.ID); revokeErr != nil {
			fmt.Printf("Warning: failed to revoke api refresh chain after replay: %v\n", revokeErr)
		}
		// token_id alone identifies the chain for ops; api_key_id is omitted to
		// avoid logging a "*Key"-named field (CodeQL go/clear-text-logging).
		fmt.Printf("WARN api refresh-token replay detected token_id=%d\n", row.ID)
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	key, err := s.storage.GetAPIKeyByID(ctx, *row.APIKeyID)
	if err != nil || key == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	if key.RevokedAt != nil || (key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now())) {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	exp := time.Now().Add(apiAccessTokenTTL)
	accessToken, err := jwt.GenerateAPIKey(key.JTI, key.OrgID, key.Scopes, &exp)
	if err != nil {
		return nil, fmt.Errorf("failed to generate api access JWT: %w", err)
	}

	newSecret, err := generateRefreshSecret()
	if err != nil {
		return nil, err
	}

	_, err = s.storage.RotateAPIRefreshToken(
		ctx, row.ID, int64(key.ID), row.OrgID, hashRefreshSecret(newSecret),
		time.Now().Add(refreshTokenTTL), userAgent, ip,
	)
	if err != nil {
		return nil, fmt.Errorf("rotate api refresh token: %w", err)
	}

	return &APITokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newSecret,
		ExpiresIn:    int(apiAccessTokenTTL.Seconds()),
	}, nil
}
