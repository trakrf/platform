package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/scanpoint"
)

const scanPointColumns = `id, org_id, scan_device_id, location_id, external_key, name, antenna_port,
	is_boundary, COALESCE(description, ''), metadata,
	valid_from, valid_to, is_active, created_at, updated_at, deleted_at`

func scanScanPoint(row pgx.Row, p *scanpoint.ScanPoint) error {
	return row.Scan(&p.ID, &p.OrgID, &p.ScanDeviceID, &p.LocationID, &p.ExternalKey, &p.Name, &p.AntennaPort,
		&p.IsBoundary, &p.Description, &p.Metadata,
		&p.ValidFrom, &p.ValidTo, &p.IsActive, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
}

// CreateScanPoint inserts a scan point under the given device. is_boundary
// defaults to false. The FK to scan_devices (+ RLS) enforces that the device
// exists and belongs to the org.
func (s *Storage) CreateScanPoint(ctx context.Context, orgID, scanDeviceID int, req scanpoint.CreateScanPointRequest) (*scanpoint.ScanPoint, error) {
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	isBoundary := false
	if req.IsBoundary != nil {
		isBoundary = *req.IsBoundary
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	query := `
		INSERT INTO trakrf.scan_points
		(org_id, scan_device_id, location_id, external_key, name, antenna_port, is_boundary, description, metadata, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING ` + scanPointColumns

	var p scanpoint.ScanPoint
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanScanPoint(tx.QueryRow(ctx, query, orgID, scanDeviceID, req.LocationID, req.ExternalKey,
			req.Name, req.AntennaPort, isBoundary, req.Description, metadata, isActive), &p)
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("scan point with external_key %s already exists", req.ExternalKey)
		}
		if strings.Contains(err.Error(), "scan_device_id_fkey") {
			return nil, fmt.Errorf("invalid scan_device_id: device does not exist")
		}
		if strings.Contains(err.Error(), "location_id_fkey") {
			return nil, fmt.Errorf("invalid location_id: location does not exist")
		}
		return nil, fmt.Errorf("failed to create scan point: %w", err)
	}
	return &p, nil
}

// GetScanPointByID returns the live scan point or (nil, nil) if not found.
func (s *Storage) GetScanPointByID(ctx context.Context, orgID, id int) (*scanpoint.ScanPoint, error) {
	query := `SELECT ` + scanPointColumns + `
		FROM trakrf.scan_points
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`
	var p scanpoint.ScanPoint
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanScanPoint(tx.QueryRow(ctx, query, id, orgID), &p)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get scan point: %w", err)
	}
	return &p, nil
}

// ListScanPointsByDevice returns the live scan points for a device, oldest first.
func (s *Storage) ListScanPointsByDevice(ctx context.Context, orgID, scanDeviceID int) ([]scanpoint.ScanPoint, error) {
	query := `SELECT ` + scanPointColumns + `
		FROM trakrf.scan_points
		WHERE org_id = $1 AND scan_device_id = $2 AND deleted_at IS NULL
		ORDER BY antenna_port NULLS FIRST, created_at`
	points := []scanpoint.ScanPoint{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, scanDeviceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p scanpoint.ScanPoint
			if err := scanScanPoint(rows, &p); err != nil {
				return err
			}
			points = append(points, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list scan points: %w", err)
	}
	return points, nil
}

// UpdateScanPoint applies a partial update. ClearLocationID sets location_id to
// NULL. Returns (nil, nil) if no live point with that id exists for the org.
func (s *Storage) UpdateScanPoint(ctx context.Context, orgID, id int, req scanpoint.UpdateScanPointRequest) (*scanpoint.ScanPoint, error) {
	setClauses := []string{}
	args := []any{id, orgID}
	pos := 3
	add := func(col string, val any) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, pos))
		args = append(args, val)
		pos++
	}
	if req.Name != nil {
		add("name", *req.Name)
	}
	if req.ClearLocationID {
		setClauses = append(setClauses, "location_id = NULL")
	} else if req.LocationID != nil {
		add("location_id", *req.LocationID)
	}
	if req.AntennaPort != nil {
		add("antenna_port", *req.AntennaPort)
	}
	if req.IsBoundary != nil {
		add("is_boundary", *req.IsBoundary)
	}
	if req.Description != nil {
		add("description", *req.Description)
	}
	if req.Metadata != nil {
		add("metadata", *req.Metadata)
	}
	if req.IsActive != nil {
		add("is_active", *req.IsActive)
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	query := fmt.Sprintf(`
		UPDATE trakrf.scan_points
		SET %s
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		RETURNING `+scanPointColumns, strings.Join(setClauses, ", "))

	var p scanpoint.ScanPoint
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanScanPoint(tx.QueryRow(ctx, query, args...), &p)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "location_id_fkey") {
			return nil, fmt.Errorf("invalid location_id: location does not exist")
		}
		return nil, fmt.Errorf("failed to update scan point: %w", err)
	}
	return &p, nil
}

// DeleteScanPoint soft-deletes the point. Returns false if none existed.
func (s *Storage) DeleteScanPoint(ctx context.Context, orgID, id int) (bool, error) {
	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			UPDATE trakrf.scan_points
			   SET deleted_at = NOW()
			 WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("could not delete scan point: %w", err)
	}
	return rowsAffected > 0, nil
}
