package storage

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/report"
)

// freshScanTailMinutes bounds the raw-asset_scans tail that TRA-1117 splices onto
// the current-locations report in front of the asset_scan_latest CAGG.
//
// The CAGG is materialized_only, so a CAGG-only read is stale by at most
// end_offset (1 minute) + schedule_interval (30s) + one slipped cycle — call it
// two minutes (000028_asset_scan_latest_policy.up.sql). A tail SHORTER than that
// lag silently reintroduces the invisibility window this fix exists to close, so
// the window carries real margin rather than hugging the worst case. Widening it
// costs read work without changing any answer; narrowing it below the
// materialization lag loses rows. Adjust upward freely, downward never.
const freshScanTailMinutes = 5

// caggBucketMinutes is the asset_scan_latest bucket width
// (000027_asset_scan_latest_cagg.up.sql). Bucket-truncating an instant moves it
// backwards by strictly less than this.
const caggBucketMinutes = 1

var (
	freshScanTailWindow = strconv.Itoa(freshScanTailMinutes) + " minutes"

	// freshTailChunkWindow is freshTailCut restated one bucket looser. See
	// freshTailChunkBound.
	freshTailChunkWindow = strconv.Itoa(freshScanTailMinutes+caggBucketMinutes) + " minutes"
)

// freshTailCut is the bucket boundary separating the two sources wherever they
// must not overlap. Bucket-aligned is what makes the split exact rather than
// approximate: a CAGG bucket strictly below it aggregates only raw rows strictly
// below it, and every raw row at or above it belongs to a bucket at or above it.
//
// now() is the transaction timestamp, so every occurrence within one statement
// resolves to the same instant and the two sides cannot drift apart mid-query.
//
// Do not add this bound to the CAGG side of latest_scans. Measured on preview
// (500k buckets, ~14k per asset), `bucket < <cut>` on the un-correlated
// whole-org roll-up loses the index and drops PostgreSQL onto a parallel seq
// scan of every chunk. On the per-asset dwell probes, where asset_id supplies
// the leading equality, it stays an index scan and is required for correctness.
var freshTailCut = `time_bucket(INTERVAL '` + strconv.Itoa(caggBucketMinutes) + ` minute', now() - INTERVAL '` + freshScanTailWindow + `')`

// freshTailChunkBound restates freshTailCut one bucket looser, purely so the
// planner can exclude chunks. It is logically redundant — bucket-truncation moves
// an instant backwards by strictly less than one bucket, so every row satisfying
// the cut satisfies this too — and it selects no row the cut would reject,
// because the cut is applied alongside it.
//
// It exists because TimescaleDB's plan-time `now()` constification only
// recognises a bare `now() - INTERVAL 'x'`; wrapped in time_bucket() the bound
// becomes opaque, chunk exclusion falls back to execution start, and the planner
// has to plan a ChunkAppend across every asset_scans chunk. Measured on preview
// (16.5M rows), stating this extra bound took planning from 88ms to 16ms.
//
// If the cut ever stops being a bucket-truncation of a now()-relative instant,
// re-derive this rather than nudging the interval.
var freshTailChunkBound = `now() - INTERVAL '` + freshTailChunkWindow + `'`

// freshTailCTE renders the `fresh_tail` CTE body: the not-yet-materialized tail
// read straight off the raw hypertable, re-deriving the CAGG's own expression
// (time_bucket 1 minute, last(location_id, timestamp), max(timestamp)) so both
// sources carry the same shape and the same answers. $1 is org_id.
//
// MATERIALIZED at the call site, and that is a planning decision as much as an
// execution one. The tail is a handful of rows, but asset_scans is a large
// hypertable whose ChunkAppend cannot be pruned at plan time — the bound is
// now()-relative, so chunk exclusion happens at execution start. Every inlined
// reference would therefore re-plan across every chunk; materializing plans it
// once. On preview that is the difference between 80ms and 15ms of planning.
//
// This read runs inside WithOrgTx and is therefore subject to the asset_scans
// org-isolation policy (TRA-875), which is wanted; the explicit org_id predicate
// is what additionally lets idx_asset_scans_org_time bound the scan to the newest
// chunk. It must stay GROUP BY + last(): a DISTINCT ON over asset_scans is what
// tripped the TimescaleDB SkipScan bug that XX000-crashed preview in TRA-1021,
// and TRA-1022 moved to the CAGG specifically to escape it. Local tests will not
// catch a SkipScan regression — verify on preview.
func freshTailCTE() string {
	return `
		SELECT
			s.asset_id,
			time_bucket(INTERVAL '1 minute', s.timestamp) AS bucket,
			last(s.location_id, s.timestamp)              AS location_id,
			max(s.timestamp)                              AS last_seen
		FROM trakrf.asset_scans s
		WHERE s.org_id = $1
		  AND s.timestamp >= ` + freshTailChunkBound + `
		  AND s.timestamp >= ` + freshTailCut + `
		GROUP BY s.asset_id, 2
	`
}

