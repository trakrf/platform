package storage

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/report"
)

// QueryEngine determines which SQL strategy to use
type QueryEngine string

const (
	QueryEngineDistinctOn    QueryEngine = "distinct_on"
	QueryEngineTimescaleLast QueryEngine = "timescale_last"
)

// getReportsQueryEngine returns the configured query engine
func getReportsQueryEngine() QueryEngine {
	engine := os.Getenv("REPORTS_QUERY_ENGINE")
	if engine == string(QueryEngineTimescaleLast) {
		return QueryEngineTimescaleLast
	}
	return QueryEngineDistinctOn // default
}

// currentLocationsArgs prepares the variadic args shared by list + count
// queries. Each filter short-circuits to NULL when empty so the SQL
// `$N::T[] IS NULL OR ...` branches behave as no-ops.
func currentLocationsArgs(filter report.CurrentLocationFilter) (locIDsArg, locKeysArg, qArg, assetIDsArg, assetKeysArg any) {
	if len(filter.LocationIDs) > 0 {
		locIDsArg = filter.LocationIDs
	}
	if len(filter.LocationExternalKeys) > 0 {
		locKeysArg = filter.LocationExternalKeys
	}
	if filter.Q != nil {
		q := "%" + *filter.Q + "%"
		qArg = q
	}
	if len(filter.AssetIDs) > 0 {
		assetIDsArg = filter.AssetIDs
	}
	if len(filter.AssetExternalKeys) > 0 {
		assetKeysArg = filter.AssetExternalKeys
	}
	return
}

// ListCurrentLocations returns paginated current asset locations
func (s *Storage) ListCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) ([]report.CurrentLocationItem, error) {
	engine := getReportsQueryEngine()

	orderBy := buildCurrentLocationsOrderBy(filter.Sorts)

	var query string
	if engine == QueryEngineTimescaleLast {
		query = buildCurrentLocationsQueryTimescale(orderBy)
	} else {
		query = buildCurrentLocationsQueryDistinctOn(orderBy)
	}

	locIDsArg, locKeysArg, qArg, assetIDsArg, assetKeysArg := currentLocationsArgs(filter)

	items := []report.CurrentLocationItem{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, locIDsArg, locKeysArg, qArg, filter.Limit, filter.Offset, filter.IncludeDeleted, assetIDsArg, assetKeysArg)
		if err != nil {
			return fmt.Errorf("failed to list current locations: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var item report.CurrentLocationItem
			if err := rows.Scan(
				&item.AssetID,
				&item.AssetName,
				&item.AssetExternalKey,
				&item.LocationID,
				&item.LocationName,
				&item.LocationExternalKey,
				&item.LastSeen,
				&item.AssetDeletedAt,
			); err != nil {
				return fmt.Errorf("failed to scan current location: %w", err)
			}
			items = append(items, item)
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("error iterating current locations: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return items, nil
}

// CountCurrentLocations returns total count for pagination
func (s *Storage) CountCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) (int, error) {
	query := `
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT COUNT(*)
		FROM latest_scans ls
		JOIN trakrf.assets    a ON a.id = ls.asset_id AND a.org_id = $1 AND ` + temporallyEffective("a") + `
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.org_id = $1 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
		WHERE ($2::bigint[]  IS NULL OR l.id           = ANY($2::bigint[]))
		  AND ($3::text[] IS NULL OR l.external_key = ANY($3::text[]))
		  AND ($4::text IS NULL OR a.name ILIKE $4 OR a.external_key ILIKE $4
			   OR EXISTS (
				   SELECT 1 FROM trakrf.tags ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.deleted_at IS NULL AND ` + temporallyEffective("ai") + ` AND ai.value ILIKE $4
			   ))
		  AND (a.deleted_at IS NULL OR $5::bool)
		  AND ($6::bigint[]  IS NULL OR a.id           = ANY($6::bigint[]))
		  AND ($7::text[] IS NULL OR a.external_key = ANY($7::text[]))
	`

	locIDsArg, locKeysArg, qArg, assetIDsArg, assetKeysArg := currentLocationsArgs(filter)

	var count int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, locIDsArg, locKeysArg, qArg, filter.IncludeDeleted, assetIDsArg, assetKeysArg).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count current locations: %w", err)
	}

	return count, nil
}

