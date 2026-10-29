package notificationrecipients

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/phone"
)

func strPtr(s string) *string { return &s }

func TestNormalizePhone_RewritesToE164(t *testing.T) {
	for _, raw := range []string{"(555) 123-4567", "555.123.4567", "+1 555 123 4567", "+15551234567"} {
		p := strPtr(raw)
		require.NoError(t, normalizePhone(p, "US"), raw)
		require.Equal(t, "+15551234567", *p, raw)
	}
}

func TestNormalizePhone_NilIsUntouched(t *testing.T) {
	require.NoError(t, normalizePhone(nil, "US"))
}

func TestNormalizePhone_RejectsNonNumbers(t *testing.T) {
	for _, raw := range []string{" ", "call me", "123"} {
		p := strPtr(raw)
		require.ErrorIs(t, normalizePhone(p, "US"), phone.ErrInvalidNumber, raw)
		require.Equal(t, raw, *p, "rejected input is left as submitted")
	}
}
