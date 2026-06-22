package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/trakrf/platform/cli/api"
	"github.com/trakrf/platform/cli/internal/apiclient"
	"github.com/trakrf/platform/cli/internal/output"
)

func assetsCommand() *cli.Command {
	return &cli.Command{
		Name:  "assets",
		Usage: "List and inspect assets",
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "List assets",
				Flags:  listFlags(),
				Action: withRunCtx(runAssetsList),
			},
			{
				Name:      "get",
				Usage:     "Get a single asset by numeric id",
				ArgsUsage: "<id>",
				Action:    withRunCtx(runAssetsGet),
			},
		},
	}
}

// listFlags are the query filters shared by assets/locations list.
func listFlags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{Name: "limit", Usage: "max rows to return (max 200)"},
		&cli.IntFlag{Name: "offset", Usage: "rows to skip"},
		&cli.StringSliceFlag{Name: "external-key", Usage: "filter by external_key (repeatable)"},
		&cli.BoolFlag{Name: "active", Usage: "filter by active flag"},
		&cli.BoolFlag{Name: "include-deleted", Usage: "include soft-deleted rows"},
		&cli.StringFlag{Name: "search", Aliases: []string{"q"}, Usage: "case-insensitive substring search"},
		&cli.StringFlag{Name: "sort", Usage: "comma-separated sort; prefix '-' for DESC"},
	}
}

func runAssetsList(ctx context.Context, cmd *cli.Command, rc *runCtx) error {
	params := &api.ListAssetsParams{}
	if cmd.IsSet("limit") {
		v := int(cmd.Int("limit"))
		params.Limit = &v
	}
	if cmd.IsSet("offset") {
		v := int(cmd.Int("offset"))
		params.Offset = &v
	}
	if keys := cmd.StringSlice("external-key"); len(keys) > 0 {
		params.ExternalKey = &keys
	}
	if cmd.IsSet("active") {
		v := cmd.Bool("active")
		params.IsActive = &v
	}
	if cmd.IsSet("include-deleted") {
		v := cmd.Bool("include-deleted")
		params.IncludeDeleted = &v
	}
	if cmd.IsSet("search") {
		v := cmd.String("search")
		params.Q = &v
	}
	if cmd.IsSet("sort") {
		v := cmd.String("sort")
		params.Sort = &v
	}

	resp, err := rc.client.ListAssetsWithResponse(ctx, params)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return apiclient.APIError(resp.StatusCode(), resp.Body)
	}
	return rc.render(output.Renderable{
		JSON:  resp.JSON200,
		Table: assetTable(resp.JSON200.Data),
	})
}

func runAssetsGet(ctx context.Context, cmd *cli.Command, rc *runCtx) error {
	arg, err := requireArg(cmd, "id")
	if err != nil {
		return err
	}
	assetID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid asset id %q: must be a numeric id (use `assets list --external-key=%s` to look up by external key)", arg, arg)
	}

	resp, err := rc.client.GetAssetWithResponse(ctx, assetID)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return apiclient.APIError(resp.StatusCode(), resp.Body)
	}
	return rc.render(output.Renderable{
		JSON:  resp.JSON200,
		Table: assetTable([]api.AssetView{resp.JSON200.Data}),
	})
}
