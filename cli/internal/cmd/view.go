package cmd

import (
	"strconv"
	"strings"
	"time"

	"github.com/trakrf/platform/cli/api"
	"github.com/trakrf/platform/cli/internal/output"
)

// --- pointer / value formatting helpers -------------------------------------

func id64(p *int64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func ts(p *time.Time) string {
	if p == nil {
		return ""
	}
	return p.UTC().Format(time.RFC3339)
}

// --- columnar projections for table / CSV -----------------------------------

var assetColumns = []string{"ID", "EXTERNAL_KEY", "NAME", "ACTIVE", "UPDATED"}

func assetRow(a api.AssetView) []string {
	return []string{id64(a.Id), a.ExternalKey, a.Name, yesNo(a.IsActive), ts(a.UpdatedAt)}
}

func assetTable(assets []api.AssetView) output.Table {
	rows := make([][]string, 0, len(assets))
	for _, a := range assets {
		rows = append(rows, assetRow(a))
	}
	return output.Table{Columns: assetColumns, Rows: rows}
}

var locationColumns = []string{"ID", "EXTERNAL_KEY", "NAME", "PARENT_ID", "ACTIVE"}

func locationRow(l api.LocationView) []string {
	return []string{id64(l.Id), l.ExternalKey, l.Name, id64(l.ParentId), yesNo(l.IsActive)}
}

func locationTable(locs []api.LocationView) output.Table {
	rows := make([][]string, 0, len(locs))
	for _, l := range locs {
		rows = append(rows, locationRow(l))
	}
	return output.Table{Columns: locationColumns, Rows: rows}
}

func orgTable(o api.OrgView) output.Table {
	return output.Table{
		Columns: []string{"ID", "NAME", "API_KEY_ID", "SCOPES"},
		Rows:    [][]string{{id64(o.Id), o.Name, o.ApiKeyId, strings.Join(o.Scopes, " ")}},
	}
}
