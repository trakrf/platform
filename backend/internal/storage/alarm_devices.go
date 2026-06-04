package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/alarmdevice"
)

// alarmDeviceColumns is the canonical SELECT/RETURNING column list, kept
// identical across every alarm_devices query so scan targets line up.
const alarmDeviceColumns = `id, org_id, name, type, base_url, switch_id,
	scan_point_id, is_active, metadata, created_at, updated_at, deleted_at`

func scanAlarmDevice(row pgx.Row, d *alarmdevice.AlarmDevice) error {
	return row.Scan(&d.ID, &d.OrgID, &d.Name, &d.Type, &d.BaseURL, &d.SwitchID,
		&d.ScanPointID, &d.IsActive, &d.Metadata, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt)
}

// CreateAlarmDevice inserts an alarm device. type defaults to shelly_gen4,
// switch_id to 0, is_active to true, metadata to {} when omitted.
func (s *Storage) CreateAlarmDevice(ctx context.Context, orgID int, req alarmdevice.CreateAlarmDeviceRequest) (*alarmdevice.AlarmDevice, error) {
	deviceType := req.Type
	if deviceType == "" {
		deviceType = alarmdevice.TypeShellyGen4
	}
	switchID := 0
	if req.SwitchID != nil {
		switchID = *req.SwitchID
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
		INSERT INTO trakrf.alarm_devices
		(org_id, name, type, base_url, switch_id, scan_point_id, is_active, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + alarmDeviceColumns

	var d alarmdevice.AlarmDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanAlarmDevice(tx.QueryRow(ctx, query, orgID, req.Name, deviceType, req.BaseURL,
			switchID, req.ScanPointID, isActive, metadata), &d)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create alarm device: %w", err)
	}
	return &d, nil
}

// GetAlarmDeviceByID returns the live alarm device or (nil, nil) if not found.
func (s *Storage) GetAlarmDeviceByID(ctx context.Context, orgID, id int) (*alarmdevice.AlarmDevice, error) {
	query := `SELECT ` + alarmDeviceColumns + `
		FROM trakrf.alarm_devices
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`
	var d alarmdevice.AlarmDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanAlarmDevice(tx.QueryRow(ctx, query, id, orgID), &d)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get alarm device: %w", err)
	}
	return &d, nil
}

// ListAlarmDevices returns live alarm devices for the org, newest first.
func (s *Storage) ListAlarmDevices(ctx context.Context, orgID, limit, offset int) ([]alarmdevice.AlarmDevice, error) {
	query := `SELECT ` + alarmDeviceColumns + `
		FROM trakrf.alarm_devices
		WHERE org_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	devices := []alarmdevice.AlarmDevice{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d alarmdevice.AlarmDevice
			if err := scanAlarmDevice(rows, &d); err != nil {
				return err
			}
			devices = append(devices, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list alarm devices: %w", err)
	}
	return devices, nil
}

// CountAlarmDevices returns the number of live alarm devices for the org.
func (s *Storage) CountAlarmDevices(ctx context.Context, orgID int) (int, error) {
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COUNT(*) FROM trakrf.alarm_devices WHERE org_id = $1 AND deleted_at IS NULL`, orgID).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count alarm devices: %w", err)
	}
	return n, nil
}

// ListAlarmDevicesForScanPoint returns active, non-deleted devices bound to the
// given scan point. Used by the geofence firer.
func (s *Storage) ListAlarmDevicesForScanPoint(ctx context.Context, orgID, scanPointID int) ([]alarmdevice.AlarmDevice, error) {
	query := `SELECT ` + alarmDeviceColumns + `
		FROM trakrf.alarm_devices
		WHERE org_id = $1 AND scan_point_id = $2 AND is_active = true AND deleted_at IS NULL
		ORDER BY id`
	out := []alarmdevice.AlarmDevice{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, scanPointID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d alarmdevice.AlarmDevice
			if err := scanAlarmDevice(rows, &d); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list alarm devices for scan point: %w", err)
	}
	return out, nil
}

// UpdateAlarmDevice applies a partial update. Returns (nil, nil) if no live
// device with that id exists for the org.
func (s *Storage) UpdateAlarmDevice(ctx context.Context, orgID, id int, req alarmdevice.UpdateAlarmDeviceRequest) (*alarmdevice.AlarmDevice, error) {
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
	if req.BaseURL != nil {
		add("base_url", *req.BaseURL)
	}
	if req.SwitchID != nil {
		add("switch_id", *req.SwitchID)
	}
	if req.ScanPointID != nil {
		add("scan_point_id", *req.ScanPointID)
	}
	if req.IsActive != nil {
		add("is_active", *req.IsActive)
	}
	if req.Metadata != nil {
		add("metadata", *req.Metadata)
	}

	// Nothing to update: return the current row (or nil if missing).
	if len(setClauses) == 0 {
		return s.GetAlarmDeviceByID(ctx, orgID, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf(`
		UPDATE trakrf.alarm_devices
		SET %s
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		RETURNING `+alarmDeviceColumns, strings.Join(setClauses, ", "))

	var d alarmdevice.AlarmDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanAlarmDevice(tx.QueryRow(ctx, query, args...), &d)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update alarm device: %w", err)
	}
	return &d, nil
}

// DeleteAlarmDevice soft-deletes the device. Returns false if no live device
// with that id existed.
func (s *Storage) DeleteAlarmDevice(ctx context.Context, orgID, id int) (bool, error) {
	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			UPDATE trakrf.alarm_devices
			   SET deleted_at = NOW()
			 WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("could not delete alarm device: %w", err)
	}
	return rowsAffected > 0, nil
}
