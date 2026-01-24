package storage

import (
	"context"
	"fmt"
	"os"

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

// ListCurrentLocations returns paginated current asset locations
func (s *Storage) ListCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) ([]report.CurrentLocationItem, error) {
	engine := getReportsQueryEngine()

	var query string
	if engine == QueryEngineTimescaleLast {
		query = buildCurrentLocationsQueryTimescale()
	} else {
		query = buildCurrentLocationsQueryDistinctOn()
	}

	rows, err := s.pool.Query(ctx, query, orgID, filter.LocationID, filter.Search, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list current locations: %w", err)
	}
	defer rows.Close()

	var items []report.CurrentLocationItem
	for rows.Next() {
		var item report.CurrentLocationItem
		err := rows.Scan(
			&item.AssetID,
			&item.AssetName,
			&item.AssetIdentifier,
			&item.LocationID,
			&item.LocationName,
			&item.LastSeen,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan current location: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating current locations: %w", err)
	}

	if items == nil {
		items = []report.CurrentLocationItem{}
	}

	return items, nil
}

// CountCurrentLocations returns total count for pagination
func (s *Storage) CountCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) (int, error) {
	query := buildCurrentLocationsCountQuery()

	var count int
	err := s.pool.QueryRow(ctx, query, orgID, filter.LocationID, filter.Search).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count current locations: %w", err)
	}

	return count, nil
}

func buildCurrentLocationsQueryDistinctOn() string {
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
			ls.asset_id,
			a.name AS asset_name,
			COALESCE(
				(SELECT i.value FROM trakrf.identifiers i
				 WHERE i.asset_id = a.id AND i.is_active = true LIMIT 1),
				''
			) AS asset_identifier,
			ls.location_id,
			l.name AS location_name,
			ls.last_seen
		FROM latest_scans ls
		JOIN trakrf.assets a ON a.id = ls.asset_id
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id
		WHERE ($2::int IS NULL OR ls.location_id = $2)
		  AND ($3::text IS NULL OR a.name ILIKE '%' || $3 || '%'
			   OR EXISTS (
				   SELECT 1 FROM trakrf.identifiers ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE '%' || $3 || '%'
			   ))
		ORDER BY a.name
		LIMIT $4 OFFSET $5
	`
}

func buildCurrentLocationsQueryTimescale() string {
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
			ls.asset_id,
			a.name AS asset_name,
			COALESCE(
				(SELECT i.value FROM trakrf.identifiers i
				 WHERE i.asset_id = a.id AND i.is_active = true LIMIT 1),
				''
			) AS asset_identifier,
			ls.location_id,
			l.name AS location_name,
			ls.last_seen
		FROM latest_scans ls
		JOIN trakrf.assets a ON a.id = ls.asset_id
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id
		WHERE ($2::int IS NULL OR ls.location_id = $2)
		  AND ($3::text IS NULL OR a.name ILIKE '%' || $3 || '%'
			   OR EXISTS (
				   SELECT 1 FROM trakrf.identifiers ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE '%' || $3 || '%'
			   ))
		ORDER BY a.name
		LIMIT $4 OFFSET $5
	`
}

func buildCurrentLocationsCountQuery() string {
	return `
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
		JOIN trakrf.assets a ON a.id = ls.asset_id
		WHERE ($2::int IS NULL OR ls.location_id = $2)
		  AND ($3::text IS NULL OR a.name ILIKE '%' || $3 || '%'
			   OR EXISTS (
				   SELECT 1 FROM trakrf.identifiers ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE '%' || $3 || '%'
			   ))
	`
}
