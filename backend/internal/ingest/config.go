package ingest

import "os"

// Config controls the MQTT subscriber. An empty URL disables it entirely
// (keeps local dev, tests, and pre-cutover prod inert).
type Config struct {
	URL      string // mqtts://user:pass@host:port  (MQTT_URL)
	ClientID string // base client id (MQTT_CLIENT_ID); subscriber appends a per-process suffix
}

// Enabled reports whether the subscriber should start.
func (c Config) Enabled() bool { return c.URL != "" }

// ConfigFromEnv reads the MQTT subscriber config from the environment.
//
// TRA-922: MQTT_TOPIC is retired. The subscriber no longer uses a static
// subscription filter — it subscribes to exactly the registered publish_topics
// (data-driven, via the topicroute registry), so there is no topic to configure.
func ConfigFromEnv() Config {
	c := Config{
		URL:      os.Getenv("MQTT_URL"),
		ClientID: os.Getenv("MQTT_CLIENT_ID"),
	}
	if c.ClientID == "" {
		c.ClientID = "trakrf-subscriber"
	}
	return c
}
