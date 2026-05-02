package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// AC6: github.com/... module paths must not appear in the error.detail surface.
// Sanitize at the WriteJSONError boundary so every error response is scrubbed
// regardless of how the underlying error was wrapped.
func TestWriteJSONError_StripsTrakrfModulePathFromDetail(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets", nil)

	detail := "failed to query github.com/trakrf/platform/backend/internal/storage.ListAssets: connection refused"
	httputil.WriteJSONError(w, r, 500, apierrors.ErrInternal, detail, "req-1")

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotContains(t, resp.Error.Detail, "github.com/", "module path must be scrubbed from detail")
	assert.NotContains(t, resp.Error.Detail, "trakrf/platform", "internal package structure must not leak")
	assert.Contains(t, resp.Error.Detail, "connection refused", "underlying cause must remain visible")
}

func TestWriteJSONError_StripsThirdPartyModulePathFromDetail(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets", nil)

	detail := "scan failed: github.com/jackc/pgx/v5.errBadConn"
	httputil.WriteJSONError(w, r, 500, apierrors.ErrInternal, detail, "req-1")

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotContains(t, resp.Error.Detail, "github.com/", "third-party module paths also scrubbed")
}

func TestWriteJSONError_StripsNonGithubModulePathFromDetail(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets", nil)

	// Sanitizer matches host/path shape, not a literal hostname, so vanity
	// imports and other forges are scrubbed too.
	detail := "scan failed: golang.org/x/sync/singleflight.Group: deadlock"
	httputil.WriteJSONError(w, r, 500, apierrors.ErrInternal, detail, "req-1")

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotContains(t, resp.Error.Detail, "golang.org/", "vanity-host module path scrubbed")
	assert.Contains(t, resp.Error.Detail, "deadlock", "underlying cause remains visible")
}

func TestWriteJSONError_LeavesPlainDetailUntouched(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets", nil)

	detail := "asset FORK-007 not found"
	httputil.WriteJSONError(w, r, 404, apierrors.ErrNotFound, detail, "req-1")

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "asset FORK-007 not found", resp.Error.Detail)
}

func TestWriteJSONErrorWithFields_StripsModulePathFromDetail(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/assets", nil)

	detail := "validation failed: github.com/trakrf/platform/backend/internal/models/asset.Validate: bad input"
	httputil.WriteJSONErrorWithFields(w, r, 422, apierrors.ErrValidation, detail, "req-1", nil)

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotContains(t, resp.Error.Detail, "github.com/")
	assert.Contains(t, strings.ToLower(resp.Error.Detail), "bad input")
}
