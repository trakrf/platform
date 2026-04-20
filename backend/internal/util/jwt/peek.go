package jwt

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// PeekIssuer parses the JWT body without verifying signature or expiry,
// returning the "iss" claim. Used by middleware.EitherAuth to pick between
// session and API-key validation chains; full validation runs downstream.
// Safe because peek authorizes nothing on its own.
func PeekIssuer(tokenString string) (string, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	tok, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("peek jwt: %w", err)
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("peek jwt: unexpected claims type")
	}
	iss, _ := claims["iss"].(string)
	return iss, nil
}
