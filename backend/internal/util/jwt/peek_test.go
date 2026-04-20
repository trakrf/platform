package jwt_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestPeekIssuer_APIKeyToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "peek-test")
	tok, err := jwt.GenerateAPIKey("jti-1", 42, []string{"assets:read"}, nil)
	require.NoError(t, err)

	iss, err := jwt.PeekIssuer(tok)
	require.NoError(t, err)
	assert.Equal(t, "trakrf-api-key", iss)
}

func TestPeekIssuer_SessionToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "peek-test")
	orgID := 7
	tok, err := jwt.Generate(1, "u@e.com", &orgID)
	require.NoError(t, err)

	iss, err := jwt.PeekIssuer(tok)
	require.NoError(t, err)
	assert.Equal(t, "", iss, "session JWTs carry no iss claim")
}

func TestPeekIssuer_Garbage(t *testing.T) {
	_, err := jwt.PeekIssuer("not-a-jwt")
	assert.Error(t, err)
}

func TestPeekIssuer_ExpiredTokenStillPeeks(t *testing.T) {
	t.Setenv("JWT_SECRET", "peek-test")
	past := time.Now().Add(-1 * time.Hour)
	tok, err := jwt.GenerateAPIKey("jti-exp", 42, []string{"x"}, &past)
	require.NoError(t, err)

	// Full validation would reject expired; peek should still return iss.
	iss, err := jwt.PeekIssuer(tok)
	require.NoError(t, err)
	assert.Equal(t, "trakrf-api-key", iss)
}
