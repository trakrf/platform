package httputil_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TRA-618 §S4: surface out-of-range path-param failures as
// validation_error + fields[] (matching limit-too-large), not bad_request.

func TestParseSurrogateID_ValidValues(t *testing.T) {
	// TRA-657 / BB25 B3: path-param max widened to SurrogateIDMax (2^53-1)
	// so generated SDKs that don't enforce path-param maximum get 404
	// not_found (handler returns no row when the int4 surrogate column
	// can't hold the value) instead of 400 validation_error. In-range
	// values that exceed int4 still parse cleanly here; the not-found
	// surfacing is the handler's responsibility.
	cases := []struct {
		raw  string
		want int
	}{
		{"1", 1},
		{"42", 42},
		{strconv.Itoa(math.MaxInt32), math.MaxInt32},
		{strconv.FormatInt(int64(math.MaxInt32)+1, 10), int(math.MaxInt32) + 1},
		{"999999999999", 999999999999},
		{strconv.FormatInt(httputil.SurrogateIDMax, 10), int(httputil.SurrogateIDMax)},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			n, err := httputil.ParseSurrogateID("asset_id", tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.want, n)
		})
	}
}

func TestParseSurrogateID_FieldParamErrors(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		field     string
		wantCode  string
		wantParam string // "min" or "max" or "" (none)
	}{
		{"non-numeric", "abc", "asset_id", "invalid_value", ""},
		{"empty", "", "asset_id", "invalid_value", ""},
		{"zero below min", "0", "asset_id", "too_small", "min"},
		{"negative below min", "-1", "asset_id", "too_small", "min"},
		{"one above SurrogateIDMax", strconv.FormatInt(httputil.SurrogateIDMax+1, 10), "asset_id", "too_large", "max"},
		// strconv.ParseInt(_, 10, 64) returns ErrRange for values that
		// don't fit in int64; surfaces as invalid_value via the parse path.
		{"above int64 max", "9999999999999999999999", "asset_id", "invalid_value", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := httputil.ParseSurrogateID(tc.field, tc.raw)
			require.Error(t, err)

			var fpe *httputil.FieldParamError
			require.True(t, errors.As(err, &fpe), "expected *FieldParamError, got %T", err)
			assert.Equal(t, tc.field, fpe.Field)
			assert.Equal(t, tc.wantCode, fpe.Code)
			if tc.wantParam != "" {
				_, ok := fpe.Params[tc.wantParam]
				assert.True(t, ok, "expected params[%q] populated", tc.wantParam)
			}
		})
	}
}

func TestRespondPathParamError_ValidationEnvelopeWithFields(t *testing.T) {
	// Build a too-small failure to exercise the validation_error envelope —
	// after TRA-657 widening, out-of-int32 values no longer trip too_large
	// (they pass through and become 404 from the handler).
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets/0", nil)

	_, err := httputil.ParseSurrogateID("asset_id", "0")
	require.Error(t, err)
	httputil.RespondPathParamError(w, r, err, "req-1")

	assert.Equal(t, 400, w.Code)

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, string(apierrors.ErrValidation), resp.Error.Type)
	assert.Equal(t, 400, resp.Error.Status)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "asset_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "too_small", resp.Error.Fields[0].Code)
	assert.Equal(t, float64(1), resp.Error.Fields[0].Params["min"])
}

func TestRespondPathParamError_NonNumericSurfacesAsInvalidValue(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets/abc", nil)

	_, err := httputil.ParseSurrogateID("asset_id", "abc")
	require.Error(t, err)
	httputil.RespondPathParamError(w, r, err, "req-2")

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, string(apierrors.ErrValidation), resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "asset_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}

func TestRespondPathParamError_FallsBackToBadRequestForUntypedError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets/x", nil)

	httputil.RespondPathParamError(w, r, errors.New("some other error"), "req-3")

	assert.Equal(t, 400, w.Code)

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, string(apierrors.ErrBadRequest), resp.Error.Type)
	assert.Empty(t, resp.Error.Fields)
}

func TestParsePathInt_BoundsAreInclusive(t *testing.T) {
	// At-min and at-max accept; just-below-min and just-above-max reject.
	got, err := httputil.ParsePathInt("page", "1", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, got)

	got, err = httputil.ParsePathInt("page", "10", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 10, got)

	_, err = httputil.ParsePathInt("page", "0", 1, 10)
	var fpe *httputil.FieldParamError
	require.True(t, errors.As(err, &fpe))
	assert.Equal(t, "too_small", fpe.Code)

	_, err = httputil.ParsePathInt("page", "11", 1, 10)
	require.True(t, errors.As(err, &fpe))
	assert.Equal(t, "too_large", fpe.Code)
}

// Sanity: Error() returns the human message so callers that fall through
// the type assertion still log a useful string.
func TestFieldParamError_ErrorString(t *testing.T) {
	_, err := httputil.ParseSurrogateID("asset_id", "0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "asset_id")
	// Nil receiver does not panic.
	var nilErr *httputil.FieldParamError
	_ = fmt.Sprint(nilErr.Error())
}
