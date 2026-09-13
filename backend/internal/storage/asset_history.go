package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/report"
)

// Asset history lists STAYS: one item per unbroken run of observations of an
// asset at one location. It is not a list of scans. A tag parked in front of a
// fixed reader writes a row every minute, so a per-scan history renders a parked
// asset as a timeline of identical one-minute entries and pages it 200 rows at a
// time.
//
// A stay is detected by gaps-and-islands over the asset_scan_latest continuous
// aggregate (one bucket per asset per minute) plus the not-yet-materialized raw
// tail, split at freshTailCut exactly as the asset-locations dwell does. Reading
// the aggregate rather than raw asset_scans is a cost decision, measured on
// preview against the soak rig's parked Squidget: over 90 days of history the
// raw window pass took 6.9s (4.3M rows, most written before scans were truncated
// to the minute) against 113ms for the whole request here. Post-truncation the
// two sources hold the same rows; before it, a run shorter than a minute is
// folded into its bucket, which is the granularity history is defined at.
//
// A scan that resolved to no location is its own state (IS DISTINCT FROM, not
// <>), so it breaks a stay in both directions — the same rule dwell uses.
//
// Every item describes its stay independently of the requested window; the
// window only decides which stays are listed (those observed inside it):
//
//   - event_observed_at is when the stay began, even if that is before `from`.
//     Only the oldest listed stay can have begun earlier, so it alone walks back.
//   - duration_seconds runs to the first observation somewhere else, even if
//     that is after `to`. It is null only when the asset has not been seen
//     anywhere else since — so a window ending in the past never manufactures an
//     ongoing stay. Only the newest listed stay can end past `to`.
//
// Silence inside a stay is not a break: an asset last read on Monday and next
// read at the same location on Friday has one stay spanning the week. Whether a
// long enough gap should end a stay is the same open age-out question dwell
// has, and it is deliberately not decided here.
//
// Both walks mirror the dwell probes' shape — each source asked separately and
// COALESCEd in the order that makes the answer the right one, because a single
// ORDER BY over the union cannot use the per-chunk index.
//
// Parameters, shared by the list and count queries: $1 asset_id, $2 org_id,
// $3 from, $4 to. The list adds $5 limit and $6 offset. org_id is filtered
// explicitly because RLS does not extend to a continuous aggregate.

// assetHistoryStaysCTEs renders the CTEs both history queries share: `tail`,
// `observed` (the minute buckets inside the window) and `starts` (the buckets
// that begin a stay, carrying each one's observation time).
func assetHistoryStaysCTEs() string {
	return `
		tail AS MATERIALIZED (
			SELECT
				time_bucket(INTERVAL '1 minute', s.timestamp) AS bucket,
				last(s.location_id, s.timestamp)              AS location_id,
				max(s.timestamp)                              AS last_seen
			FROM trakrf.asset_scans s
			WHERE s.asset_id = $1
			  AND s.org_id = $2
			  AND s.timestamp >= ` + freshTailChunkBound + `
			  AND s.timestamp >= ` + freshTailCut + `
			GROUP BY 1
		),
		observed AS (
			-- bucket > from - 1 minute is the indexable restatement of
			-- last_seen >= from: a bucket's last_seen is less than a minute past
			-- its start. The exact bound is applied below, after the union.
			SELECT c.bucket, c.location_id, c.last_seen
			FROM trakrf.asset_scan_latest c
			WHERE c.org_id = $2
			  AND c.asset_id = $1
			  AND c.bucket < ` + freshTailCut + `
			  AND ($3::timestamptz IS NULL OR c.bucket > $3::timestamptz - INTERVAL '1 minute')
			  AND ($4::timestamptz IS NULL OR c.bucket <= $4::timestamptz)
			UNION ALL
			SELECT t.bucket, t.location_id, t.last_seen FROM tail t
		),
		starts AS (
			SELECT bucket, location_id, last_seen
			FROM (
				SELECT
					bucket,
					location_id,
					last_seen,
					LAG(bucket)      OVER w AS prev_bucket,
					LAG(location_id) OVER w AS prev_location_id
				FROM observed
				WHERE ($3::timestamptz IS NULL OR last_seen >= $3::timestamptz)
				  AND ($4::timestamptz IS NULL OR last_seen <= $4::timestamptz)
				WINDOW w AS (ORDER BY bucket)
			) o
			WHERE prev_bucket IS NULL
			   OR location_id IS DISTINCT FROM prev_location_id
		)`
}

