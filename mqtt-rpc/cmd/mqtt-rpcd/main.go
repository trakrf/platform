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
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerd"
	"github.com/trakrf/platform/mqtt-rpc/internal/readerd/cs463"
	"github.com/trakrf/platform/mqtt-rpc/internal/update"
)

// reconcileTimeout bounds the startup golden-config reconcile (a handful of /API
// round-trips against the reader's localhost interface).
const reconcileTimeout = 30 * time.Second

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
	cfg.Version = version

	// Remote self-update. CheckPending runs first, before anything touches the
	// reader: if this process is a fresh update on probation, it commits only
	// after answering an RPC through the broker (see readerd.selfTest) — never on
	// reconcile, which a wedged vendor app would fail.
	restart := make(chan struct{}, 1)
	upd := update.New(update.Layout{Dir: update.DefaultDir}, version, func() {
		select {
		case restart <- struct{}{}:
		default:
		}
	}, logger)
	if err := upd.CheckPending(); err != nil {
		logger.Error().Err(err).Msg("read update state")
	}
	if upd.OnProbation() {
		logger.Warn().Msg("this build is an update on probation; it commits after answering an rpc through the broker, else the guard rolls it back")
	}

	// The daemon runs on the reader and talks to the reader's localhost servlet
	// API. URL/user/pass are overridable via env; pass is the only secret.
	baseURL := envOr("READER_API_URL", "http://localhost")
	user := envOr("READER_API_USER", "trakrf")
	pass := os.Getenv("READER_API_PASS")
	client := cs463.New(baseURL, user, pass, 0)

	adapter := cs463.NewAdapter(client, cs463.AdapterConfig{
		AntennaCount: cfg.AntennaCount,
		EventID:      cfg.EventID,
		Logger:       &logger,
	})

	// Converge the reader to the golden TrakRF mqtt-rpc config before serving RPC.
	// The daemon is opinionated about the ONE hand-crafted prerequisite: the golden
	// CloudServer. If it is missing, refuse to serve (fatal) — the missing/misnamed
	// server is a permanent commissioning error and serving a half-configured reader
	// is worse than not serving. Transient failures (reader busy, broker down,
	// GlassFish still booting) are logged loudly but the daemon still serves; the
	// next restart re-arms. With systemd Restart=always, the fatal case self-heals
	// once the operator creates the CloudServer.
	if cfg.Reconcile {
		rctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
		err := adapter.Reconcile(rctx)
		cancel()
		switch {
		case err == nil:
			logger.Info().Msg("golden-config reconcile complete")
		case errors.Is(err, cs463.ErrMissingCloudServer):
			logger.Fatal().Err(err).Msg("golden-config reconcile: required CloudServer missing — refusing to serve; create it (exact name) and the daemon will recover on restart")
		default:
			logger.Error().Err(err).Msg("golden-config reconcile failed; serving RPC anyway")
		}
	}

	d := readerd.New(cfg, adapter, &logger)
	d.SetUpdater(upd)
	if err := d.Start(); err != nil {
		logger.Fatal().Err(err).Msg("start daemon")
	}
	defer d.Stop()

	logger.Info().
		Str("base_topic", cfg.Broker.BaseTopic).
		Str("rpc_client_id", cfg.Broker.RPCClientID).
		Str("reader_url", baseURL).
		Msg("mqtt-rpcd running")

	select {
	case <-ctx.Done():
		logger.Info().Msg("mqtt-rpcd shutting down")
	case <-restart:
		// Exit cleanly; systemd (Restart=always) starts the binary the active
		// link now points at, and the guard's ExecStartPre counts the start.
		logger.Info().Msg("mqtt-rpcd exiting to restart onto the updated version")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
