package ingest

import "os"

// Config controls the MQTT subscriber. An empty URL disables it entirely
// (keeps local dev, tests, and pre-cutover prod inert).
type Config struct {
	URL      string // mqtts://user:pass@host:port  (MQTT_URL)
	Topic    string // subscription filter (MQTT_TOPIC), e.g. trakrf.id/# or $share/grp/trakrf.id/#
	ClientID string // base client id (MQTT_CLIENT_ID); subscriber appends a per-process suffix
}

// Enabled reports whether the subscriber should start.
func (c Config) Enabled() bool { return c.URL != "" }

// ConfigFromEnv reads the MQTT subscriber config from the environment, applying
// defaults for the topic filter and client id.
func ConfigFromEnv() Config {
	c := Config{
		URL:      os.Getenv("MQTT_URL"),
		Topic:    os.Getenv("MQTT_TOPIC"),
		ClientID: os.Getenv("MQTT_CLIENT_ID"),
	}
	if c.Topic == "" {
		c.Topic = "trakrf.id/#"
	}
	if c.ClientID == "" {
		c.ClientID = "trakrf-subscriber"
	}
	return c
}
