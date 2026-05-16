package httputil_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TRA-739 (BB42 F8): ParseListParams emits code=unknown_field on an
// unknown filter key — matching the body-side strict-decode analogue
// (POST {"bogus":1} → unknown_field) and the BB32 changelog claim. The
// asymmetry where query keys returned invalid_value while body keys
// returned unknown_field surfaced on the BB42 retest; generated clients
// branching on unknown_field per the changelog had to special-case the
// query side.
func TestParseListParams_UnknownKey_EmitsUnknownField(t *testing.T) {
	req := httptest.NewRequest("GET", "/?bogus=1", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{Filters: []string{"name"}})
	require.Error(t, err)

	var lpe *httputil.ListParamError
	require.True(t, errors.As(err, &lpe))
	require.Len(t, lpe.Fields, 1)
	assert.Equal(t, "bogus", lpe.Fields[0].Field)
	assert.Equal(t, "unknown_field", lpe.Fields[0].Code,
		"unknown filter key is a field-shaped failure — same code as body-side strict decode")
	assert.Contains(t, lpe.Fields[0].Message, "unknown parameter: bogus")
}