// buildAssetHistoryQuery renders the list query, paging stays in the given
// direction ("ASC" or "DESC").
func buildAssetHistoryQuery(dir string) string {
	return `
		WITH ` + assetHistoryStaysCTEs() + `,
		stays AS (
			SELECT
				bucket,
				location_id,
				last_seen                               AS started_at,
				LEAD(last_seen) OVER (ORDER BY bucket)  AS next_started_at,
				ROW_NUMBER()    OVER (ORDER BY bucket)  AS stay_no,
				COUNT(*)        OVER ()                 AS stay_count
			FROM starts
		),
		page AS MATERIALIZED (
			SELECT * FROM stays
			ORDER BY bucket ` + dir + `
			LIMIT $5 OFFSET $6
		),
		bounded AS (
			SELECT
				p.bucket,
				p.location_id,
				CASE WHEN p.stay_no = 1 AND $3::timestamptz IS NOT NULL THEN (
					-- The oldest listed stay may have begun before the window:
					-- find the last bucket anywhere else before it, then the
					-- first bucket after that. Aggregate first for the older
					-- answer, tail first for the newer.
					SELECT COALESCE(
						(SELECT c.last_seen
						 FROM trakrf.asset_scan_latest c
						 WHERE c.org_id = $2
						   AND c.asset_id = $1
						   AND c.bucket < ` + freshTailCut + `
						   AND c.bucket > w.stay_after
						 ORDER BY c.bucket
						 LIMIT 1),
						(SELECT t.last_seen FROM tail t
						 WHERE t.bucket > w.stay_after
						 ORDER BY t.bucket
						 LIMIT 1)
					)
					FROM (
						SELECT COALESCE(
							(SELECT max(t.bucket) FROM tail t
							 WHERE t.bucket < p.bucket
							   AND t.location_id IS DISTINCT FROM p.location_id),
							(SELECT max(c.bucket)
							 FROM trakrf.asset_scan_latest c
							 WHERE c.org_id = $2
							   AND c.asset_id = $1
							   AND c.bucket < ` + freshTailCut + `
							   AND c.bucket < p.bucket
							   AND c.location_id IS DISTINCT FROM p.location_id),
							'-infinity'::timestamptz
						) AS stay_after
					) w
				) ELSE p.started_at END AS started_at,
				CASE WHEN p.stay_no = p.stay_count AND $4::timestamptz IS NOT NULL THEN (
					-- The newest listed stay may have outlived the window: its
					-- end is the first bucket anywhere else after it.
					SELECT COALESCE(
						(SELECT c.last_seen
						 FROM trakrf.asset_scan_latest c
						 WHERE c.org_id = $2
						   AND c.asset_id = $1
						   AND c.bucket < ` + freshTailCut + `
						   AND c.bucket > p.bucket
						   AND c.location_id IS DISTINCT FROM p.location_id
						 ORDER BY c.bucket
						 LIMIT 1),
						(SELECT t.last_seen FROM tail t
						 WHERE t.bucket > p.bucket
						   AND t.location_id IS DISTINCT FROM p.location_id
						 ORDER BY t.bucket
						 LIMIT 1)
					)
				) ELSE p.next_started_at END AS next_started_at
			FROM page p
		)
		SELECT
			b.started_at,
			b.location_id,
			l.name         AS location_name,
			l.external_key AS location_external_key,
			-- Cast to BIGINT, not INT: a legitimate >68-year gap between two
			-- stays overflows EXTRACT(EPOCH ...)::INT (int4, SQLSTATE 22003).
			-- DurationSeconds is *int / 64-bit Go-side, so it scans cleanly.
			EXTRACT(EPOCH FROM (b.next_started_at - b.started_at))::BIGINT AS duration_seconds
		FROM bounded b
		LEFT JOIN trakrf.locations l ON l.id = b.location_id AND l.org_id = $2 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
		ORDER BY b.bucket ` + dir + `
	`
}

// countAssetHistoryQuery renders the count query: the number of stays observed
// inside the window, which is what the list pages over.
func countAssetHistoryQuery() string {
	return `
		WITH ` + assetHistoryStaysCTEs() + `
		SELECT COUNT(*) FROM starts
	`
}

// assetHistoryDirection resolves the ?sort= tokens into a paging direction.
// event_observed_at is the only sortable field; default is newest first.
func assetHistoryDirection(sorts []report.AssetHistorySort) string {
	for _, s := range sorts {
		if s.Field != "event_observed_at" {
			continue
		}
		if s.Desc {
			return "DESC"
		}
		return "ASC"
	}
	return "DESC"
}

// ListAssetHistory returns one page of an asset's stays.
func (s *Storage) ListAssetHistory(ctx context.Context, assetID, orgID int, filter report.AssetHistoryFilter) ([]report.AssetHistoryItem, error) {
	query := buildAssetHistoryQuery(assetHistoryDirection(filter.Sorts))

	// Run inside WithOrgTx so SET LOCAL app.current_org_id is in effect: the
	// tail reads asset_scans and the LEFT JOIN reads trakrf.locations, both under
	// org-isolation RLS policies that cast current_setting('app.current_org_id')
	// ::bigint. Querying on the raw pool leaves that setting unset and the policy
	// aborts the scan (SQLSTATE 22P02 / 42704) — a 500 on every asset with any
	// history. (TRA-865.)
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

// CountAssetHistory returns the number of stays, for pagination.
func (s *Storage) CountAssetHistory(ctx context.Context, assetID, orgID int, filter report.AssetHistoryFilter) (int, error) {
	// WithOrgTx for the same reason as ListAssetHistory: the tail reads
	// asset_scans, which carries its own org-isolation RLS policy (TRA-875).
	var count int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, countAssetHistoryQuery(), assetID, orgID, filter.From, filter.To).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count asset history: %w", err)
	}

	return count, nil
}
