package httputil_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TRA-811 / BB71 F1: boolean query params accept only exact lowercase
// `true` / `false`. Mixed case (True, TRUE, tRuE, ...) is rejected with the
// same `invalid_value` envelope as `yes` / `no` / `1` / `0`. The docs and the
// runtime rejection message both say lowercase; the parser now honors that.
func TestParseListParams_BoolCaseSensitive(t *testing.T) {
	allow := httputil.ListAllowlist{
		Filters:     []string{"is_active", "include_deleted"},
		BoolFilters: []string{"is_active", "include_deleted"},
	}

	t.Run("accepts exact lowercase true", func(t *testing.T) {
		for _, key := range []string{"is_active", "include_deleted"} {
			req := httptest.NewRequest("GET", "/?"+key+"=true", nil)
			p, err := httputil.ParseListParams(req, allow)
			require.NoErrorf(t, err, "key=%q", key)
			assert.Equalf(t, []string{"true"}, p.Filters[key], "key=%q", key)
		}
	})

	t.Run("accepts exact lowercase false", func(t *testing.T) {
		for _, key := range []string{"is_active", "include_deleted"} {
			req := httptest.NewRequest("GET", "/?"+key+"=false", nil)
			p, err := httputil.ParseListParams(req, allow)
			require.NoErrorf(t, err, "key=%q", key)
			assert.Equalf(t, []string{"false"}, p.Filters[key], "key=%q", key)
		}
	})

	t.Run("rejects mixed-case true variants with invalid_value", func(t *testing.T) {
		for _, key := range []string{"is_active", "include_deleted"} {
			for _, raw := range []string{"True", "TRUE", "tRuE", "tRUE"} {
				req := httptest.NewRequest("GET", "/?"+key+"="+raw, nil)
				_, err := httputil.ParseListParams(req, allow)
				require.Errorf(t, err, "key=%q raw=%q", key, raw)
				var lpe *httputil.ListParamError
				require.Truef(t, errors.As(err, &lpe), "key=%q raw=%q: not ListParamError", key, raw)
				require.Lenf(t, lpe.Fields, 1, "key=%q raw=%q", key, raw)
				assert.Equalf(t, key, lpe.Fields[0].Field, "key=%q raw=%q", key, raw)
				assert.Equalf(t, "invalid_value", lpe.Fields[0].Code, "key=%q raw=%q", key, raw)
			}
		}
	})

	t.Run("rejects mixed-case false variants with invalid_value", func(t *testing.T) {
		for _, key := range []string{"is_active", "include_deleted"} {
			for _, raw := range []string{"False", "FALSE", "fAlSe", "fALSE"} {
				req := httptest.NewRequest("GET", "/?"+key+"="+raw, nil)
				_, err := httputil.ParseListParams(req, allow)
				require.Errorf(t, err, "key=%q raw=%q", key, raw)
				var lpe *httputil.ListParamError
				require.Truef(t, errors.As(err, &lpe), "key=%q raw=%q: not ListParamError", key, raw)
				require.Lenf(t, lpe.Fields, 1, "key=%q raw=%q", key, raw)
				assert.Equalf(t, key, lpe.Fields[0].Field, "key=%q raw=%q", key, raw)
				assert.Equalf(t, "invalid_value", lpe.Fields[0].Code, "key=%q raw=%q", key, raw)
			}
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
