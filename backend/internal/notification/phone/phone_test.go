package phone

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeE164(t *testing.T) {
	cases := []struct{ raw, region, want string }{
		{"(555) 123-4567", "US", "+15551234567"},
		{"555.123.4567", "US", "+15551234567"},
		{"+1 555 123 4567", "US", "+15551234567"},
		{"+44 20 7946 0958", "US", "+442079460958"}, // own country code kept
		{"0772 123456", "UG", "+256772123456"},      // trunk prefix, non-US region
		{"5551234567", "us", "+15551234567"},        // region case-insensitive
	}
	for _, c := range cases {
		got, err := NormalizeE164(c.raw, c.region)
		require.NoError(t, err, c.raw)
		require.Equal(t, c.want, got, c.raw)
	}
}

func TestNormalizeE164_Rejects(t *testing.T) {
	for _, raw := range []string{"", "call me", "123"} {
		_, err := NormalizeE164(raw, "US")
		require.ErrorIs(t, err, ErrInvalidNumber, raw)
		if raw != "" {
			require.NotContains(t, err.Error(), raw) // PII stays out of errors
		}
	}
	_, err := NormalizeE164("5551234567", "ZZ")
	require.ErrorIs(t, err, ErrUnknownRegion)
}

func TestValidateRegion(t *testing.T) {
	require.NoError(t, ValidateRegion("US"))
	require.NoError(t, ValidateRegion("ug"))
	require.ErrorIs(t, ValidateRegion("ZZ"), ErrUnknownRegion)
	require.ErrorIs(t, ValidateRegion(""), ErrUnknownRegion)
}
