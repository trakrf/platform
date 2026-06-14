// @title TrakRF API
// @version v1
// @description TrakRF public REST API. See /api for the customer-facing reference.
// @contact.name TrakRF Support
// @contact.email support@trakrf.id
// @license.name Business Source License 1.1
// @license.url https://github.com/trakrf/platform/blob/main/LICENSE
// @host app.trakrf.id
// @BasePath /api/v1
// @schemes https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description TrakRF API access token. Format: "Bearer <access_token>". Create API credentials in the API Keys section of your TrakRF account to obtain a {client_id, client_secret} pair, then exchange them at POST /oauth/token (grant_type=client_credentials) for a short-lived access token; send that token here. Some OpenAPI generators (e.g. openapi-fetch, openapi-generator-cli python target) do not auto-attach the Authorization header from this scheme — set it manually if your generated client does not. Similarly, some generated clients (notably openapi-fetch) do not auto-attach `Content-Type: application/merge-patch+json` on PATCH operations, even though the operation's requestBody.content lists only that media type; set the header manually per PATCH call if your client does not.
// @securityDefinitions.apikey SessionAuth
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
	"runtime"
	"syscall"

	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/cmd/migrate"
	readerdcmd "github.com/trakrf/platform/backend/internal/cmd/readerd"
	"github.com/trakrf/platform/backend/internal/cmd/serve"
	"github.com/trakrf/platform/backend/internal/logger"
)

//go:embed frontend/dist
var frontendFS embed.FS

// Build-time metadata. All four vars are ldflags targets populated by the
// Dockerfile / justfile / CI workflow; defaults apply to ad-hoc `go run .`.
// See TRA-481 for why this lives in /health rather than a dedicated
// /api/v1/version endpoint.
var (
	version   = "dev"
	commit    = "unknown"
	tag       = "dev"
	buildTime = "unknown"
)

type command int

const (
	// Bare `server` (no args) defaults to `serve` — no DDL at runtime.
	// Migrations run explicitly via the `migrate` subcommand under a separate
	// role with DDL rights. See backend/migrations/README.md for role-separation
	// rationale (TRA-85). Production helm explicitly invokes both subcommands;
	// this default matters for local docker / ad-hoc runs.
	cmdServe command = iota
	cmdMigrate
	cmdReaderd
	cmdHelp
	cmdUnknown
)

const usage = "usage: server [serve|migrate|readerd]"

func parseCommand(args []string) (command, error) {
	if len(args) == 0 {
		return cmdServe, nil
	}
	if len(args) > 1 {
		return cmdUnknown, fmt.Errorf("unexpected extra arguments: %v", args[1:])
	}
	switch args[0] {
	case "serve":
		return cmdServe, nil
	case "migrate":
		return cmdMigrate, nil
	case "readerd":
		return cmdReaderd, nil
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

	info := buildinfo.Info{
		Version:   version,
		Commit:    commit,
		Tag:       tag,
		BuildTime: buildTime,
		GoVersion: runtime.Version(),
	}

	runErr := run(ctx, cmd, info)
	if runErr != nil {
		log.Error().Err(runErr).Msg("Command failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd command, info buildinfo.Info) error {
	switch cmd {
	case cmdMigrate:
		return migrate.Run(ctx, info)
	case cmdReaderd:
		return readerdcmd.Run(ctx, info)
	case cmdServe:
		return serve.Run(ctx, info, frontendFS)
	}
	return fmt.Errorf("unreachable command: %v", cmd)
}