// buildCurrentLocationsOrderBy resolves the documented sort enum
// (asset_last_seen, asset_external_key, location_external_key) into the SQL
// ORDER BY fragment used by both query strategies. Default order — when
// no sort is supplied — is most-recent-first by last_seen, with a stable
// tiebreaker on asset id so pagination is deterministic across pages.
//
// The wire-level sort key is `asset_last_seen` per TRA-717 / BB34 F2; the
// underlying storage column is still `last_seen` on the latest-scan
// materialization (TRA-641 / BB21 §2.6 carried over). "no prefix means
// ASC" per the public API convention.
func buildCurrentLocationsOrderBy(sorts []report.CurrentLocationSort) string {
	if len(sorts) == 0 {
		return "ls.last_seen DESC, a.id ASC"
	}
	out := make([]string, 0, len(sorts))
	for _, s := range sorts {
		var col string
		switch s.Field {
		case "asset_last_seen":
			col = "ls.last_seen"
		case "asset_external_key":
			col = "a.external_key"
		case "location_external_key":
			col = "l.external_key"
		default:
			continue
		}
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		out = append(out, col+" "+dir)
	}
	if len(out) == 0 {
		return "ls.last_seen DESC, a.id ASC"
	}
	return strings.Join(out, ", ")
}

func buildCurrentLocationsQueryDistinctOn(orderBy string) string {
	return `
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id,
				s.timestamp AS last_seen
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT
			a.id            AS asset_id,
			a.name          AS asset_name,
			a.external_key  AS asset_external_key,
			l.id            AS location_id,
			l.name          AS location_name,
			l.external_key  AS location_external_key,
			ls.last_seen,
			a.deleted_at    AS asset_deleted_at
		FROM latest_scans ls
		JOIN trakrf.assets a ON a.id = ls.asset_id AND a.org_id = $1 AND ` + temporallyEffective("a") + `
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.org_id = $1 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
		WHERE ($2::bigint[]  IS NULL OR l.id           = ANY($2::bigint[]))
		  AND ($3::text[] IS NULL OR l.external_key = ANY($3::text[]))
		  AND ($4::text IS NULL OR a.name ILIKE $4 OR a.external_key ILIKE $4
			   OR EXISTS (
				   SELECT 1 FROM trakrf.tags ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.deleted_at IS NULL AND ` + temporallyEffective("ai") + ` AND ai.value ILIKE $4
			   ))
		  AND (a.deleted_at IS NULL OR $7::bool)
		  AND ($8::bigint[]  IS NULL OR a.id           = ANY($8::bigint[]))
		  AND ($9::text[] IS NULL OR a.external_key = ANY($9::text[]))
		ORDER BY ` + orderBy + `
		LIMIT $5 OFFSET $6
	`
}

func buildCurrentLocationsQueryTimescale(orderBy string) string {
	return `
		WITH latest_scans AS (
			SELECT
				asset_id,
				last(location_id, timestamp) AS location_id,
				max(timestamp) AS last_seen
			FROM trakrf.asset_scans
			WHERE org_id = $1
			GROUP BY asset_id
		)
		SELECT
			a.id            AS asset_id,
			a.name          AS asset_name,
			a.external_key  AS asset_external_key,
			l.id            AS location_id,
			l.name          AS location_name,
			l.external_key  AS location_external_key,
			ls.last_seen,
			a.deleted_at    AS asset_deleted_at
		FROM latest_scans ls
		JOIN trakrf.assets a ON a.id = ls.asset_id AND a.org_id = $1 AND ` + temporallyEffective("a") + `
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.org_id = $1 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
		WHERE ($2::bigint[]  IS NULL OR l.id           = ANY($2::bigint[]))
		  AND ($3::text[] IS NULL OR l.external_key = ANY($3::text[]))
		  AND ($4::text IS NULL OR a.name ILIKE $4 OR a.external_key ILIKE $4
			   OR EXISTS (
				   SELECT 1 FROM trakrf.tags ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.deleted_at IS NULL AND ` + temporallyEffective("ai") + ` AND ai.value ILIKE $4
			   ))
		  AND (a.deleted_at IS NULL OR $7::bool)
		  AND ($8::bigint[]  IS NULL OR a.id           = ANY($8::bigint[]))
		  AND ($9::text[] IS NULL OR a.external_key = ANY($9::text[]))
		ORDER BY ` + orderBy + `
		LIMIT $5 OFFSET $6
	`
}

