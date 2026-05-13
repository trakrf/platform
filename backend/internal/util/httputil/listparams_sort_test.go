package httputil_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TRA-693 / BB30 §2.4: endpoints that declare no sort fields must surface
// "sort parameter not supported on this endpoint" rather than the
// field-shaped "unknown sort field: X" message — the latter misled callers
// toward fixing the field name when the parameter is simply not supported.
func TestParseListParams_Sort_NotSupportedOnSubResource(t *testing.T) {
	req := httptest.NewRequest("GET", "/?sort=name", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	require.Error(t, err)

	var lpe *httputil.ListParamError
	require.True(t, errors.As(err, &lpe))
	require.Len(t, lpe.Fields, 1)
	assert.Equal(t, "sort", lpe.Fields[0].Field)
	assert.Equal(t, "invalid_value", lpe.Fields[0].Code)
	assert.Equal(t, "sort parameter not supported on this endpoint", lpe.Fields[0].Message)
}

// Endpoints that DO declare sort fields keep the field-shaped error so
// callers can correct a typo.
func TestParseListParams_Sort_UnknownFieldWhenSortable(t *testing.T) {
	req := httptest.NewRequest("GET", "/?sort=bogus", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{Sorts: []string{"name"}})
	require.Error(t, err)

	var lpe *httputil.ListParamError
	require.True(t, errors.As(err, &lpe))
	require.Len(t, lpe.Fields, 1)
	assert.Equal(t, "sort", lpe.Fields[0].Field)
	assert.Equal(t, "invalid_value", lpe.Fields[0].Code)
	assert.Contains(t, lpe.Fields[0].Message, "unknown sort field")
}

// Empty ?sort= still decodes to "no sorting applied" regardless of allowlist.
func TestParseListParams_Sort_EmptyValueIsNoOp(t *testing.T) {
	req := httptest.NewRequest("GET", "/?sort=", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	require.NoError(t, err)
	assert.Empty(t, p.Sorts)
}
