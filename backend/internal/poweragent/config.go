package poweragent

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReaderConfig describes one reader the agent controls.
type ReaderConfig struct {
	PublishTopic string `json:"publish_topic"` // the reader key; command/state topics derive from it
	BaseURL      string `json:"base_url"`      // e.g. http://192.168.50.212
	Username     string `json:"username"`
	Password     string `json:"password"`
}

// Config is the agent's full configuration.
type Config struct {
	BrokerURL string         `json:"broker_url"` // mqtt[s]://user:pass@host:port (defaults to MQTT_URL)
	ClientID  string         `json:"client_id"`  // defaults to "trakrf-poweragent"
	Readers   []ReaderConfig `json:"readers"`
}

// LoadConfig builds the agent config. Precedence:
//  1. JSON file at READER_AGENT_CONFIG (a {"readers":[…]} document), else
//  2. a single reader from READER_PUBLISH_TOPIC / READER_ADDR / READER_USER /
//     READER_PASS env vars (handy for local dev and single-reader edge boxes).
//
// BrokerURL falls back to MQTT_URL and ClientID to MQTT_CLIENT_ID/default so the
// agent reuses the same broker config as the rest of the stack.
func LoadConfig() (Config, error) {
	var c Config
	if path := os.Getenv("READER_AGENT_CONFIG"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("poweragent: read config %s: %w", path, err)
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return Config{}, fmt.Errorf("poweragent: parse config %s: %w", path, err)
		}
	} else if pt := os.Getenv("READER_PUBLISH_TOPIC"); pt != "" {
		c.Readers = []ReaderConfig{{
			PublishTopic: pt,
			BaseURL:      firstNonEmpty(os.Getenv("READER_ADDR"), os.Getenv("READER_URL")),
			Username:     firstNonEmpty(os.Getenv("READER_USER"), "root"),
			Password:     os.Getenv("READER_PASS"),
		}}
	}

	if c.BrokerURL == "" {
		c.BrokerURL = os.Getenv("MQTT_URL")
	}
	if c.ClientID == "" {
		c.ClientID = firstNonEmpty(os.Getenv("MQTT_CLIENT_ID"), "trakrf-poweragent")
	}

	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) validate() error {
	if c.BrokerURL == "" {
		return fmt.Errorf("poweragent: broker URL is required (set MQTT_URL or broker_url)")
	}
	if len(c.Readers) == 0 {
		return fmt.Errorf("poweragent: at least one reader is required")
	}
	for i, r := range c.Readers {
		if r.PublishTopic == "" {
			return fmt.Errorf("poweragent: reader[%d] missing publish_topic", i)
		}
		if r.BaseURL == "" {
			return fmt.Errorf("poweragent: reader[%d] (%s) missing base_url", i, r.PublishTopic)
		}
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
