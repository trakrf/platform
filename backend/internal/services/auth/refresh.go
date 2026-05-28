package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// refreshTokenTTL is the lifetime of a refresh token. 30 days mirrors the
// typical "stay signed in" expectation; rotated single-use so a stolen
// secret only buys an attacker until the next legit refresh.
const refreshTokenTTL = 30 * 24 * time.Hour

// refreshSecretBytes controls the entropy of the opaque refresh secret. 32
// bytes → 64 hex chars, matches the password-reset token width.
const refreshSecretBytes = 32

// generateRefreshSecret returns a cryptographically random hex string.
func generateRefreshSecret() (string, error) {
	buf := make([]byte, refreshSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate refresh secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// hashRefreshSecret returns the SHA-256 hex digest of an opaque secret.
// Only the digest is persisted; the secret itself lives client-side.
func hashRefreshSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// MintTokenPair issues a fresh access JWT + refresh token row for a user.
// Returns the access JWT, the opaque refresh secret (only chance to see it
// in cleartext), and the access TTL in seconds.
func (s *Service) MintTokenPair(ctx context.Context, userID int, email string, orgID *int, userAgent, ip string, generateJWT func(int, string, *int) (string, error)) (accessToken, refreshSecret string, expiresIn int, err error) {
	accessToken, err = generateJWT(userID, email, orgID)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to generate access JWT: %w", err)
	}

	refreshSecret, err = generateRefreshSecret()
	if err != nil {
		return "", "", 0, err
	}

	_, err = s.storage.CreateRefreshToken(
		ctx, userID, orgID, hashRefreshSecret(refreshSecret),
		time.Now().Add(refreshTokenTTL), userAgent, ip,
	)
	if err != nil {
		return "", "", 0, err
	}

	return accessToken, refreshSecret, jwt.GetExpirationSeconds(), nil
}

// Refresh exchanges a valid refresh secret for a new access+refresh pair.
//
// On a refresh token that has already been used we treat the request as a
// compromise indicator: the active chain (every token reachable through
// replaced_by from this row) is revoked and an error is returned. This is
// the OAuth2 refresh-token-rotation replay-detection pattern.
func (s *Service) Refresh(ctx context.Context, presentedSecret, userAgent, ip string, generateJWT func(int, string, *int) (string, error)) (*auth.RefreshResponse, error) {
	hash := hashRefreshSecret(presentedSecret)
	row, err := s.storage.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("lookup refresh token: %w", err)
	}
	if row == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	if row.RevokedAt != nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	if row.UsedAt != nil {
		// Replay of an already-rotated token → chain compromise.
		if revokeErr := s.storage.RevokeRefreshTokenChain(ctx, row.ID); revokeErr != nil {
			fmt.Printf("Warning: failed to revoke refresh chain after replay: %v\n", revokeErr)
		}
		fmt.Printf("WARN refresh-token replay detected user_id=%d token_id=%d\n", row.UserID, row.ID)
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	if time.Now().After(row.ExpiresAt) {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	if row.UserID == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	usr, err := s.storage.GetUserByID(ctx, *row.UserID)
	if err != nil || usr == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	accessToken, err := generateJWT(usr.ID, usr.Email, row.OrgID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access JWT: %w", err)
	}

	newSecret, err := generateRefreshSecret()
	if err != nil {
		return nil, err
	}

	_, err = s.storage.RotateRefreshToken(
		ctx, row.ID, *row.UserID, row.OrgID, hashRefreshSecret(newSecret),
		time.Now().Add(refreshTokenTTL), userAgent, ip,
	)
	if err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	return &auth.RefreshResponse{
		AccessToken:  accessToken,
		RefreshToken: newSecret,
		ExpiresIn:    jwt.GetExpirationSeconds(),
	}, nil
}

// Logout revokes the supplied refresh token. Tolerant of unknown tokens —
// reveals nothing to a caller fishing for valid hashes.
func (s *Service) Logout(ctx context.Context, presentedSecret string) error {
	if presentedSecret == "" {
		return nil
	}
	row, err := s.storage.GetRefreshTokenByHash(ctx, hashRefreshSecret(presentedSecret))
	if err != nil {
		return fmt.Errorf("lookup refresh token: %w", err)
	}
	if row == nil {
		return nil
	}
	return s.storage.RevokeRefreshToken(ctx, row.ID)
}
