package swaggerspec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetPublicSpecResolution lets per-test environment changes take effect.
// Production code resolves the spec lazily via sync.Once; tests need to
// re-run the resolver after flipping APP_ENV.
func resetPublicSpecResolution(t *testing.T) {
	t.Helper()
	publicSpecOnce = sync.Once{}
	publicJSONServed = nil
	publicYAMLServed = nil
	t.Cleanup(func() {
		publicSpecOnce = sync.Once{}
		publicJSONServed = nil
		publicYAMLServed = nil
	})
}

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

func TestServePublicYAML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec := httptest.NewRecorder()

	ServePublicYAML(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/yaml", rec.Header().Get("Content-Type"))
	require.NotEmpty(t, rec.Body.Bytes(), "body must be non-empty")
	require.Contains(t, rec.Body.String(), "openapi:", "body should contain YAML key 'openapi:'")
}

// TRA-717 / BB34 F4: info.contact.url is environment-aware at serve
// time. The committed spec hard-codes the production canonical URL;
// when APP_ENV=preview the backend swaps it to the preview equivalent
// so a spec pulled from app.preview.trakrf.id/api/v1/openapi.* reads
// preview, matching the env-aware servers[] block.
func TestPublicSpec_ContactURLPreviewSubstitution(t *testing.T) {
	resetPublicSpecResolution(t)
	t.Setenv("APP_ENV", "preview")

	for _, c := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"json", ServePublicJSON},
		{"yaml", ServePublicYAML},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			resetPublicSpecResolution(t)
			t.Setenv("APP_ENV", "preview")
			rec := httptest.NewRecorder()
			c.handler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			body := rec.Body.String()
			assert.Contains(t, body, previewContactURL,
				"preview spec must carry the preview contact URL")
			// The production marketing-app contact URL (with /api suffix) must
			// be absent. Bare-hostname servers[] entries stay untouched.
			assert.False(t, strings.Contains(body, canonicalContactURL),
				"preview spec must not carry the production contact URL")
		})
	}
}

// On production (and any non-preview environment), the served spec
// equals the committed bytes verbatim — no substitution.
func TestPublicSpec_ContactURLProductionUnchanged(t *testing.T) {
	resetPublicSpecResolution(t)
	t.Setenv("APP_ENV", "production")

	rec := httptest.NewRecorder()
	ServePublicJSON(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Contains(t, rec.Body.String(), canonicalContactURL,
		"non-preview spec must carry the production contact URL")
	assert.False(t, strings.Contains(rec.Body.String(), previewContactURL),
		"non-preview spec must not carry the preview contact URL")
}
