package httputil_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TRA-645 / BB22 F3: boolean query params accept any case variant of true/false
// and normalize to lowercase. Non-boolean strings (yes, 1, on, ...) still 400.
func TestParseListParams_BoolCaseInsensitive(t *testing.T) {
	allow := httputil.ListAllowlist{
		Filters:     []string{"is_active"},
		BoolFilters: []string{"is_active"},
	}

	t.Run("accepts mixed case true and normalizes", func(t *testing.T) {
		for _, raw := range []string{"true", "True", "TRUE", "tRuE"} {
			req := httptest.NewRequest("GET", "/?is_active="+raw, nil)
			p, err := httputil.ParseListParams(req, allow)
			require.NoErrorf(t, err, "raw=%q", raw)
			assert.Equalf(t, []string{"true"}, p.Filters["is_active"], "raw=%q", raw)
		}
	})

	t.Run("accepts mixed case false and normalizes", func(t *testing.T) {
		for _, raw := range []string{"false", "False", "FALSE", "fAlSe"} {
			req := httptest.NewRequest("GET", "/?is_active="+raw, nil)
			p, err := httputil.ParseListParams(req, allow)
			require.NoErrorf(t, err, "raw=%q", raw)
			assert.Equalf(t, []string{"false"}, p.Filters["is_active"], "raw=%q", raw)
		}
	})

	t.Run("rejects non-boolean strings with invalid_value", func(t *testing.T) {
		for _, raw := range []string{"yes", "no", "0", "1", "on", "off", ""} {
			req := httptest.NewRequest("GET", "/?is_active="+raw, nil)
			_, err := httputil.ParseListParams(req, allow)
			require.Errorf(t, err, "raw=%q", raw)
			var lpe *httputil.ListParamError
			require.Truef(t, errors.As(err, &lpe), "raw=%q: not ListParamError", raw)
			require.Lenf(t, lpe.Fields, 1, "raw=%q", raw)
			assert.Equalf(t, "is_active", lpe.Fields[0].Field, "raw=%q", raw)
			assert.Equalf(t, "invalid_value", lpe.Fields[0].Code, "raw=%q", raw)
		}
	})
}
