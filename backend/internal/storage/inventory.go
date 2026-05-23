package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SaveInventoryRequest represents the request to save inventory scans
type SaveInventoryRequest struct {
	LocationID int
	AssetIDs   []int
}

// SaveInventoryResult represents the result of saving inventory scans
type SaveInventoryResult struct {
	Count        int       `json:"count"`
	LocationID   int       `json:"location_id"`
	LocationName string    `json:"location_name"`
	Timestamp    time.Time `json:"timestamp"`
}

// InventoryAccessError provides diagnostic context for 403 responses.
//
// OrgID, LocationID, AssetIDs, and the per-bucket ID lists are internal-only:
// used for structured logging in the handler and must never appear in Error()
// output or be serialised to the wire. The json:"-" tags enforce this at the
// type level — Error() handles the human-readable side. (TRA-547)
//
// Asset failures are bucketed by why each ID failed so the surface message
// names a real cause instead of asserting "org mismatch" for everything that
// trips the validation count (TRA-812 — the same 403 used to fire for
// duplicates, soft-deleted IDs, and nonexistent IDs as well as genuine
// cross-org, and the diagnostic was misleading in three of those four).
type InventoryAccessError struct {
	Reason     string // "location" or "assets"
	OrgID      int    `json:"-"`
	LocationID int    `json:"-"`
	AssetIDs   []int  `json:"-"`
	ValidCount int
	TotalCount int

	// Per-bucket breakdowns of which AssetIDs failed and why. Populated only
	// for Reason == "assets". For logging; never serialised to the wire.
	MissingAssetIDs     []int `json:"-"`
	SoftDeletedAssetIDs []int `json:"-"`
	CrossOrgAssetIDs    []int `json:"-"`
}

func (e *InventoryAccessError) Error() string {
	switch e.Reason {
	case "location":
		return "location not found or access denied"
	case "assets":
		// User-facing surface stays generic — listing missing vs. cross-org
		// counts would let a caller probe other orgs by ID. Diagnostic detail
		// goes to the handler log via the typed fields above.
		return fmt.Sprintf("%d of %d assets are unavailable; refresh and try again",
			e.TotalCount-e.ValidCount, e.TotalCount)
	default:
		return "access denied"
	}
}

func (e *InventoryAccessError) IsAccessDenied() bool {
	return true
}

// SaveInventoryScans persists scanned assets to the asset_scans hypertable.
// It validates that both the location and all assets belong to the specified
// org, then batch inserts records — all within a single WithOrgTx transaction
// so RLS is active for the validation reads and the TOCTOU gap is eliminated.
//
// Duplicate asset IDs in req.AssetIDs are deduped before validation and insert.
// Multiple RFID tags can point to the same asset (multi-tag asset support), so
// a single scan session naturally produces duplicates in the public-API
// payload (TRA-812). Without deduping, the validation count comparison —
// previously `COUNT(*) WHERE id = ANY(...)` (semi-join, deduped by Postgres)
// vs. `len(req.AssetIDs)` — under-counted vs. input and tripped the
// access-denied guard for a payload that was actually well-formed, AND the
// per-ID insert loop wrote redundant scan rows.
//
// On asset-validation failure the error names a real cause: each failing ID
// is bucketed as missing, soft-deleted, or cross-org, and the bucket lists go
// to the handler log. The user-facing surface stays generic ("N of M assets
// are unavailable") so callers cannot probe other orgs by ID.
func (s *Storage) SaveInventoryScans(ctx context.Context, orgID int, req SaveInventoryRequest) (*SaveInventoryResult, error) {
	if len(req.AssetIDs) == 0 {
		return nil, fmt.Errorf("no assets to save")
	}

	uniqueAssetIDs := dedupInts(req.AssetIDs)

	var locationName string
	timestamp := time.Now()

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		// 1. Validate location belongs to org and get its name
		err := tx.QueryRow(ctx, `SELECT name FROM trakrf.locations WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, req.LocationID, orgID).Scan(&locationName)
		if err != nil {
			if err == pgx.ErrNoRows {
				return &InventoryAccessError{
					Reason:     "location",
					OrgID:      orgID,
					LocationID: req.LocationID,
				}
			}
			return fmt.Errorf("failed to validate location: %w", err)
		}

		// 2. Classify every unique asset ID. The previous validation was a
		// single COUNT(*) check that fired the same "access denied" for four
		// different failure modes (duplicates, soft-deleted, cross-org,
		// nonexistent), so the diagnostic was wrong in three of four. Now we
		// query each ID's org_id and deleted_at and bucket the misses by
		// cause.
		rows, err := tx.Query(ctx, `
			SELECT id, org_id, (deleted_at IS NOT NULL) AS is_deleted
			FROM trakrf.assets
			WHERE id = ANY($1)
		`, uniqueAssetIDs)
		if err != nil {
			return fmt.Errorf("failed to validate assets: %w", err)
		}
		type assetRow struct {
			id        int
			orgID     int
			isDeleted bool
		}
		foundByID := make(map[int]assetRow, len(uniqueAssetIDs))
		for rows.Next() {
			var r assetRow
			if err := rows.Scan(&r.id, &r.orgID, &r.isDeleted); err != nil {
				rows.Close()
				return fmt.Errorf("scan asset validation row: %w", err)
			}
			foundByID[r.id] = r
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate asset validation rows: %w", err)
		}
		rows.Close()

		var missing, softDeleted, crossOrg []int
		for _, id := range uniqueAssetIDs {
			r, ok := foundByID[id]
			switch {
			case !ok:
				missing = append(missing, id)
			case r.orgID != orgID:
				crossOrg = append(crossOrg, id)
			case r.isDeleted:
				softDeleted = append(softDeleted, id)
			}
		}
		invalidCount := len(missing) + len(softDeleted) + len(crossOrg)
		if invalidCount > 0 {
			return &InventoryAccessError{
				Reason:              "assets",
				OrgID:               orgID,
				AssetIDs:            uniqueAssetIDs,
				ValidCount:          len(uniqueAssetIDs) - invalidCount,
				TotalCount:          len(uniqueAssetIDs),
				MissingAssetIDs:     missing,
				SoftDeletedAssetIDs: softDeleted,
				CrossOrgAssetIDs:    crossOrg,
			}
		}

		// 3. Batch INSERT into asset_scans — one row per unique asset
		insertQuery := `INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id) VALUES ($1, $2, $3, $4, NULL, NULL)`
		for _, assetID := range uniqueAssetIDs {
			if _, err := tx.Exec(ctx, insertQuery, timestamp, orgID, assetID, req.LocationID); err != nil {
				return fmt.Errorf("failed to insert asset scan for asset %d: %w", assetID, err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &SaveInventoryResult{
		Count:        len(uniqueAssetIDs),
		LocationID:   req.LocationID,
		LocationName: locationName,
		Timestamp:    timestamp,
	}, nil
}

// dedupInts returns input with duplicates removed, first-seen order preserved.
// Used by SaveInventoryScans to handle multi-tag-per-asset scans where the
// public-API payload naturally produces duplicate asset IDs (TRA-812).
func dedupInts(in []int) []int {
	out := make([]int, 0, len(in))
	seen := make(map[int]struct{}, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
