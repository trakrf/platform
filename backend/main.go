// @title TrakRF API
// @version v1
// @description TrakRF public REST API. See docs.trakrf.id/api for the customer-facing reference.
// @contact.name TrakRF Support
// @contact.email support@trakrf.id
// @license.name Business Source License 1.1
// @license.url https://github.com/trakrf/platform/blob/main/LICENSE
// @host app.trakrf.id
// @BasePath /api/v1
// @schemes https
// @securityDefinitions.apikey APIKey
// @in header
// @name Authorization
// @description TrakRF API key (JWT). Format: "Bearer <jwt>". Mint keys from the API Keys section of your TrakRF account (key-management UI is tracked by TRA-393).
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Session JWT for internal endpoints (platform frontend uses this).
package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/trakrf/platform/backend/internal/cmd/migrate"
	"github.com/trakrf/platform/backend/internal/cmd/serve"
	"github.com/trakrf/platform/backend/internal/logger"
)

//go:embed frontend/dist
var frontendFS embed.FS

var version = "dev"

type command int

const (
	cmdCombined command = iota // no arg: migrate then serve
	cmdServe
	cmdMigrate
	cmdHelp
	cmdUnknown
)

const usage = "usage: server [serve|migrate]"

func parseCommand(args []string) (command, error) {
	if len(args) == 0 {
		return cmdCombined, nil
	}
	if len(args) > 1 {
		return cmdUnknown, fmt.Errorf("unexpected extra arguments: %v", args[1:])
	}
	switch args[0] {
	case "serve":
		return cmdServe, nil
	case "migrate":
		return cmdMigrate, nil
	case "-h", "--help":
		return cmdHelp, nil
	default:
		return cmdUnknown, fmt.Errorf("unknown subcommand: %q", args[0])
	}
}

func main() {
	cmd, err := parseCommand(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if cmd == cmdHelp {
		fmt.Println(usage)
		os.Exit(0)
	}

	loggerCfg := logger.NewConfig(version)
	logger.Initialize(loggerCfg)
	log := logger.Get()
	log.Info().Msg("Logger initialized")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runErr := run(ctx, cmd)
	if runErr != nil {
		log.Error().Err(runErr).Msg("Command failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd command) error {
	switch cmd {
	case cmdMigrate:
		return migrate.Run(ctx, version)
	case cmdServe:
		return serve.Run(ctx, version, frontendFS)
	case cmdCombined:
		if err := migrate.Run(ctx, version); err != nil {
			return err
		}
		return serve.Run(ctx, version, frontendFS)
	}
	return fmt.Errorf("unreachable command: %v", cmd)
}
