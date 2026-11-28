package arbitration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfigIsBLEOnly(t *testing.T) {
	c := DefaultConfig()
	assert.True(t, c.Enabled("gl_s10"))
	assert.True(t, c.Enabled("moko_mk107"))
	assert.True(t, c.Enabled("esp32_ble_generic"))
	assert.False(t, c.Enabled("csl_cs463"), "fixed RFID readers are not arbitrated")
	assert.False(t, c.Enabled("csl_cs108"))
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("ARBITRATION_WINDOW", "8s")
	t.Setenv("ARBITRATION_DWELL", "3s")
	t.Setenv("ARBITRATION_MARGIN_DB", "0")
	t.Setenv("ARBITRATION_SILENCE", "2m")
	t.Setenv("ARBITRATION_DEVICE_TYPES", " gl_s10 , csl_cs463 ")
	c := ConfigFromEnv()
	assert.Equal(t, 8*time.Second, c.Window)
	assert.Equal(t, 3*time.Second, c.Dwell)
	assert.Equal(t, 0, c.MarginDB)
	assert.Equal(t, 2*time.Minute, c.Silence)
	assert.True(t, c.Enabled("gl_s10"))
	assert.True(t, c.Enabled("csl_cs463"))
	assert.False(t, c.Enabled("moko_mk107"))
}

func TestConfigFromEnvRejectsBadValues(t *testing.T) {
	t.Setenv("ARBITRATION_WINDOW", "-1s")
	t.Setenv("ARBITRATION_DWELL", "soon")
	t.Setenv("ARBITRATION_MARGIN_DB", "-3")
	assert.Equal(t, DefaultConfig().Window, ConfigFromEnv().Window)
	assert.Equal(t, DefaultConfig().Dwell, ConfigFromEnv().Dwell)
	assert.Equal(t, DefaultConfig().MarginDB, ConfigFromEnv().MarginDB)
}

func TestConfigFromEnvReturnRule(t *testing.T) {
	assert.Equal(t, 5*time.Minute, DefaultConfig().ReturnWindow)
	assert.Equal(t, 30*time.Second, DefaultConfig().ReturnDwell)

	t.Setenv("ARBITRATION_RETURN_WINDOW", "15m")
	t.Setenv("ARBITRATION_RETURN_DWELL", "1m")
	assert.Equal(t, 15*time.Minute, ConfigFromEnv().ReturnWindow)
	assert.Equal(t, time.Minute, ConfigFromEnv().ReturnDwell)

	t.Setenv("ARBITRATION_RETURN_WINDOW", "0s")
	assert.Equal(t, time.Duration(0), ConfigFromEnv().ReturnWindow, "0 turns the return rule off")

	t.Setenv("ARBITRATION_RETURN_WINDOW", "-1m")
	t.Setenv("ARBITRATION_RETURN_DWELL", "0s")
	assert.Equal(t, DefaultConfig().ReturnWindow, ConfigFromEnv().ReturnWindow)
	assert.Equal(t, DefaultConfig().ReturnDwell, ConfigFromEnv().ReturnDwell)
}

func TestConfigFromEnvNoneDisables(t *testing.T) {
	t.Setenv("ARBITRATION_DEVICE_TYPES", "none")
	assert.False(t, ConfigFromEnv().Enabled("gl_s10"))
}
