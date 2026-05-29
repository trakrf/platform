package jwt

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID       int    `json:"user_id"`
	Email        string `json:"email"`
	CurrentOrgID *int   `json:"current_org_id,omitempty"`
	jwt.RegisteredClaims
}

// Generate creates a signed JWT token for an authenticated user.
func Generate(userID int, email string, orgID *int) (string, error) {
	expiration := getExpiration()
	expirationTime := time.Now().Add(time.Duration(expiration) * time.Second)

	claims := &Claims{
		UserID:       userID,
		Email:        email,
		CurrentOrgID: orgID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString([]byte(getSecret()))
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return tokenString, nil
}

// Validate parses and validates a session JWT.
//
// Session and API-key JWTs share the signing secret (TRA-393 / TRA-392 design),
// so a valid API-key JWT would otherwise parse cleanly against the session
// claims struct with zero-value UserID / CurrentOrgID and slip through.
// Reject them explicitly by issuer — session JWTs carry no iss, API-key JWTs
// carry "trakrf-api-key".
func Validate(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getSecret()), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid JWT token")
	}

	if claims.Issuer == apiKeyIssuer {
		return nil, fmt.Errorf("api-key token cannot be used for session auth")
	}

	return claims, nil
}

// devSecret is the fallback signing secret for non-production environments when
// JWT_SECRET is unset. It is publicly known, so ValidateSecret refuses to let it
// (or other known-weak values) sign tokens in production.
const devSecret = "dev-secret-change-in-production"

func getSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = devSecret
	}
	return secret
}

// knownWeakSecrets are signing secrets that must never sign tokens in a deployed
// environment: the unset/empty fallback (resolves to devSecret), devSecret
// itself, and the helm chart default "change-me" (helm/trakrf-backend
// values.yaml). All are publicly known, so any would let anyone forge a Bearer
// for any org.
var knownWeakSecrets = map[string]bool{
	"":          true,
	devSecret:   true,
	"change-me": true,
}

// devOrTestEnvs are the only environments permitted to boot with a weak/dev
// JWT_SECRET: local development (APP_ENV unset) and the test/CI harness
// (APP_ENV="test", which backend/justfile's contract-test recipe sets with the
// dev fallback secret). Every other APP_ENV — preview, staging, production — is
// a deployed environment that must supply a real secret.
var devOrTestEnvs = map[string]bool{
	"":     true,
	"test": true,
}

// ValidateSecret fails fast when a DEPLOYED environment lacks a real signing
// secret. Call it once at startup so the process refuses to boot rather than
// silently signing forgeable tokens with a publicly-known default.
//
// Scope: only local (APP_ENV unset) and the test/CI harness (APP_ENV="test")
// may use the dev fallback. Any other APP_ENV (preview, staging, production) is
// a deployed environment and must provide a real secret — so a misconfig
// fail-boots loudly on the preview proving ground, not silently in production.
// Enforcing on preview is safe: preview already has a real secret (it boots
// normally) and only fail-boots if its secret ever regresses to a weak value.
func ValidateSecret() error {
	appEnv := os.Getenv("APP_ENV")
	if devOrTestEnvs[appEnv] {
		return nil
	}
	if knownWeakSecrets[os.Getenv("JWT_SECRET")] {
		return fmt.Errorf("JWT_SECRET must be set to a real, non-default value in %q "+
			"(it is unset or a known-weak default — \"\", %q, or \"change-me\"); "+
			"refusing to start to avoid signing forgeable tokens", appEnv, devSecret)
	}
	return nil
}

// GetExpirationSeconds returns the configured access-token TTL in seconds.
// Exposed so callers issuing a token-pair can advertise expires_in to clients.
func GetExpirationSeconds() int {
	return getExpiration()
}

func getExpiration() int {
	exp := os.Getenv("JWT_EXPIRATION")
	if exp == "" {
		return 3600
	}

	seconds, err := strconv.Atoi(exp)
	if err != nil || seconds <= 0 {
		return 3600
	}

	return seconds
}
