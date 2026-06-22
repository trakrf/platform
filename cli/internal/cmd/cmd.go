// Package cmd builds the urfave/cli v3 command tree for the trakrf CLI and wires
// each command to the generated API client through the config/auth/output
// helpers. Command files: auth.go, assets.go, locations.go, orgs.go.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/trakrf/platform/cli/api"
	"github.com/trakrf/platform/cli/internal/apiclient"
	"github.com/trakrf/platform/cli/internal/auth"
	"github.com/trakrf/platform/cli/internal/config"
	"github.com/trakrf/platform/cli/internal/output"
)

// NewApp constructs the root command. version is stamped at build time.
func NewApp(version string) *cli.Command {
	return &cli.Command{
		Name:                  "trakrf",
		Usage:                 "Scriptable access to the TrakRF public REST API",
		Version:               version,
		EnableShellCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "profile",
				Usage: "config profile to use (default: current profile)",
			},
			&cli.StringFlag{
				Name:  "env",
				Usage: "override the profile environment: prod or preview",
			},
			&cli.StringFlag{
				Name:  "format",
				Usage: "output format: table, json, or csv",
				Value: output.FormatTable,
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "shorthand for --format json",
			},
			&cli.StringFlag{
				Name:  "config",
				Usage: "path to the config file (default: ~/.trakrf/config.yaml)",
			},
		},
		Commands: []*cli.Command{
			authCommand(),
			assetsCommand(),
			locationsCommand(),
			orgsCommand(),
		},
	}
}

// runCtx carries everything a resource command needs: resolved config, the
// chosen output format, and an authenticated API client.
type runCtx struct {
	cfg         *config.Config
	configPath  string
	profileName string
	profile     *config.Profile
	baseURL     string
	format      string
	client      *api.ClientWithResponses
}

// newRunCtx resolves config + profile and builds an authenticated client. It is
// used by every command that calls the API (not by auth login / orgs switch,
// which manipulate config directly).
func newRunCtx(cmd *cli.Command) (*runCtx, error) {
	format, err := output.ParseFormat(cmd.String("format"), cmd.Bool("json"))
	if err != nil {
		return nil, err
	}
	configPath, err := resolveConfigPath(cmd)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	name, prof, err := cfg.Resolve(config.ResolveInput{
		Profile:   cmd.String("profile"),
		Env:       cmd.String("env"),
		OrgEnv:    os.Getenv("TRAKRF_ORG"),
		APIKeyEnv: os.Getenv("TRAKRF_API_KEY"),
	})
	if err != nil {
		return nil, err
	}
	baseURL, err := resolveBaseURL(prof.Env)
	if err != nil {
		return nil, err
	}

	_, stored := cfg.Profiles[name]
	provider := &auth.Provider{
		Profile: prof,
		Minter:  &apiclient.Minter{BaseURL: baseURL},
		Persist: func(tok config.CachedToken) error {
			// Only cache the token for a real, stored profile — never for
			// ephemeral TRAKRF_API_KEY credentials.
			if !stored {
				return nil
			}
			cfg.Profiles[name].Token = &tok
			return config.Save(configPath, cfg)
		},
	}
	client, err := apiclient.New(baseURL, provider)
	if err != nil {
		return nil, err
	}

	return &runCtx{
		cfg:         cfg,
		configPath:  configPath,
		profileName: name,
		profile:     prof,
		baseURL:     baseURL,
		format:      format,
		client:      client,
	}, nil
}

func resolveConfigPath(cmd *cli.Command) (string, error) {
	if p := cmd.String("config"); p != "" {
		return p, nil
	}
	return config.DefaultPath()
}

// resolveBaseURL maps an environment to its API base URL, honoring a
// TRAKRF_API_URL override (useful for self-hosted deployments and tests).
func resolveBaseURL(env string) (string, error) {
	if override := os.Getenv("TRAKRF_API_URL"); override != "" {
		return override, nil
	}
	return config.BaseURL(env)
}

// render writes a result to stdout in the resolved format.
func (rc *runCtx) render(r output.Renderable) error {
	return output.Render(os.Stdout, rc.format, r)
}

// withContext is a tiny adapter so command actions can ignore the cli.Command
// when they only need the resolved runCtx.
func withRunCtx(fn func(context.Context, *cli.Command, *runCtx) error) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		rc, err := newRunCtx(cmd)
		if err != nil {
			return err
		}
		return fn(ctx, cmd, rc)
	}
}

func requireArg(cmd *cli.Command, name string) (string, error) {
	v := cmd.Args().First()
	if v == "" {
		return "", fmt.Errorf("missing required <%s> argument", name)
	}
	return v, nil
}
