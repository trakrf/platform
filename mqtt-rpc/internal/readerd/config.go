package readerd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Default on-reader file locations and identifiers. The reader's own EmbeddedGlassFish
// install owns these; the daemon auto-discovers its broker config from them so it
// shares one source of truth with the CloudServer connection the reader already uses.
const (
	defaultCloudServerFile = "/opt/EmbeddedGlassFish/config/CloudServer"
	defaultEventListFile   = "/opt/EmbeddedGlassFish/config/EventListCS463"
	defaultCertDir         = "/opt/EmbeddedGlassFish/cert/CloudServer"
	// defaultCloudServerID matches cs463.NameMQTTServer — the daemon both reads its
	// broker config from this pre-created entry and references it by name in the
	// golden chain. Existing readers provisioned under the old "TrakRF MQTT" name
	// must set READERD_CLOUDSERVER_ID until migrated.
	defaultCloudServerID = "TrakRF mqtt-rpc MQTT Server"
	// defaultEventID matches cs463.NameEvent — the golden event the startup
	// reconcile creates and arms. Used as the EventID fallback when a from-scratch
	// reader's EventList has no enabled event yet (see LoadConfig).
	defaultEventID      = "TrakRF mqtt-rpc Event"
	defaultAntennaCount = 4
)

// errNoEnabledEvent is returned by parseEnabledEvent when the EventList parses but
// contains no enabled event. LoadConfig treats this as recoverable (falls back to
// the golden event when reconcile is on) — distinct from a malformed-file error.
var errNoEnabledEvent = errors.New("readerd: no enabled event in EventList")

// BrokerConfig describes how the daemon connects to the MQTT broker for the RPC
// control channel.
type BrokerConfig struct {
	URL         string
	CACertPath  string
	BaseTopic   string
	RPCClientID string
}

// Config is the full daemon configuration.
type Config struct {
	Broker       BrokerConfig
	EventID      string
	AntennaCount int
	// Reconcile, when true (default), runs the golden-config reconcile on startup.
	Reconcile bool
}

// cloudServerEntry mirrors one object in the reader's CloudServer config array.
type cloudServerEntry struct {
	ServerID       string `json:"serverId"`
	Type           string `json:"type"`
	Path           string `json:"path"`
	Port           int    `json:"port"`
	ClientID       string `json:"clientId"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	EnableSSL      bool   `json:"enableSSL"`
	ServerCertFile string `json:"serverCertFile"`
	Topic          string `json:"topic"`
}

// eventEntry mirrors one object in the reader's EventList config array.
type eventEntry struct {
	EventID string `json:"eventId"`
	Enable  bool   `json:"enable"`
}

// parseCloudServer selects the MQTT server identified by serverID from the reader's
// CloudServer config JSON and derives the broker connection from it. certDir is the
// base directory under which per-server CA certs live.
func parseCloudServer(data []byte, serverID, certDir string) (BrokerConfig, error) {
	var entries []cloudServerEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return BrokerConfig{}, fmt.Errorf("readerd: parse CloudServer: %w", err)
	}

	for _, e := range entries {
		if e.ServerID != serverID || e.Type != "MQTT" {
			continue
		}

		scheme := "mqtt"
		// CA cert only applies to a TLS broker that names an explicit cert file.
		// A plaintext broker (enableSSL=false — e.g. the demo-box local Mosquitto)
		// has no TLS, and a TLS broker with no serverCertFile uses the system root
		// pool; in both cases CACertPath stays empty so the daemon does not try to
		// load a (bogus) cert path and fatal at startup.
		caCertPath := ""
		if e.EnableSSL {
			scheme = "mqtts"
			if e.ServerCertFile != "" {
				caCertPath = filepath.Join(certDir, e.ServerID, e.ServerCertFile)
			}
		}
		u := &url.URL{
			Scheme: scheme,
			User:   url.UserPassword(e.Username, e.Password),
			Host:   e.Path + ":" + strconv.Itoa(e.Port),
		}

		baseTopic := strings.TrimSuffix(e.Topic, "/reads")

		return BrokerConfig{
			URL:         u.String(),
			CACertPath:  caCertPath,
			BaseTopic:   baseTopic,
			RPCClientID: e.ClientID + "-rpc",
		}, nil
	}

	return BrokerConfig{}, fmt.Errorf("readerd: no MQTT CloudServer entry with serverId %q", serverID)
}

// parseEnabledEvent returns the eventId of the single enabled event in the reader's
// EventList config JSON.
func parseEnabledEvent(data []byte) (string, error) {
	var entries []eventEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return "", fmt.Errorf("readerd: parse EventList: %w", err)
	}
	for _, e := range entries {
		if e.Enable {
			return e.EventID, nil
		}
	}
	return "", errNoEnabledEvent
}

// LoadConfig builds the daemon Config from environment variables, falling back to
// auto-discovery from the reader's own EmbeddedGlassFish config files when the
// broker is not specified directly.
func LoadConfig() (Config, error) {
	var cfg Config

	// Broker: explicit env wins; else auto-discover from the reader's CloudServer.
	if directURL := os.Getenv("READERD_BROKER_URL"); directURL != "" {
		cfg.Broker = BrokerConfig{
			URL:         directURL,
			CACertPath:  os.Getenv("READERD_CA_CERT"),
			BaseTopic:   os.Getenv("READERD_BASE_TOPIC"),
			RPCClientID: os.Getenv("READERD_RPC_CLIENT_ID"),
		}
	} else {
		csFile := envOr("READERD_CLOUDSERVER_FILE", defaultCloudServerFile)
		serverID := envOr("READERD_CLOUDSERVER_ID", defaultCloudServerID)
		certDir := envOr("READERD_CERT_DIR", defaultCertDir)

		data, err := os.ReadFile(csFile)
		if err != nil {
			return Config{}, fmt.Errorf("readerd: read CloudServer file: %w", err)
		}
		bc, err := parseCloudServer(data, serverID, certDir)
		if err != nil {
			return Config{}, err
		}
		cfg.Broker = bc
	}

	// Reconcile golden config on startup (default on). Determined before EventID
	// because the EventID fallback depends on whether reconcile will run.
	cfg.Reconcile = true
	if v := os.Getenv("READERD_RECONCILE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("readerd: invalid READERD_RECONCILE %q: %w", v, err)
		}
		cfg.Reconcile = b
	}

	// EventID: explicit env wins; else the enabled event in the EventList. On a
	// from-scratch reader (all events deleted) the EventList has no enabled event
	// yet, but the startup reconcile is about to create+arm the golden event — so
	// fall back to its name rather than fataling before reconcile can run. Without
	// reconcile nothing creates the event, so the missing-event error still stands;
	// a malformed EventList is always fatal (never silently papered over).
	if ev := os.Getenv("READERD_EVENT_ID"); ev != "" {
		cfg.EventID = ev
	} else {
		evFile := envOr("READERD_EVENTLIST_FILE", defaultEventListFile)
		data, err := os.ReadFile(evFile)
		if err != nil {
			return Config{}, fmt.Errorf("readerd: read EventList file: %w", err)
		}
		ev, err := parseEnabledEvent(data)
		switch {
		case errors.Is(err, errNoEnabledEvent) && cfg.Reconcile:
			ev = defaultEventID
		case err != nil:
			return Config{}, err
		}
		cfg.EventID = ev
	}

	// AntennaCount.
	cfg.AntennaCount = defaultAntennaCount
	if v := os.Getenv("READERD_ANTENNA_COUNT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("readerd: invalid READERD_ANTENNA_COUNT %q: %w", v, err)
		}
		cfg.AntennaCount = n
	}

	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
