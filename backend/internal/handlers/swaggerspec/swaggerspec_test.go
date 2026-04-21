package swaggerspec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServePublicJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()

	ServePublicJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var spec map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &spec), "body must be valid JSON")
	require.Contains(t, spec, "openapi", "spec must contain top-level openapi field")
}