// buildAssetHistoryOrderBy renders the ORDER BY fragment for the
// listAssetHistory query. Default — when no sort token is supplied — is
// most-recent-first by event_observed_at with a stable tiebreaker on
// location_id so pagination is deterministic across pages of
// same-timestamp rows. "no prefix means ASC" per the public API
// convention; only the spec-allowlisted sort field is recognised.
func buildAssetHistoryOrderBy(sorts []report.AssetHistorySort) string {
	const defaultOrder = "timestamp DESC, location_id ASC"
	if len(sorts) == 0 {
		return defaultOrder
	}
	out := make([]string, 0, len(sorts))
	for _, s := range sorts {
		var col string
		switch s.Field {
		case "event_observed_at":
			col = "timestamp"
		default:
			continue
		}
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		out = append(out, col+" "+dir)
	}
	if len(out) == 0 {
		return defaultOrder
	}
	return strings.Join(out, ", ")
}

// ListAssetHistory returns paginated location history for a single asset
func (s *Storage) ListAssetHistory(ctx context.Context, assetID, orgID int, filter report.AssetHistoryFilter) ([]report.AssetHistoryItem, error) {
	orderBy := buildAssetHistoryOrderBy(filter.Sorts)
	query := `
		WITH scans AS (
			SELECT
				s.timestamp,
				s.location_id,
				l.name         AS location_name,
				l.external_key AS location_external_key,
				LEAD(s.timestamp) OVER (ORDER BY s.timestamp) AS next_timestamp
			FROM trakrf.asset_scans s
			LEFT JOIN trakrf.locations l ON l.id = s.location_id AND l.org_id = $2 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
			WHERE s.asset_id = $1
			  AND s.org_id = $2
			  AND ($3::timestamptz IS NULL OR s.timestamp >= $3)
			  AND ($4::timestamptz IS NULL OR s.timestamp <= $4)
		)
		SELECT
			timestamp,
			location_id,
			location_name,
			location_external_key,
			-- TRA-865: cast to BIGINT, not INT. A single sentinel/bad timestamp
			-- in an asset's seeded scan history can put two consecutive scans
			-- more than ~68 years apart, and EXTRACT(EPOCH ...)::INT overflows
			-- int4 (SQLSTATE 22003) — crashing the whole query with a 500 for
			-- every row of that asset's history. BIGINT can hold any epoch
			-- difference across the timestamp range. (DurationSeconds is *int /
			-- 64-bit Go-side, so this scans cleanly.)
			EXTRACT(EPOCH FROM (next_timestamp - timestamp))::BIGINT AS duration_seconds
		FROM scans
		ORDER BY ` + orderBy + `
		LIMIT $5 OFFSET $6
	`

	rows, err := s.pool.Query(ctx, query, assetID, orgID, filter.From, filter.To, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list asset history: %w", err)
	}
	defer rows.Close()

	items := []report.AssetHistoryItem{}
	for rows.Next() {
		var item report.AssetHistoryItem
		err := rows.Scan(
			&item.Timestamp,
			&item.LocationID,
			&item.LocationName,
			&item.LocationExternalKey,
			&item.DurationSeconds,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset history: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating asset history: %w", err)
	}

	if items == nil {
		items = []report.AssetHistoryItem{}
	}

	return items, nil
}

// CountAssetHistory returns total count for pagination
func (s *Storage) CountAssetHistory(ctx context.Context, assetID, orgID int, filter report.AssetHistoryFilter) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM trakrf.asset_scans s
		WHERE s.asset_id = $1
		  AND s.org_id = $2
		  AND ($3::timestamptz IS NULL OR s.timestamp >= $3)
		  AND ($4::timestamptz IS NULL OR s.timestamp <= $4)
	`

	var count int
	err := s.pool.QueryRow(ctx, query, assetID, orgID, filter.From, filter.To).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count asset history: %w", err)
	}

	return count, nil
}
