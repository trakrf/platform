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

func locationsCommand() *cli.Command {
	return &cli.Command{
		Name:  "locations",
		Usage: "List and inspect locations",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List locations",
				Flags: append(listFlags(),
					&cli.IntSliceFlag{Name: "parent-id", Usage: "filter by parent id (repeatable)"},
					&cli.StringSliceFlag{Name: "parent-external-key", Usage: "filter by parent external_key (repeatable)"},
				),
				Action: withRunCtx(runLocationsList),
			},
			{
				Name:      "get",
				Usage:     "Get a single location by numeric id",
				ArgsUsage: "<id>",
				Action:    withRunCtx(runLocationsGet),
			},
		},
	}
}

func runLocationsList(ctx context.Context, cmd *cli.Command, rc *runCtx) error {
	params := &api.ListLocationsParams{}
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
	if pids := cmd.IntSlice("parent-id"); len(pids) > 0 {
		ids := make([]int64, len(pids))
		for i, p := range pids {
			ids[i] = int64(p)
		}
		params.ParentId = &ids
	}
	if pks := cmd.StringSlice("parent-external-key"); len(pks) > 0 {
		params.ParentExternalKey = &pks
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

	resp, err := rc.client.ListLocationsWithResponse(ctx, params)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return apiclient.APIError(resp.StatusCode(), resp.Body)
	}
	return rc.render(output.Renderable{
		JSON:  resp.JSON200,
		Table: locationTable(resp.JSON200.Data),
	})
}

func runLocationsGet(ctx context.Context, cmd *cli.Command, rc *runCtx) error {
	arg, err := requireArg(cmd, "id")
	if err != nil {
		return err
	}
	locationID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid location id %q: must be a numeric id (use `locations list --external-key=%s` to look up by external key)", arg, arg)
	}

	resp, err := rc.client.GetLocationWithResponse(ctx, locationID)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return apiclient.APIError(resp.StatusCode(), resp.Body)
	}
	return rc.render(output.Renderable{
		JSON:  resp.JSON200,
		Table: locationTable([]api.LocationView{resp.JSON200.Data}),
	})
}
