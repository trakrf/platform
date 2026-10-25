package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFromEnv(t *testing.T) {
	t.Run("unset defaults to US", func(t *testing.T) {
		t.Setenv("NOTIFICATION_DEFAULT_PHONE_REGION", "")
		cfg, err := ConfigFromEnv()
		require.NoError(t, err)
		assert.Equal(t, "US", cfg.PhoneRegion)
	})
	t.Run("lower case is upper-cased", func(t *testing.T) {
		t.Setenv("NOTIFICATION_DEFAULT_PHONE_REGION", "ug")
		cfg, err := ConfigFromEnv()
		require.NoError(t, err)
		assert.Equal(t, "UG", cfg.PhoneRegion)
	})
	t.Run("unknown region is an error naming the variable", func(t *testing.T) {
		t.Setenv("NOTIFICATION_DEFAULT_PHONE_REGION", "ZZ")
		_, err := ConfigFromEnv()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NOTIFICATION_DEFAULT_PHONE_REGION")
	})
}
