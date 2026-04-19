package jwt

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestGenerateAndValidateAPIKey(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    jti := "11111111-2222-3333-4444-555555555555"
    orgID := 42
    scopes := []string{"assets:read", "locations:read"}

    token, err := GenerateAPIKey(jti, orgID, scopes, nil)
    require.NoError(t, err)
    require.NotEmpty(t, token)

    claims, err := ValidateAPIKey(token)
    require.NoError(t, err)
    assert.Equal(t, jti, claims.Subject)
    assert.Equal(t, orgID, claims.OrgID)
    assert.Equal(t, scopes, claims.Scopes)
    assert.Equal(t, "trakrf-api-key", claims.Issuer)
    assert.Contains(t, claims.Audience, "trakrf-api")
    assert.Nil(t, claims.ExpiresAt)
}

func TestGenerateAPIKeyWithExpiry(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    exp := time.Now().Add(24 * time.Hour)
    token, err := GenerateAPIKey("jti", 1, []string{"assets:read"}, &exp)
    require.NoError(t, err)

    claims, err := ValidateAPIKey(token)
    require.NoError(t, err)
    require.NotNil(t, claims.ExpiresAt)
    assert.WithinDuration(t, exp, claims.ExpiresAt.Time, time.Second)
}

func TestValidateAPIKeyRejectsSessionToken(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    sessionToken, err := Generate(1, "user@example.com", intPtr(42))
    require.NoError(t, err)

    _, err = ValidateAPIKey(sessionToken)
    assert.Error(t, err, "session token must not validate as an api-key token")
}

func TestValidateAPIKeyRejectsExpired(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    past := time.Now().Add(-1 * time.Hour)
    token, err := GenerateAPIKey("jti", 1, []string{"assets:read"}, &past)
    require.NoError(t, err)

    _, err = ValidateAPIKey(token)
    assert.Error(t, err)
}

func TestValidateAPIKeyRejectsBadSignature(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    token, err := GenerateAPIKey("jti", 1, []string{"assets:read"}, nil)
    require.NoError(t, err)

    t.Setenv("JWT_SECRET", "different-secret")
    _, err = ValidateAPIKey(token)
    assert.Error(t, err)
}

func intPtr(i int) *int { return &i }
