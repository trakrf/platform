// Package readerd runs the on-reader MQTT JSON-RPC control daemon as a
// long-running subcommand. It auto-discovers its broker config from the
// reader's own EmbeddedGlassFish CloudServer (see internal/readerd.LoadConfig),
// builds a CS463 HTTP client pointed at the reader's localhost servlet API, and
// serves the neutral readerrpc contract until the context is cancelled.
//
// This daemon runs ON the reader, so READER_API_URL defaults to http://localhost.
package readerd

import (
	"context"
	"os"

	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/readerd"
	"github.com/trakrf/platform/backend/internal/readerd/cs463"
)

// Run loads the daemon config, wires up the CS463 adapter over the reader's
// localhost HTTP API, starts the MQTT-RPC daemon, and blocks until ctx is
// cancelled (SIGINT/SIGTERM). It returns nil on a clean shutdown.
func Run(ctx context.Context, info buildinfo.Info) error {
	log := logger.Get()

	cfg, err := readerd.LoadConfig()
	if err != nil {
		return err
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

	d := readerd.New(cfg, adapter, log)
	if err := d.Start(); err != nil {
		return err
	}
	defer d.Stop()

	log.Info().
		Str("version", info.Version).
		Str("base_topic", cfg.Broker.BaseTopic).
		Str("rpc_client_id", cfg.Broker.RPCClientID).
		Str("reader_url", baseURL).
		Msg("readerd started")

	<-ctx.Done()
	log.Info().Msg("readerd shutting down")
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
