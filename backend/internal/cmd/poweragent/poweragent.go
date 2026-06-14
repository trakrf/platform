// Package poweragent is the `server poweragent` subcommand entrypoint: it loads
// the agent config, starts the MQTT command loop, and blocks until the process
// is signalled. It shares the server binary/image so the edge box needs no extra
// artifact — just a second container invoking this subcommand.
package poweragent

import (
	"context"

	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/poweragent"
)

// Run starts the power agent and blocks until ctx is cancelled.
func Run(ctx context.Context, info buildinfo.Info) error {
	log := logger.Get()

	cfg, err := poweragent.LoadConfig()
	if err != nil {
		return err
	}

	agent := poweragent.New(cfg, *log)
	agent.Start()
	defer agent.Stop()

	log.Info().Str("version", info.Version).Int("readers", len(cfg.Readers)).Msg("power agent running")
	<-ctx.Done()
	log.Info().Msg("power agent shutting down")
	return nil
}
