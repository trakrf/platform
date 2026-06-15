// Command mqtt-rpcd is the standalone on-reader MQTT JSON-RPC control daemon.
// It runs ON a CSL CS463 reader: it auto-discovers its broker config from the
// reader's own EmbeddedGlassFish CloudServer (see internal/readerd.LoadConfig),
// builds a CS463 HTTP client pointed at the reader's localhost servlet API, and
// serves the neutral readerrpc contract until SIGINT/SIGTERM.
//
// Because it runs on the reader, READER_API_URL defaults to http://localhost.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerd"
	"github.com/trakrf/platform/mqtt-rpc/internal/readerd/cs463"
)

// version is overridable via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := zerolog.New(os.Stderr).With().
		Timestamp().
		Str("service", "mqtt-rpcd").
		Str("version", version).
		Logger()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := readerd.LoadConfig()
	if err != nil {
		logger.Fatal().Err(err).Msg("load config")
	}

	// The daemon runs on the reader and talks to the reader's localhost servlet
	// API. URL/user/pass are overridable via env; pass is the only secret.
	baseURL := envOr("READER_API_URL", "http://localhost")
	user := envOr("READER_API_USER", "root")
	pass := os.Getenv("READER_API_PASS")
	client := cs463.New(baseURL, user, pass, 0)

	adapter := cs463.NewAdapter(client, cs463.AdapterConfig{
		AntennaCount: cfg.AntennaCount,
		EventID:      cfg.EventID,
	})

	d := readerd.New(cfg, adapter, &logger)
	if err := d.Start(); err != nil {
		logger.Fatal().Err(err).Msg("start daemon")
	}
	defer d.Stop()

	logger.Info().
		Str("base_topic", cfg.Broker.BaseTopic).
		Str("rpc_client_id", cfg.Broker.RPCClientID).
		Str("reader_url", baseURL).
		Msg("mqtt-rpcd running")

	<-ctx.Done()
	logger.Info().Msg("mqtt-rpcd shutting down")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
