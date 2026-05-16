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

// TRA-673 / BB27 F1: 5xx responses must collapse to a fixed generic detail.
// pgx and other DB-driver internals previously leaked via raw err.Error()
// pass-through. The full cause stays in server-side logs for debugging.
func TestWriteJSONError_5xxReplacesDetailWithGenericMessage(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets", nil)

	detail := "failed to query github.com/trakrf/platform/backend/internal/storage.ListAssets: connection refused"
	httputil.WriteJSONError(w, r, 500, apierrors.ErrInternal, detail, "req-1")

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "An unexpected error occurred", resp.Error.Detail)
	assert.NotContains(t, resp.Error.Detail, "connection refused", "underlying cause must not reach client on 5xx")
	assert.NotContains(t, resp.Error.Detail, "github.com/", "internal package structure must not leak")
}

func TestWriteJSONError_5xxScrubsPgxDriverString(t *testing.T) {
	// TRA-668 reproducer: pgx int4-encoding failure string must not reach
	// the client. OID and column-type fingerprinting are information
	// disclosure that fails security review.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets/2147483648", nil)

	detail := "failed to encode args[0]: unable to encode 2147483648 into binary format for int4 (OID 23): 2147483648 is greater than maximum value for int4"
	httputil.WriteJSONError(w, r, 500, apierrors.ErrInternal, detail, "req-pgx")

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "An unexpected error occurred", resp.Error.Detail)
	assert.NotContains(t, resp.Error.Detail, "int4")
	assert.NotContains(t, resp.Error.Detail, "OID")
	assert.NotContains(t, resp.Error.Detail, "encode")
	assert.Equal(t, "req-pgx", resp.Error.RequestID, "request_id stays for log correlation")
}

func TestWriteJSONError_5xxKeepsEnvelopeShape(t *testing.T) {
	// Replacing detail must not strip the rest of the RFC 7807 envelope —
	// type/title/status/instance/request_id must all still render.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets", nil)

	httputil.WriteJSONError(w, r, 503, apierrors.ErrInternal, "upstream timeout", "req-503")

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, string(apierrors.ErrInternal), resp.Error.Type)
	assert.Equal(t, 503, resp.Error.Status)
	assert.Equal(t, "/api/v1/assets", resp.Error.Instance)
	assert.Equal(t, "req-503", resp.Error.RequestID)
	assert.Equal(t, "An unexpected error occurred", resp.Error.Detail)
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

// TRA-739 (BB42 F1): the module-path scrubber must not collapse legitimate
// https://docs.trakrf.id/... URLs that error messages cite for guidance.
// The TRA-734 read-only message landed correctly in fields[].message but
// arrived at the top-level error.detail as "https://[internal]" because
// the previous regex flagged any host/path shape.
func TestWriteJSONErrorWithFields_PreservesHttpsURLInDetail(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/assets", nil)

	detail := "See https://docs.trakrf.id/api/data-model for the full data model."
	httputil.WriteJSONErrorWithFields(w, r, 400, apierrors.ErrValidation, detail, "req-1", nil)

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, detail, resp.Error.Detail)
	assert.NotContains(t, resp.Error.Detail, "[internal]")
}

// Mixed input: a legitimate URL and a bare module path in the same string —
// the URL is preserved and the bare path collapses to [internal].
func TestWriteJSONErrorWithFields_MixedURLAndModulePath(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/assets", nil)

	detail := "wrapped: github.com/trakrf/platform/backend/internal/foo.Bar: see https://docs.trakrf.id/api/data-model"
	httputil.WriteJSONErrorWithFields(w, r, 400, apierrors.ErrValidation, detail, "req-1", nil)

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotContains(t, resp.Error.Detail, "github.com/")
	assert.Contains(t, resp.Error.Detail, "[internal]")
	assert.Contains(t, resp.Error.Detail, "https://docs.trakrf.id/api/data-model")
}
