package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/report"
)

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

// disableSkipScan turns off TimescaleDB's SkipScan planner optimization for the
// current transaction.
//
// The asset-locations report resolves each asset's most-recent scan with a
// `SELECT DISTINCT ON (asset_id) ... ORDER BY asset_id, timestamp DESC` over the
// asset_scans hypertable — exactly the shape SkipScan targets. Since TRA-875
// added an RLS policy to asset_scans, the policy qual is planned as a `Result`
// subplan under the SkipScan node, and TimescaleDB 2.18 aborts at execution with
// "unsupported subplan type for SkipScan: Result" (SQLSTATE XX000) — a hard 500
// on every call once the org has enough scan rows for the planner to choose
// SkipScan. It surfaces only under the RLS role (trakrf-app), never as superuser,
// because the superuser bypasses the policy that injects the Result node.
//
// Disabling SkipScan makes the DISTINCT ON fall back to a plain ordered index
// scan, which returns identical rows. SET LOCAL scopes this to the transaction
// so no other query loses the optimization. Both the list and count queries use
// DISTINCT ON, so both must call this.
func disableSkipScan(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "SET LOCAL timescaledb.enable_skipscan = off"); err != nil {
		return fmt.Errorf("failed to disable skipscan: %w", err)
	}
	return nil
}

// ListCurrentLocations returns paginated current asset locations.
//
// Latest-scan-per-asset is resolved from the asset_scan_latest continuous
// aggregate (TRA-1022): the CAGG holds last(location_id)/max(timestamp) per
// time_bucket per (org_id, asset_id); the query collapses those buckets to one
// row per asset with an outer last()/max(). This replaces the DISTINCT ON over
// the asset_scans hypertable that TRA-1021 had to defuse with SkipScan-off.
// org_id is filtered explicitly because RLS does not extend to the CAGG.
func (s *Storage) ListCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) ([]report.CurrentLocationItem, error) {
	orderBy := buildCurrentLocationsOrderBy(filter.Sorts)
	query := buildCurrentLocationsQuery(orderBy)

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
	// Same CAGG-sourced latest_scans CTE as the list query (TRA-1022).
	query := `
		WITH latest_scans AS (
			SELECT
				asset_id,
				last(location_id, last_seen) AS location_id
			FROM trakrf.asset_scan_latest
			WHERE org_id = $1
			GROUP BY asset_id
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

// buildCurrentLocationsQuery renders the list query. The latest_scans CTE reads
// the asset_scan_latest CAGG and collapses its per-bucket rows to one row per
// asset (last(location_id) by newest bucket, max(last_seen)). Everything below
// the CTE — joins, temporal-validity predicates, filters, sort, pagination — is
// unchanged from the pre-CAGG query.
func buildCurrentLocationsQuery(orderBy string) string {
	return `
		WITH latest_scans AS (
			SELECT
				asset_id,
				last(location_id, last_seen) AS location_id,
				max(last_seen)               AS last_seen
			FROM trakrf.asset_scan_latest
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
			-- Cast to BIGINT, not INT: a legitimate >68-year gap between two
			-- consecutive scans overflows EXTRACT(EPOCH ...)::INT (int4,
			-- SQLSTATE 22003). BIGINT holds any epoch difference across the
			-- timestamp range; DurationSeconds is *int / 64-bit Go-side, so it
			-- scans cleanly.
			EXTRACT(EPOCH FROM (next_timestamp - timestamp))::BIGINT AS duration_seconds
		FROM scans
		ORDER BY ` + orderBy + `
		LIMIT $5 OFFSET $6
	`

	// Run inside WithOrgTx so SET LOCAL app.current_org_id is in effect: the
	// LEFT JOIN onto trakrf.locations is subject to the org-isolation RLS
	// policy, which casts current_setting('app.current_org_id')::bigint. Querying
	// on the raw pool leaves that setting empty/unset and the policy aborts the
	// scan (SQLSTATE 22P02 / 42704) the moment a location row is probed — i.e. a
	// 500 on every asset that has any scan history. (TRA-865.)
	items := []report.AssetHistoryItem{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, assetID, orgID, filter.From, filter.To, filter.Limit, filter.Offset)
		if err != nil {
			return fmt.Errorf("failed to list asset history: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var item report.AssetHistoryItem
			if err := rows.Scan(
				&item.Timestamp,
				&item.LocationID,
				&item.LocationName,
				&item.LocationExternalKey,
				&item.DurationSeconds,
			); err != nil {
				return fmt.Errorf("failed to scan asset history: %w", err)
			}
			items = append(items, item)
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("error iterating asset history: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
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

	// Wrapped in WithOrgTx for parity with ListAssetHistory and the other
	// report queries: asset_scans carries its own org-isolation RLS policy
	// (TRA-875), so the org context must be set or this COUNT fails the policy
	// qual the moment it scans (22P02/42704) — the same loud failure mode
	// TRA-865 produced on the locations join.
	var count int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, assetID, orgID, filter.From, filter.To).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count asset history: %w", err)
	}

	return count, nil
}
