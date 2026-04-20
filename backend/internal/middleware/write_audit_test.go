package middleware_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestWriteAudit_LogsAPIKeyPrincipal(t *testing.T) {
	var buf bytes.Buffer
	prev := logger.Get()
	defer logger.SetForTest(*prev)
	logger.SetForTest(zerolog.New(&buf))

	handler := middleware.WriteAudit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", strings.NewReader(`{}`))
	req = req.WithContext(middleware.WithAPIKeyPrincipalForTest(req.Context(), &middleware.APIKeyPrincipal{
		OrgID:  42,
		Scopes: []string{"assets:write"},
		JTI:    "jti-abc",
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), "audit middleware must emit a single JSON log line")
	assert.Equal(t, "api.write", line["event"])
	assert.EqualValues(t, 42, line["org_id"])
	assert.Equal(t, "api_key:jti-abc", line["principal"])
	assert.Equal(t, http.MethodPost, line["method"])
	assert.Equal(t, "/api/v1/assets", line["path"])
	assert.EqualValues(t, 201, line["status"])
}

func TestWriteAudit_LogsSessionPrincipal(t *testing.T) {
	var buf bytes.Buffer
	prev := logger.Get()
	defer logger.SetForTest(*prev)
	logger.SetForTest(zerolog.New(&buf))

	handler := middleware.WriteAudit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/7", strings.NewReader(`{}`))
	orgID := 17
	req = req.WithContext(middleware.WithUserClaimsForTest(req.Context(), &jwt.Claims{
		UserID:       99,
		CurrentOrgID: &orgID,
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "user:99", line["principal"])
	assert.EqualValues(t, 17, line["org_id"])
}

func TestWriteAudit_LogsUnauthenticatedWithZeroOrg(t *testing.T) {
	var buf bytes.Buffer
	prev := logger.Get()
	defer logger.SetForTest(*prev)
	logger.SetForTest(zerolog.New(&buf))

	handler := middleware.WriteAudit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/3", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "anonymous", line["principal"])
	assert.EqualValues(t, 0, line["org_id"])
	assert.EqualValues(t, 401, line["status"])
}
