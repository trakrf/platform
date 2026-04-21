package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	apiKeyIssuer   = "trakrf-api-key"
	apiKeyAudience = "trakrf-api"
)

// APIKeyClaims carries the authorization context encoded into an API-key JWT.
type APIKeyClaims struct {
	OrgID  int      `json:"org_id"`
	Scopes []string `json:"scopes"`
	jwt.RegisteredClaims
}

// GenerateAPIKey mints a signed JWT for a newly-created api_keys row.
// sub is the row's jti (UUID string). exp is optional; nil means no expiry claim.
func GenerateAPIKey(jti string, orgID int, scopes []string, exp *time.Time) (string, error) {
	registered := jwt.RegisteredClaims{
		Issuer:   apiKeyIssuer,
		Subject:  jti,
		Audience: jwt.ClaimStrings{apiKeyAudience},
		IssuedAt: jwt.NewNumericDate(time.Now()),
	}
	if exp != nil {
		registered.ExpiresAt = jwt.NewNumericDate(*exp)
	}

	claims := &APIKeyClaims{
		OrgID:            orgID,
		Scopes:           scopes,
		RegisteredClaims: registered,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(getSecret()))
	if err != nil {
		return "", fmt.Errorf("sign api-key jwt: %w", err)
	}
	return signed, nil
}

// ValidateAPIKey verifies signature, iss, aud, and exp. Does not consult the DB.
func ValidateAPIKey(tokenString string) (*APIKeyClaims, error) {
	claims := &APIKeyClaims{}

	parser := jwt.NewParser(
		jwt.WithIssuer(apiKeyIssuer),
		jwt.WithAudience(apiKeyAudience),
	)

	token, err := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(getSecret()), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse api-key jwt: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid api-key jwt")
	}
	return claims, nil
}
