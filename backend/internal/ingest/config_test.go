package ingest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigEnabled(t *testing.T) {
	assert.False(t, Config{URL: ""}.Enabled())
	assert.True(t, Config{URL: "mqtts://x"}.Enabled())
}

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("MQTT_URL", "mqtts://u:p@host:8883")
	t.Setenv("MQTT_TOPIC", "")
	t.Setenv("MQTT_CLIENT_ID", "")
	c := ConfigFromEnv()
	assert.Equal(t, "mqtts://u:p@host:8883", c.URL)
	assert.Equal(t, "trakrf.id/#", c.Topic)
	assert.Equal(t, "trakrf-subscriber", c.ClientID)
}

func TestConfigFromEnvOverrides(t *testing.T) {
	t.Setenv("MQTT_URL", "mqtts://u:p@host:8883")
	t.Setenv("MQTT_TOPIC", "$share/grp/trakrf.id/#")
	t.Setenv("MQTT_CLIENT_ID", "custom-id")
	c := ConfigFromEnv()
	assert.Equal(t, "$share/grp/trakrf.id/#", c.Topic)
	assert.Equal(t, "custom-id", c.ClientID)
}
