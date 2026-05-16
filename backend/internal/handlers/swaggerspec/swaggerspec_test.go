package swaggerspec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// expectedCacheControl is the Cache-Control directive every spec handler
// must emit. TRA-743 made docs.trakrf.id redirect to this origin, so all
// four endpoints (internal+public × json+yaml) share the same caching
// posture for CF edge.
const expectedCacheControl = "public, max-age=60, stale-while-revalidate=300"

func TestServePublicJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()

	ServePublicJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Equal(t, expectedCacheControl, rec.Header().Get("Cache-Control"))

	var spec map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &spec), "body must be valid JSON")
	require.Contains(t, spec, "openapi", "spec must contain top-level openapi field")
}

func TestServePublicYAML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec := httptest.NewRecorder()

	ServePublicYAML(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/yaml", rec.Header().Get("Content-Type"))
	require.Equal(t, expectedCacheControl, rec.Header().Get("Cache-Control"))
	require.NotEmpty(t, rec.Body.Bytes(), "body must be non-empty")
	require.Contains(t, rec.Body.String(), "openapi:", "body should contain YAML key 'openapi:'")
}

func TestServeJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	ServeJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Equal(t, expectedCacheControl, rec.Header().Get("Cache-Control"))

	var spec map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &spec), "body must be valid JSON")
	require.Contains(t, spec, "openapi", "spec must contain top-level openapi field")
}

func TestServeYAML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()

	ServeYAML(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/yaml", rec.Header().Get("Content-Type"))
	require.Equal(t, expectedCacheControl, rec.Header().Get("Cache-Control"))
	require.NotEmpty(t, rec.Body.Bytes(), "body must be non-empty")
	require.Contains(t, rec.Body.String(), "openapi:", "body should contain YAML key 'openapi:'")
}
