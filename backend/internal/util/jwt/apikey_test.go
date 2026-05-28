package jwt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateAccessToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-abc123")

	jti := "11111111-2222-3333-4444-555555555555"
	orgID := 42
	scopes := []string{"assets:read", "locations:read"}

	exp := time.Now().Add(15 * time.Minute)
	token, err := GenerateAccessToken(jti, orgID, scopes, &exp)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, jti, claims.Subject)
	assert.Equal(t, orgID, claims.OrgID)
	assert.Equal(t, scopes, claims.Scopes)
	assert.Equal(t, "trakrf-api-key", claims.Issuer)
	assert.Contains(t, claims.Audience, "trakrf-api")
	assert.NotNil(t, claims.ExpiresAt)
}

func TestValidateAccessTokenRejectsMissingExp(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-abc123")

	// A token minted with no expiry (the deprecated long-lived shape) must be rejected.
	token, err := GenerateAccessToken("some-jti", 1, []string{"assets:read"}, nil)
	require.NoError(t, err)

	_, err = ValidateAccessToken(token)
	assert.Error(t, err, "access token without exp must be rejected")
}

func TestGenerateAccessTokenWithExpiry(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-abc123")

	exp := time.Now().Add(24 * time.Hour)
	token, err := GenerateAccessToken("jti", 1, []string{"assets:read"}, &exp)
	require.NoError(t, err)

	claims, err := ValidateAccessToken(token)
	require.NoError(t, err)
	require.NotNil(t, claims.ExpiresAt)
	assert.WithinDuration(t, exp, claims.ExpiresAt.Time, time.Second)
}

func TestValidateAccessTokenRejectsSessionToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-abc123")

	sessionToken, err := Generate(1, "user@example.com", intPtr(42))
	require.NoError(t, err)

	_, err = ValidateAccessToken(sessionToken)
	assert.Error(t, err, "session token must not validate as an api-key token")
}

func TestValidateAccessTokenRejectsExpired(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-abc123")

	past := time.Now().Add(-1 * time.Hour)
	token, err := GenerateAccessToken("jti", 1, []string{"assets:read"}, &past)
	require.NoError(t, err)

	_, err = ValidateAccessToken(token)
	assert.Error(t, err)
}

func TestValidateAccessTokenRejectsBadSignature(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-abc123")

	token, err := GenerateAccessToken("jti", 1, []string{"assets:read"}, nil)
	require.NoError(t, err)

	t.Setenv("JWT_SECRET", "different-secret")
	_, err = ValidateAccessToken(token)
	assert.Error(t, err)
}

// TestValidateRejectsAPIKeyToken verifies the session-side discriminator:
// an API-key JWT must NOT parse as a valid session token even though both
// share the signing secret. Without this guard, the session middleware
// would silently accept an API-key JWT with zero-value UserID/CurrentOrgID,
// and downstream handlers would fail with misleading "missing org context"
// errors instead of a clear 401.
func TestValidateRejectsAPIKeyToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-abc123")

	apiToken, err := GenerateAccessToken("jti", 42, []string{"assets:read"}, nil)
	require.NoError(t, err)

	_, err = Validate(apiToken)
	assert.Error(t, err, "session Validate must reject api-key tokens")
}

func intPtr(i int) *int { return &i }
