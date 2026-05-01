package httputil_test

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestParseListParams_Defaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters: []string{"location"},
		Sorts:   []string{"identifier", "name"},
	})
	require.NoError(t, err)
	assert.Equal(t, 50, p.Limit)
	assert.Equal(t, 0, p.Offset)
	assert.Empty(t, p.Filters)
	assert.Empty(t, p.Sorts)
}

func TestParseListParams_LimitCap(t *testing.T) {
	req := httptest.NewRequest("GET", "/?limit=500", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "200")
}

func TestParseListParams_UnknownParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/?mystery=1", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{Filters: []string{"location"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mystery")
}

func TestParseListParams_Filters(t *testing.T) {
	req := httptest.NewRequest("GET", "/?location=wh-1&location=wh-2&is_active=true", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters: []string{"location", "is_active"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"wh-1", "wh-2"}, p.Filters["location"])
	assert.Equal(t, []string{"true"}, p.Filters["is_active"])
}

func TestParseListParams_Sort(t *testing.T) {
	req := httptest.NewRequest("GET", "/?sort=name,-created_at", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Sorts: []string{"name", "created_at"},
	})
	require.NoError(t, err)
	require.Len(t, p.Sorts, 2)
	assert.Equal(t, "name", p.Sorts[0].Field)
	assert.False(t, p.Sorts[0].Desc)
	assert.Equal(t, "created_at", p.Sorts[1].Field)
	assert.True(t, p.Sorts[1].Desc)
}

func TestParseListParams_UnknownSortField(t *testing.T) {
	req := httptest.NewRequest("GET", "/?sort=banana", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{Sorts: []string{"name"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "banana")
}

func TestParseListParams_InvalidOffsetNegative(t *testing.T) {
	req := httptest.NewRequest("GET", "/?offset=-1", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	require.Error(t, err)
}

func TestParseListParams_LimitAndOffsetExempt(t *testing.T) {
	// limit/offset/sort are always allowed without declaration.
	req := httptest.NewRequest("GET", "/?limit=25&offset=5", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	require.NoError(t, err)
	assert.Equal(t, 25, p.Limit)
	assert.Equal(t, 5, p.Offset)
}

func TestParseListParams_BoolFilter_Accepts(t *testing.T) {
	for _, v := range []string{"true", "false"} {
		req := httptest.NewRequest("GET", "/?is_active="+v, nil)
		p, err := httputil.ParseListParams(req, httputil.ListAllowlist{
			Filters:     []string{"is_active"},
			BoolFilters: []string{"is_active"},
		})
		require.NoError(t, err, "value=%q", v)
		assert.Equal(t, []string{v}, p.Filters["is_active"])
	}
}

func TestParseListParams_BoolFilter_Rejects(t *testing.T) {
	for _, v := range []string{"TRUE", "False", "1", "0", "wat", ""} {
		req := httptest.NewRequest("GET", "/?is_active="+v, nil)
		_, err := httputil.ParseListParams(req, httputil.ListAllowlist{
			Filters:     []string{"is_active"},
			BoolFilters: []string{"is_active"},
		})
		require.Error(t, err, "value=%q should be rejected", v)
		assert.Contains(t, err.Error(), "is_active")
		assert.Contains(t, err.Error(), "'true' or 'false'")
	}
}

func TestParseListParams_BoolFilter_NonBoolFilterUnaffected(t *testing.T) {
	// Filters not in BoolFilters accept any string.
	req := httptest.NewRequest("GET", "/?type=whatever", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters: []string{"type"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"whatever"}, p.Filters["type"])
}
