package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/trakrf/platform/cli/internal/apiclient"
	"github.com/trakrf/platform/cli/internal/config"
	"github.com/trakrf/platform/cli/internal/output"
)

func orgsCommand() *cli.Command {
	return &cli.Command{
		Name:  "orgs",
		Usage: "Show the caller's org and switch between local profiles",
		Commands: []*cli.Command{
			{
				Name: "list",
				// The public API binds one org per API key, so there is no
				// org-list endpoint: `list` shows the org this key resolves to.
				Usage:  "Show the organization the active API key belongs to",
				Action: withRunCtx(runOrgsList),
			},
			{
				Name:      "switch",
				Usage:     "Switch the active config profile (local only — not an API call)",
				ArgsUsage: "<profile>",
				Action:    runOrgsSwitch,
			},
		},
	}
}

func runOrgsList(ctx context.Context, cmd *cli.Command, rc *runCtx) error {
	resp, err := rc.client.GetCurrentOrgWithResponse(ctx)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return apiclient.APIError(resp.StatusCode(), resp.Body)
	}
	return rc.render(output.Renderable{
		JSON:  resp.JSON200,
		Table: orgTable(resp.JSON200.Data),
	})
}

// runOrgsSwitch changes which local profile is active. The public API ties an
// org to its API key, so "switching orgs" means selecting a different stored
// credential profile — a local config edit, not a server call.
func runOrgsSwitch(ctx context.Context, cmd *cli.Command) error {
	target, err := requireArg(cmd, "profile")
	if err != nil {
		return err
	}
	configPath, err := resolveConfigPath(cmd)
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[target]; !ok {
		return fmt.Errorf("profile %q not found (run `trakrf auth login --profile %s`)", target, target)
	}
	cfg.CurrentProfile = target
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Switched to profile %q (%s).\n", target, cfg.Profiles[target].Env)
	return nil
}
