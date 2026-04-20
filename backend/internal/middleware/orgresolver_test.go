package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestGetRequestOrgID_APIKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), APIKeyPrincipalKey, &APIKeyPrincipal{OrgID: 99, Scopes: []string{"x"}})
	req = req.WithContext(ctx)

	org, err := GetRequestOrgID(req)
	assert.NoError(t, err)
	assert.Equal(t, 99, org)
}

func TestGetRequestOrgID_Session(t *testing.T) {
	orgID := 42
	claims := &jwt.Claims{UserID: 1, Email: "u@e.com", CurrentOrgID: &orgID}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), UserClaimsKey, claims)
	req = req.WithContext(ctx)

	org, err := GetRequestOrgID(req)
	assert.NoError(t, err)
	assert.Equal(t, 42, org)
}

func TestGetRequestOrgID_SessionWithoutOrg(t *testing.T) {
	claims := &jwt.Claims{UserID: 1, Email: "u@e.com", CurrentOrgID: nil}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), UserClaimsKey, claims)
	req = req.WithContext(ctx)

	_, err := GetRequestOrgID(req)
	assert.Error(t, err)
}

func TestGetRequestOrgID_NoPrincipal(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := GetRequestOrgID(req)
	assert.Error(t, err)
}
