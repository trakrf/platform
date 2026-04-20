package jwt

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// TokenKind identifies which auth chain should validate a signed token.
type TokenKind int

const (
	TokenKindUnknown TokenKind = iota
	TokenKindSession
	TokenKindAPIKey
)

// ClassifyToken verifies the JWT's HMAC signature against the shared secret
// and returns a kind based on the "iss" claim. Claim-validation (exp, nbf, etc.)
// is NOT performed here — the caller dispatches to the appropriate chain which
// re-runs full validation including expiry, issuer match, audience, and
// revocation checks.
//
// Returning a signature-verified classification lets middleware route between
// session and API-key validation chains without peeking at an unverified token.
func ClassifyToken(tokenString string) (TokenKind, error) {
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	_, err := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(getSecret()), nil
	})
	if err != nil {
		return TokenKindUnknown, fmt.Errorf("classify jwt: %w", err)
	}
	iss, _ := claims["iss"].(string)
	switch iss {
	case apiKeyIssuer:
		return TokenKindAPIKey, nil
	case "":
		return TokenKindSession, nil
	default:
		return TokenKindUnknown, fmt.Errorf("unknown issuer %q", iss)
	}
}
