package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
)

// scanDeviceColumns is the canonical SELECT/RETURNING column list, kept
// identical across every scan_devices query so scan targets line up.
const scanDeviceColumns = `id, org_id, external_key, name, type, transport, publish_topic,
	serial_number, model, COALESCE(description, ''), metadata,
	valid_from, valid_to, is_active, created_at, updated_at, deleted_at`

func scanScanDevice(row pgx.Row, d *scandevice.ScanDevice) error {
	return row.Scan(&d.ID, &d.OrgID, &d.ExternalKey, &d.Name, &d.Type, &d.Transport, &d.PublishTopic,
		&d.SerialNumber, &d.Model, &d.Description, &d.Metadata,
		&d.ValidFrom, &d.ValidTo, &d.IsActive, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt)
}

// CreateScanDevice inserts a scan device. transport defaults to mqtt and
// publish_topic defaults to trakrf.id/{external_key}/reads when omitted.
func (s *Storage) CreateScanDevice(ctx context.Context, orgID int, req scandevice.CreateScanDeviceRequest) (*scandevice.ScanDevice, error) {
	transport := req.Transport
	if transport == "" {
		transport = scandevice.TransportMQTT
	}
	publishTopic := req.PublishTopic
	if publishTopic == nil {
		dt := scandevice.DefaultPublishTopic(req.ExternalKey)
		publishTopic = &dt
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	query := `
		INSERT INTO trakrf.scan_devices
		(org_id, external_key, name, type, transport, publish_topic, serial_number, model, description, metadata, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING ` + scanDeviceColumns

	var d scandevice.ScanDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanScanDevice(tx.QueryRow(ctx, query, orgID, req.ExternalKey, req.Name, req.Type,
			transport, publishTopic, req.SerialNumber, req.Model, req.Description, metadata, isActive), &d)
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("scan device with external_key %s already exists", req.ExternalKey)
		}
		return nil, fmt.Errorf("failed to create scan device: %w", err)
	}
	return &d, nil
}

// GetScanDeviceByID returns the live scan device or (nil, nil) if not found.
func (s *Storage) GetScanDeviceByID(ctx context.Context, orgID, id int) (*scandevice.ScanDevice, error) {
	query := `SELECT ` + scanDeviceColumns + `
		FROM trakrf.scan_devices
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`
	var d scandevice.ScanDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanScanDevice(tx.QueryRow(ctx, query, id, orgID), &d)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get scan device: %w", err)
	}
	return &d, nil
}

// ListScanDevices returns live scan devices for the org, newest first.
func (s *Storage) ListScanDevices(ctx context.Context, orgID, limit, offset int) ([]scandevice.ScanDevice, error) {
	query := `SELECT ` + scanDeviceColumns + `
		FROM trakrf.scan_devices
		WHERE org_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	devices := []scandevice.ScanDevice{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d scandevice.ScanDevice
			if err := scanScanDevice(rows, &d); err != nil {
				return err
			}
			devices = append(devices, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list scan devices: %w", err)
	}
	return devices, nil
}

// CountScanDevices counts live scan devices for the org.
func (s *Storage) CountScanDevices(ctx context.Context, orgID int) (int, error) {
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COUNT(*) FROM trakrf.scan_devices WHERE org_id = $1 AND deleted_at IS NULL`, orgID).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count scan devices: %w", err)
	}
	return n, nil
}

// UpdateScanDevice applies a partial update and returns the updated device, or
// (nil, nil) if no live device with that id exists for the org.
func (s *Storage) UpdateScanDevice(ctx context.Context, orgID, id int, req scandevice.UpdateScanDeviceRequest) (*scandevice.ScanDevice, error) {
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
	if req.Type != nil {
		add("type", *req.Type)
	}
	if req.Transport != nil {
		add("transport", *req.Transport)
	}
	if req.PublishTopic != nil {
		add("publish_topic", *req.PublishTopic)
	}
	if req.SerialNumber != nil {
		add("serial_number", *req.SerialNumber)
	}
	if req.Model != nil {
		add("model", *req.Model)
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
	// Always advance updated_at (filesystem touch semantics, matches assets/locations).
	setClauses = append(setClauses, "updated_at = NOW()")

	query := fmt.Sprintf(`
		UPDATE trakrf.scan_devices
		SET %s
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		RETURNING `+scanDeviceColumns, strings.Join(setClauses, ", "))

	var d scandevice.ScanDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanScanDevice(tx.QueryRow(ctx, query, args...), &d)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("scan device publish_topic already in use")
		}
		return nil, fmt.Errorf("failed to update scan device: %w", err)
	}
	return &d, nil
}

// DeleteScanDevice soft-deletes the device and cascades the soft-delete to its
// scan points. Returns false if no live device with that id existed.
func (s *Storage) DeleteScanDevice(ctx context.Context, orgID, id int) (bool, error) {
	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			UPDATE trakrf.scan_devices
			   SET deleted_at = NOW()
			 WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		if rowsAffected == 0 {
			return nil
		}
		_, err = tx.Exec(ctx, `
			UPDATE trakrf.scan_points
			   SET deleted_at = NOW()
			 WHERE scan_device_id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("could not delete scan device: %w", err)
	}
	return rowsAffected > 0, nil
}
