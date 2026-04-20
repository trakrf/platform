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

	var locIdentsArg any
	if len(filter.LocationIdentifiers) > 0 {
		locIdentsArg = filter.LocationIdentifiers
	}
	var qArg any
	if filter.Q != nil {
		q := "%" + *filter.Q + "%"
		qArg = q
	}

	rows, err := s.pool.Query(ctx, query, orgID, locIdentsArg, qArg, filter.Limit, filter.Offset)
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
			&item.LocationIdentifier,
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
		JOIN trakrf.assets    a ON a.id = ls.asset_id
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.deleted_at IS NULL
		WHERE ($2::text[] IS NULL OR l.identifier = ANY($2::text[]))
		  AND ($3::text IS NULL OR a.name ILIKE $3
			   OR EXISTS (
				   SELECT 1 FROM trakrf.identifiers ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE $3
			   ))
	`

	var locIdentsArg any
	if len(filter.LocationIdentifiers) > 0 {
		locIdentsArg = filter.LocationIdentifiers
	}
	var qArg any
	if filter.Q != nil {
		q := "%" + *filter.Q + "%"
		qArg = q
	}

	var count int
	err := s.pool.QueryRow(ctx, query, orgID, locIdentsArg, qArg).Scan(&count)
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
			l.identifier AS location_identifier,
			ls.last_seen
		FROM latest_scans ls
		JOIN trakrf.assets a ON a.id = ls.asset_id
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.deleted_at IS NULL
		WHERE ($2::text[] IS NULL OR l.identifier = ANY($2::text[]))
		  AND ($3::text IS NULL OR a.name ILIKE $3
			   OR EXISTS (
				   SELECT 1 FROM trakrf.identifiers ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE $3
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
			l.identifier AS location_identifier,
			ls.last_seen
		FROM latest_scans ls
		JOIN trakrf.assets a ON a.id = ls.asset_id
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.deleted_at IS NULL
		WHERE ($2::text[] IS NULL OR l.identifier = ANY($2::text[]))
		  AND ($3::text IS NULL OR a.name ILIKE $3
			   OR EXISTS (
				   SELECT 1 FROM trakrf.identifiers ai
				   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE $3
			   ))
		ORDER BY a.name
		LIMIT $4 OFFSET $5
	`
}

// ListAssetHistory returns paginated location history for a single asset
func (s *Storage) ListAssetHistory(ctx context.Context, assetID, orgID int, filter report.AssetHistoryFilter) ([]report.AssetHistoryItem, error) {
	query := `
		WITH scans AS (
			SELECT
				s.timestamp,
				s.location_id,
				l.name AS location_name,
				l.identifier AS location_identifier,
				LEAD(s.timestamp) OVER (ORDER BY s.timestamp) AS next_timestamp
			FROM trakrf.asset_scans s
			LEFT JOIN trakrf.locations l ON l.id = s.location_id
			WHERE s.asset_id = $1
			  AND s.org_id = $2
			  AND ($3::timestamptz IS NULL OR s.timestamp >= $3)
			  AND ($4::timestamptz IS NULL OR s.timestamp <= $4)
		)
		SELECT
			timestamp,
			location_id,
			location_name,
			location_identifier,
			EXTRACT(EPOCH FROM (next_timestamp - timestamp))::INT AS duration_seconds
		FROM scans
		ORDER BY timestamp DESC
		LIMIT $5 OFFSET $6
	`

	rows, err := s.pool.Query(ctx, query, assetID, orgID, filter.From, filter.To, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list asset history: %w", err)
	}
	defer rows.Close()

	var items []report.AssetHistoryItem
	for rows.Next() {
		var item report.AssetHistoryItem
		err := rows.Scan(
			&item.Timestamp,
			&item.LocationID,
			&item.LocationName,
			&item.LocationIdentifier,
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