// latestScansCTE renders the `latest_scans` CTE body: exactly one row per asset,
// carrying its most recent location and last_seen. Feeds both the list query's
// page CTE and the count query, which must agree or pagination reports a total
// its own rows cannot reach.
//
// Each side rolls up to per-asset independently before they meet, rather than
// unioning at bucket granularity and rolling up once. The shapes are equivalent —
// last()/max() over a partition of the rows is the same as over all of them — but
// the per-side form leaves the CAGG aggregation byte-identical to what it was
// before TRA-1117 instead of feeding a hash aggregate over an Append.
//
// The two sides are allowed to OVERLAP here, unlike everywhere else, and that is
// what lets the CAGG side keep its index-friendly plan (see freshTailCut). The
// overlap is harmless because this roll-up only asks for the newest observation:
// the CAGG derives from the same raw rows, so for any bucket it holds a last_seen
// that is at best equal to the tail's and never later. A tie resolves to the same
// underlying scan and therefore the same location. The tail always wins or draws;
// it can never be overruled by a staler copy of itself.
//
// org_id is filtered explicitly because RLS does not extend to a CAGG at all.
func latestScansCTE() string {
	return `
		SELECT
			asset_id,
			last(location_id, last_seen) AS location_id,
			max(last_seen)               AS last_seen
		FROM (
			SELECT
				asset_id,
				last(location_id, last_seen) AS location_id,
				max(last_seen)               AS last_seen
			FROM trakrf.asset_scan_latest
			WHERE org_id = $1
			GROUP BY asset_id
			UNION ALL
			SELECT
				asset_id,
				last(location_id, last_seen) AS location_id,
				max(last_seen)               AS last_seen
			FROM fresh_tail
			GROUP BY asset_id
		) per_source
		GROUP BY asset_id
	`
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

// ListCurrentLocations returns paginated current asset locations.
//
// Latest-scan-per-asset is resolved by the latest_scans CTE (TRA-1022,
// TRA-1117): last(location_id)/max(timestamp) taken from the asset_scan_latest
// continuous aggregate for settled history and from a bounded tail of raw
// asset_scans for the part the CAGG has not materialized yet, so a just-written
// scan is on the report immediately. This replaces the DISTINCT ON over the
// asset_scans hypertable that TRA-1021 had to defuse with SkipScan-off. org_id
// is filtered explicitly because RLS does not extend to the CAGG.
func (s *Storage) ListCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) ([]report.CurrentLocationItem, error) {
	query := buildCurrentLocationsQuery(
		buildCurrentLocationsOrderBy(filter.Sorts, innerSortColumns),
		buildCurrentLocationsOrderBy(filter.Sorts, outerSortColumns),
	)

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
				&item.DwellStartedAt,
				&item.DwellSeconds,
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

// countCurrentLocationsQuery renders the count query. It shares fresh_tail and
// latest_scans verbatim with the list query (TRA-1022, TRA-1117) — the count
// paginates the rows the list returns, so it has to see the fresh tail too or
// total_count undershoots the moment anything is saved. It has no page CTE and no
// dwell LATERAL, so it never needs to resolve a run.
func countCurrentLocationsQuery() string {
	return `
		WITH fresh_tail AS MATERIALIZED (` + freshTailCTE() + `),
		latest_scans AS (` + latestScansCTE() + `)
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
}

// CountCurrentLocations returns total count for pagination
func (s *Storage) CountCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) (int, error) {
	query := countCurrentLocationsQuery()

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

// currentLocationsSortColumns names the SQL columns the documented sort enum
// resolves to for one rendering of the current-locations query. The query is
// rendered twice (TRA-1023): once inside the `page` CTE, over the joined base
// relations, and once outside it, over the CTE's already-aliased output. Both
// orderings must agree or pagination stops being deterministic.
type currentLocationsSortColumns struct {
	lastSeen            string
	assetExternalKey    string
	locationExternalKey string
	assetID             string // stable tiebreaker
}

var (
	// innerSortColumns addresses the base relations joined in the page CTE.
	innerSortColumns = currentLocationsSortColumns{
		lastSeen:            "ls.last_seen",
		assetExternalKey:    "a.external_key",
		locationExternalKey: "l.external_key",
		assetID:             "a.id",
	}
	// outerSortColumns addresses the page CTE's output aliases.
	outerSortColumns = currentLocationsSortColumns{
		lastSeen:            "p.last_seen",
		assetExternalKey:    "p.asset_external_key",
		locationExternalKey: "p.location_external_key",
		assetID:             "p.asset_id",
	}
)

// buildCurrentLocationsOrderBy resolves the documented sort enum
// (asset_last_seen, asset_external_key, location_external_key) into a SQL
// ORDER BY fragment against the supplied column set. Default order — when
// no sort is supplied — is most-recent-first by last_seen, with a stable
// tiebreaker on asset id so pagination is deterministic across pages.
//
// The wire-level sort key is `asset_last_seen` per TRA-717 / BB34 F2; the
// underlying storage column is still `last_seen` on the latest-scan
// materialization (TRA-641 / BB21 §2.6 carried over). "no prefix means
// ASC" per the public API convention.
//
// Dwell is deliberately NOT sortable (TRA-1023): sorting on it would force the
// dwell LATERAL to run for every asset in the org before the LIMIT could be
// applied, which is exactly the cost the page CTE exists to avoid.
func buildCurrentLocationsOrderBy(sorts []report.CurrentLocationSort, cols currentLocationsSortColumns) string {
	defaultOrder := cols.lastSeen + " DESC, " + cols.assetID + " ASC"
	if len(sorts) == 0 {
		return defaultOrder
	}
	out := make([]string, 0, len(sorts))
	for _, s := range sorts {
		var col string
		switch s.Field {
		case "asset_last_seen":
			col = cols.lastSeen
		case "asset_external_key":
			col = cols.assetExternalKey
		case "location_external_key":
			col = cols.locationExternalKey
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

// buildCurrentLocationsQuery renders the list query. The latest_scans CTE
// resolves one row per asset (last(location_id) by newest observation,
// max(last_seen)) across the CAGG and the fresh_tail CTE. The joins,
// temporal-validity predicates, filters, sort and pagination are unchanged from
// the pre-CAGG query — they now just live inside the `page` CTE.
//
// TRA-1023 adds dwell. The dwell LATERAL hangs off `page`, NOT off the joined
// relations, and `page` is MATERIALIZED on purpose: dwell must be computed for
// the <=200 rows the caller actually receives, never for every asset in the
// org. Inlining the CTE would still leave the Limit node as a barrier, but
// spelling it MATERIALIZED makes the cost contract explicit and unbreakable by
// a future planner change.
//
// The dwell LATERALs must see the fresh tail as well as the CAGG: a move
// recorded only in the tail would otherwise leave the walk-back unable to find
// any bucket at the new location, and dwell would come back NULL on exactly the
// row the user just created (TRA-1117).
//
// Resolving one asset's dwell is two steps:
//
//  1. `w` — the newest bucket whose location_id IS DISTINCT FROM the asset's
//     current location: the last time it was somewhere else. NULL when it has
//     never been anywhere else, which COALESCEs to -infinity so step 2 falls
//     through to the asset's first bucket ever.
//  2. `d` — the oldest bucket strictly newer than that: the first bucket of the
//     current run. Its last_seen is dwell_started_at.
//
// Each step asks the tail first and the CAGG second, COALESCEd, rather than
// asking a union of the two. That works because the sources are disjoint at
// freshTailCut, so for step 1 any tail answer is automatically the newer one and
// for step 2 any CAGG answer is automatically the older one — the COALESCE order
// IS the ordering. It is not merely tidier: a single ORDER BY / max() over the
// union cannot use the per-chunk index the way a single-source probe can, and
// measured on preview that difference was 41ms per row against ~1ms. Both probes
// as written are the same index-ordered LIMIT 1 the query used before TRA-1117.
//
// This is the one place the CAGG must carry `bucket < <cut>`: unlike the
// latest_scans roll-up, run detection is corrupted by a bucket the CAGG
// materialized while it was still filling — a stale last(location_id) reads as a
// genuine visit elsewhere and truncates the run. Here asset_id supplies the
// leading index equality, so the bound costs nothing.
//
// IS DISTINCT FROM (rather than <>) is load-bearing: a scan that resolved to no
// location is a distinct state, so a NULL-location bucket breaks the run in
// both directions and an asset whose current location is NULL still resolves.
//
// dwell_started_at is the run-start bucket's last_seen rather than its bucket
// start because the CAGG only keeps last(location_id) per 1-minute bucket: on
// the boundary bucket the asset may have been at its PREVIOUS location earlier
// in that minute. last_seen is a timestamp at which it was definitely already
// here — conservative, never early, never off by more than one bucket.
//
// Note dwell describes the underlying scan run and is emitted even when the
// location entity is nulled out of the row projection by soft-delete or
// temporal validity — same treatment as asset_last_seen. That is why the
// LATERAL correlates on ls.location_id (carried through the CTE as
// scan_location_id), not on the projected l.id.
func buildCurrentLocationsQuery(innerOrderBy, outerOrderBy string) string {
	return `
		WITH fresh_tail AS MATERIALIZED (` + freshTailCTE() + `),
		latest_scans AS (` + latestScansCTE() + `),
		page AS MATERIALIZED (
			SELECT
				a.id            AS asset_id,
				a.name          AS asset_name,
				a.external_key  AS asset_external_key,
				l.id            AS location_id,
				l.name          AS location_name,
				l.external_key  AS location_external_key,
				ls.last_seen,
				a.deleted_at    AS asset_deleted_at,
				ls.location_id  AS scan_location_id
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
			ORDER BY ` + innerOrderBy + `
			LIMIT $5 OFFSET $6
		)
		SELECT
			p.asset_id,
			p.asset_name,
			p.asset_external_key,
			p.location_id,
			p.location_name,
			p.location_external_key,
			p.last_seen,
			p.asset_deleted_at,
			d.dwell_started_at,
			-- BIGINT for the same reason ListAssetHistory casts to it: a long
			-- enough span overflows EXTRACT(EPOCH ...)::INT (int4, SQLSTATE
			-- 22003). DwellSeconds is int64 Go-side, so it scans cleanly.
			EXTRACT(EPOCH FROM (p.last_seen - d.dwell_started_at))::BIGINT AS dwell_seconds
		FROM page p
		CROSS JOIN LATERAL (
			SELECT COALESCE(
				(SELECT max(t.bucket)
				 FROM fresh_tail t
				 WHERE t.asset_id = p.asset_id
				   AND t.location_id IS DISTINCT FROM p.scan_location_id),
				(SELECT max(c.bucket)
				 FROM trakrf.asset_scan_latest c
				 WHERE c.org_id = $1
				   AND c.asset_id = p.asset_id
				   AND c.bucket < ` + freshTailCut + `
				   AND c.location_id IS DISTINCT FROM p.scan_location_id),
				'-infinity'::timestamptz
			) AS run_after
		) w
		CROSS JOIN LATERAL (
			SELECT COALESCE(
				(SELECT c.last_seen
				 FROM trakrf.asset_scan_latest c
				 WHERE c.org_id = $1
				   AND c.asset_id = p.asset_id
				   AND c.bucket < ` + freshTailCut + `
				   AND c.bucket > w.run_after
				 ORDER BY c.bucket
				 LIMIT 1),
				(SELECT t.last_seen
				 FROM fresh_tail t
				 WHERE t.asset_id = p.asset_id
				   AND t.bucket > w.run_after
				 ORDER BY t.bucket
				 LIMIT 1)
			) AS dwell_started_at
		) d
		ORDER BY ` + outerOrderBy + `
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
