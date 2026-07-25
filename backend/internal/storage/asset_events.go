package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PreviousLocation is an asset's last recorded sighting strictly before a given
// instant. LocationID is nil when that sighting carried no location — a real
// observation, distinct from "never seen", which is represented by the asset
// being absent from the result map entirely.
type PreviousLocation struct {
	LocationID *int
	LastSeen   time.Time
}

// AssetIdentity is the logical (wire-safe) identity of an asset: the obfuscated
// surrogate id, the natural key, and the display name. No EPC, no tag id.
type AssetIdentity struct {
	ID          int
	ExternalKey string
	Name        string
}

// prevFromCAGG reads the deep history from the asset_scan_latest continuous
// aggregate (TRA-1022). The CAGG is chunked at 10 days against the base table's
// 1 day, which is what makes this plan in ~6 ms instead of ~66 ms: planning
// cost is driven by chunk count, and the base table carries ~147 chunks.
//
// LATERAL ... ORDER BY ... LIMIT 1, never DISTINCT ON: a bare DISTINCT ON over
// asset_scans XX000-crashes under RLS via SkipScan (TRA-1021/1022).
//
// RLS does not extend to continuous aggregates, so `c.org_id = $1` IS the tenant
// boundary here and is load-bearing, not decorative.
//
// Both bucket and last_seen are bounded by $3. The bucket bound is what allows
// chunk exclusion; the last_seen bound keeps a bucket that also contains the
// scan we are evaluating from reporting that scan as its own predecessor.
const prevFromCAGG = `
SELECT a.asset_id, prev.location_id, prev.last_seen
FROM unnest($2::bigint[]) AS a(asset_id)
LEFT JOIN LATERAL (
  SELECT c.location_id, c.last_seen
  FROM trakrf.asset_scan_latest c
  WHERE c.org_id = $1 AND c.asset_id = a.asset_id
    AND c.bucket < $3 AND c.last_seen < $3
  ORDER BY c.bucket DESC
  LIMIT 1
) prev ON true`

// prevFromTail covers the CAGG's materialization lag (~1-1.5 min: end_offset 1
// minute, schedule_interval 30s, materialized_only). That lag is not a corner
// case — an asset moving between two doorways 20 s apart is the normal
// fixed-reader scenario, and the CAGG alone would report the location from
// before the previous move. Read alone it would invent moves and, worse, MISS
// them: a stale origin that happens to equal the new location suppresses a real
// event, silently.
//
// The constant lower bound is what keeps this cheap. `timestamp < now()` with no
// lower bound forces the planner to consider every chunk (measured at 66 ms of
// planning); a constant-expression lower bound enables plan-time chunk
// exclusion, so this touches ~1-2 chunks.
const prevFromTail = `
SELECT a.asset_id, prev.location_id, prev.timestamp
FROM unnest($2::bigint[]) AS a(asset_id)
LEFT JOIN LATERAL (
  SELECT s.location_id, s.timestamp
  FROM trakrf.asset_scans s
  WHERE s.org_id = $1 AND s.asset_id = a.asset_id
    AND s.timestamp < $3
    AND s.timestamp > now() - INTERVAL '10 minutes'
  ORDER BY s.timestamp DESC
  LIMIT 1
) prev ON true`

// PreviousAssetLocations returns, for each asset id, its last recorded sighting
// strictly before `before`. Assets with no prior sighting are absent from the
// map — that is the "genuine first-ever sighting" signal, and it is distinct
// from a sighting whose LocationID is nil.
//
// The answer is assembled from two bounded reads (deep history from the CAGG,
// recent history from the base table) with the later timestamp winning. Neither
// source alone is correct: the CAGG cannot see the last ~90 seconds, and a
// bounded base-table read cannot see further back than its constant lower bound.
//
// Both run inside one WithOrgTx: the base-table read needs the org GUC for RLS,
// and holding them in one transaction gives both a single consistent snapshot.
func (s *Storage) PreviousAssetLocations(ctx context.Context, orgID int, assetIDs []int, before time.Time) (map[int]PreviousLocation, error) {
	out := make(map[int]PreviousLocation, len(assetIDs))
	if len(assetIDs) == 0 {
		return out, nil
	}

	ids := make([]int64, 0, len(assetIDs))
	for _, id := range assetIDs {
		ids = append(ids, int64(id))
	}

	collect := func(tx pgx.Tx, query string) error {
		rows, err := tx.Query(ctx, query, orgID, ids, before)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var assetID int64
			var locationID *int
			var seen *time.Time
			if err := rows.Scan(&assetID, &locationID, &seen); err != nil {
				return err
			}
			if seen == nil {
				// LEFT JOIN LATERAL miss: no sighting from this source.
				continue
			}
			cur, ok := out[int(assetID)]
			if ok && !seen.After(cur.LastSeen) {
				continue
			}
			out[int(assetID)] = PreviousLocation{LocationID: locationID, LastSeen: *seen}
		}
		return rows.Err()
	}

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		if err := collect(tx, prevFromCAGG); err != nil {
			return fmt.Errorf("cagg lookup: %w", err)
		}
		if err := collect(tx, prevFromTail); err != nil {
			return fmt.Errorf("base tail lookup: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to look up previous asset locations: %w", err)
	}
	return out, nil
}

// LookupMoveNames resolves the display identity of the assets and locations
// named by a surviving move. It runs only for assets that actually changed
// location, so a no-op rescan never pays for it.
//
// Ids that name no live row of this org are simply absent from the returned
// maps; the caller drops such an event rather than sending an empty name.
func (s *Storage) LookupMoveNames(ctx context.Context, orgID int, assetIDs, locationIDs []int) (map[int]AssetIdentity, map[int]string, error) {
	assets := make(map[int]AssetIdentity, len(assetIDs))
	locations := make(map[int]string, len(locationIDs))
	if len(assetIDs) == 0 && len(locationIDs) == 0 {
		return assets, locations, nil
	}

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		if len(assetIDs) > 0 {
			rows, err := tx.Query(ctx,
				`SELECT id, external_key, name FROM trakrf.assets
				 WHERE org_id = $1 AND id = ANY($2) AND deleted_at IS NULL`,
				orgID, assetIDs)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var a AssetIdentity
				if err := rows.Scan(&a.ID, &a.ExternalKey, &a.Name); err != nil {
					return err
				}
				assets[a.ID] = a
			}
			if err := rows.Err(); err != nil {
				return err
			}
		}

		if len(locationIDs) > 0 {
			rows, err := tx.Query(ctx,
				`SELECT id, name FROM trakrf.locations
				 WHERE org_id = $1 AND id = ANY($2) AND deleted_at IS NULL`,
				orgID, locationIDs)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id int
				var name string
				if err := rows.Scan(&id, &name); err != nil {
					return err
				}
				locations[id] = name
			}
			if err := rows.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to look up asset/location names: %w", err)
	}
	return assets, locations, nil
}
