package shared

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TRA-717 / BB34 F3 rework: outbound public-surface timestamps emit
// RFC 3339 with fixed three-digit millisecond fractional precision so
// the wire shape is uniform across the surface (no Go-stdlib trailing-
// zero trimming) and stable for hand-rolled regex parsers in
// generated-SDK consumers.
func TestPublicTime_MarshalJSON_MillisecondPadding(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "whole-second time pads to .000Z",
			in:   time.Date(2026, 5, 14, 12, 34, 56, 0, time.UTC),
			want: `"2026-05-14T12:34:56.000Z"`,
		},
		{
			name: "millisecond-precision input round-trips",
			in:   time.Date(2026, 5, 14, 12, 34, 56, 123000000, time.UTC),
			want: `"2026-05-14T12:34:56.123Z"`,
		},
		{
			name: "microsecond input truncates to milliseconds",
			in:   time.Date(2026, 5, 14, 12, 34, 56, 752440000, time.UTC),
			want: `"2026-05-14T12:34:56.752Z"`,
		},
		{
			name: "non-UTC zone is converted to UTC",
			in:   time.Date(2026, 5, 14, 7, 34, 56, 500000000, time.FixedZone("EST", -5*3600)),
			want: `"2026-05-14T12:34:56.500Z"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(NewPublicTime(tt.in))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

// Non-nullable public fields (PublicLocationView.updated_at) carry a
// non-pointer PublicTime; a zero value must serialize as a string, not
// null, so the response shape stays "always-a-string" per TRA-649 / BB23 S2.
func TestPublicTime_MarshalJSON_ZeroEmitsString(t *testing.T) {
	got, err := json.Marshal(PublicTime{})
	require.NoError(t, err)
	assert.Equal(t, `"0001-01-01T00:00:00.000Z"`, string(got))
}

// Nullable public fields use *PublicTime; nil → JSON null via Go's
// default pointer-marshal path (no explicit MarshalJSON on the pointer
// receiver needed).
func TestPublicTime_PointerNilEmitsNull(t *testing.T) {
	type wrap struct {
		At *PublicTime `json:"at"`
	}
	got, err := json.Marshal(wrap{At: nil})
	require.NoError(t, err)
	assert.JSONEq(t, `{"at":null}`, string(got))
}

// PublicTimePtr is the converter helper: nil source → nil pointer, real
// source → wrapped pointer.
func TestPublicTimePtr(t *testing.T) {
	assert.Nil(t, PublicTimePtr(nil))

	when := time.Date(2026, 5, 14, 12, 34, 56, 500000000, time.UTC)
	got := PublicTimePtr(&when)
	require.NotNil(t, got)
	assert.True(t, got.Time.Equal(when))
}

// Inbound parser accepts any RFC 3339 fractional precision so a
// consumer that kept microsecond-precision from an upstream system
// (e.g. the same value they received from this API on a previous call)
// can echo it back as a filter value without parse rejection.
func TestPublicTime_UnmarshalJSON_AcceptsVariablePrecision(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"no fractional", `"2026-05-14T12:34:56Z"`},
		{"millisecond", `"2026-05-14T12:34:56.123Z"`},
		{"microsecond", `"2026-05-14T12:34:56.752440Z"`},
		{"nanosecond", `"2026-05-14T12:34:56.123456789Z"`},
		{"non-UTC offset", `"2026-05-14T07:34:56.500-05:00"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pt PublicTime
			require.NoError(t, json.Unmarshal([]byte(tt.input), &pt))
			assert.False(t, pt.Time.IsZero())
		})
	}
}

// null / empty / absent input must leave the receiver at zero so a
// pointer field treats the absence the same way every other public
// timestamp pointer does.
func TestPublicTime_UnmarshalJSON_NullAndEmptyPreserveZero(t *testing.T) {
	for _, input := range []string{`null`, `""`} {
		var pt PublicTime
		require.NoError(t, json.Unmarshal([]byte(input), &pt))
		assert.True(t, pt.Time.IsZero(), "input %q must leave zero value", input)
	}
}

// Garbage input surfaces as *json.UnmarshalTypeError keyed on time.Time
// so httputil.RespondDecodeError routes it to the standard
// validation_error envelope (TRA-641 / BB21 §2.1 contract).
func TestPublicTime_UnmarshalJSON_RejectsGarbage(t *testing.T) {
	var pt PublicTime
	err := json.Unmarshal([]byte(`"not-a-date"`), &pt)
	require.Error(t, err)
	var typeErr *json.UnmarshalTypeError
	require.ErrorAs(t, err, &typeErr)
	assert.Equal(t, "time.Time", typeErr.Type.String())
}

// Round-trip across the marshal+unmarshal pair preserves millisecond
// precision (microsecond input truncates to millisecond on the way out
// and stays there on the way back in).
func TestPublicTime_RoundTrip_MillisecondPrecision(t *testing.T) {
	in := NewPublicTime(time.Date(2026, 5, 14, 12, 34, 56, 752440000, time.UTC))
	b, err := json.Marshal(in)
	require.NoError(t, err)

	var out PublicTime
	require.NoError(t, json.Unmarshal(b, &out))

	// Output side truncated 752440000ns → 752000000ns (.752Z).
	wantMillis := time.Date(2026, 5, 14, 12, 34, 56, 752000000, time.UTC)
	assert.True(t, out.Time.Equal(wantMillis), "round-trip got %s want %s", out.Time, wantMillis)
}
