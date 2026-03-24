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
type InventoryAccessError struct {
	Reason     string // "location" or "assets"
	OrgID      int
	LocationID int
	AssetIDs   []int
	ValidCount int
	TotalCount int
}

func (e *InventoryAccessError) Error() string {
	switch e.Reason {
	case "location":
		return fmt.Sprintf("location not found or access denied (org_id=%d, location_id=%d)", e.OrgID, e.LocationID)
	case "assets":
		return fmt.Sprintf("assets not found or access denied (org_id=%d, valid=%d/%d)", e.OrgID, e.ValidCount, e.TotalCount)
	default:
		return "access denied"
	}
}

func (e *InventoryAccessError) IsAccessDenied() bool {
	return true
}

// SaveInventoryScans persists scanned assets to the asset_scans hypertable.
// It validates that both the location and all assets belong to the specified org,
// then batch inserts records within a transaction.
func (s *Storage) SaveInventoryScans(ctx context.Context, orgID int, req SaveInventoryRequest) (*SaveInventoryResult, error) {
	if len(req.AssetIDs) == 0 {
		return nil, fmt.Errorf("no assets to save")
	}

	// 1. Validate location belongs to org and get its name
	var locationName string
	locationQuery := `
		SELECT name FROM trakrf.locations
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
	`
	err := s.pool.QueryRow(ctx, locationQuery, req.LocationID, orgID).Scan(&locationName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &InventoryAccessError{
				Reason:     "location",
				OrgID:      orgID,
				LocationID: req.LocationID,
			}
		}
		return nil, fmt.Errorf("failed to validate location: %w", err)
	}

	// 2. Validate all assets belong to org (batch query)
	assetCountQuery := `
		SELECT COUNT(*) FROM trakrf.assets
		WHERE id = ANY($1) AND org_id = $2 AND deleted_at IS NULL
	`
	var validAssetCount int
	err = s.pool.QueryRow(ctx, assetCountQuery, req.AssetIDs, orgID).Scan(&validAssetCount)
	if err != nil {
		return nil, fmt.Errorf("failed to validate assets: %w", err)
	}
	if validAssetCount != len(req.AssetIDs) {
		return nil, &InventoryAccessError{
			Reason:     "assets",
			OrgID:      orgID,
			AssetIDs:   req.AssetIDs,
			ValidCount: validAssetCount,
			TotalCount: len(req.AssetIDs),
		}
	}

	// 3. Begin transaction and batch INSERT into asset_scans
	timestamp := time.Now()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	insertQuery := `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, identifier_scan_id)
		VALUES ($1, $2, $3, $4, NULL, NULL)
	`

	for _, assetID := range req.AssetIDs {
		_, err = tx.Exec(ctx, insertQuery, timestamp, orgID, assetID, req.LocationID)
		if err != nil {
			return nil, fmt.Errorf("failed to insert asset scan for asset %d: %w", assetID, err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &SaveInventoryResult{
		Count:        len(req.AssetIDs),
		LocationID:   req.LocationID,
		LocationName: locationName,
		Timestamp:    timestamp,
	}, nil
}
