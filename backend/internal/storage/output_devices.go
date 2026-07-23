package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
)

// outputDeviceColumns is the canonical SELECT/RETURNING column list, kept
// identical across every output_devices query so scan targets line up.
const outputDeviceColumns = `id, org_id, name, type, transport, base_url, switch_id,
	command_topic, location_id, is_active, metadata, created_at, updated_at, deleted_at,
	scan_device_id`

func scanOutputDevice(row pgx.Row, d *outputdevice.OutputDevice) error {
	return row.Scan(&d.ID, &d.OrgID, &d.Name, &d.Type, &d.Transport, &d.BaseURL, &d.SwitchID,
		&d.CommandTopic, &d.LocationID, &d.IsActive, &d.Metadata, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt,
		&d.ScanDeviceID)
}

// outputDeviceColumnsQualified is outputDeviceColumns with every column
// qualified by the "od" alias, for the two read paths that LEFT JOIN
// scan_devices to resolve ReaderBaseTopic (GetOutputDeviceByID,
// ListOutputDevicesForLocation). Kept separate from outputDeviceColumns:
// the INSERT/RETURNING and plain-list paths have no join and no alias.
const outputDeviceColumnsQualified = `od.id, od.org_id, od.name, od.type, od.transport, od.base_url, od.switch_id,
	od.command_topic, od.location_id, od.is_active, od.metadata, od.created_at, od.updated_at, od.deleted_at,
	od.scan_device_id`

// scanOutputDeviceWithReaderTopic scans outputDeviceColumnsQualified plus a
// trailing sd.publish_topic (nullable: NULL when scan_device_id is unset or
// the joined reader is not visible under the org's RLS GUC), and derives
// ReaderBaseTopic from it. Mirrors
// handlers/readerconfig.baseTopicForDevice exactly: strip a trailing
// "/reads" suffix; "" when publish_topic is NULL/empty.
func scanOutputDeviceWithReaderTopic(row pgx.Row, d *outputdevice.OutputDevice) error {
	var publishTopic *string
	if err := row.Scan(&d.ID, &d.OrgID, &d.Name, &d.Type, &d.Transport, &d.BaseURL, &d.SwitchID,
		&d.CommandTopic, &d.LocationID, &d.IsActive, &d.Metadata, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt,
		&d.ScanDeviceID, &publishTopic); err != nil {
		return err
	}
	if publishTopic != nil {
		d.ReaderBaseTopic = strings.TrimSuffix(*publishTopic, "/reads")
	}
	return nil
}

// CreateOutputDevice inserts an output device. type defaults to shelly_gen4,
// switch_id to 0, is_active to true, metadata to {} when omitted.
func (s *Storage) CreateOutputDevice(ctx context.Context, orgID int, req outputdevice.CreateOutputDeviceRequest) (*outputdevice.OutputDevice, error) {
	deviceType := req.Type
	if deviceType == "" {
		deviceType = outputdevice.TypeShellyGen4
	}
	transport := req.Transport
	if transport == "" {
		transport = outputdevice.TransportHTTP
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
		INSERT INTO trakrf.output_devices
		(org_id, name, type, transport, base_url, switch_id, command_topic, location_id, is_active, metadata, scan_device_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING ` + outputDeviceColumns

	var d outputdevice.OutputDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanOutputDevice(tx.QueryRow(ctx, query, orgID, req.Name, deviceType, transport, req.BaseURL,
			switchID, req.CommandTopic, req.LocationID, isActive, metadata, req.ScanDeviceID), &d)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create output device: %w", err)
	}
	return &d, nil
}

// GetOutputDeviceByID returns the live output device or (nil, nil) if not
// found. LEFT JOINs scan_devices to populate ReaderBaseTopic for GPO devices
// (the test-fire path). The join runs under the org's RLS GUC, so a
// cross-org scan_device_id simply does not join and ReaderBaseTopic stays "".
func (s *Storage) GetOutputDeviceByID(ctx context.Context, orgID, id int) (*outputdevice.OutputDevice, error) {
	query := `SELECT ` + outputDeviceColumnsQualified + `, sd.publish_topic
		FROM trakrf.output_devices od
		LEFT JOIN trakrf.scan_devices sd
		  ON sd.id = od.scan_device_id AND sd.deleted_at IS NULL
		WHERE od.id = $1 AND od.org_id = $2 AND od.deleted_at IS NULL`
	var d outputdevice.OutputDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanOutputDeviceWithReaderTopic(tx.QueryRow(ctx, query, id, orgID), &d)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get output device: %w", err)
	}
	return &d, nil
}

// ListOutputDevices returns live output devices for the org, newest first.
func (s *Storage) ListOutputDevices(ctx context.Context, orgID, limit, offset int) ([]outputdevice.OutputDevice, error) {
	query := `SELECT ` + outputDeviceColumns + `
		FROM trakrf.output_devices
		WHERE org_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	devices := []outputdevice.OutputDevice{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d outputdevice.OutputDevice
			if err := scanOutputDevice(rows, &d); err != nil {
				return err
			}
			devices = append(devices, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list output devices: %w", err)
	}
	return devices, nil
}

// CountOutputDevices returns the number of live output devices for the org.
func (s *Storage) CountOutputDevices(ctx context.Context, orgID int) (int, error) {
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COUNT(*) FROM trakrf.output_devices WHERE org_id = $1 AND deleted_at IS NULL`, orgID).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count output devices: %w", err)
	}
	return n, nil
}

// ListOutputDevicesForLocation returns active, non-deleted devices bound to the
// given logical location. Used by the geofence firer: the engine resolves the
// tripped scan point to a location, and every alarm bound to that location fires.
// LEFT JOINs scan_devices to populate ReaderBaseTopic for GPO devices; the
// join runs under the org's RLS GUC, so a cross-org reader does not join and
// ReaderBaseTopic stays "" (the dispatcher, Task 8, refuses to fire on that).
func (s *Storage) ListOutputDevicesForLocation(ctx context.Context, orgID, locationID int) ([]outputdevice.OutputDevice, error) {
	query := `SELECT ` + outputDeviceColumnsQualified + `, sd.publish_topic
		FROM trakrf.output_devices od
		LEFT JOIN trakrf.scan_devices sd
		  ON sd.id = od.scan_device_id AND sd.deleted_at IS NULL
		WHERE od.org_id = $1 AND od.location_id = $2 AND od.is_active = true AND od.deleted_at IS NULL
		ORDER BY od.id`
	out := []outputdevice.OutputDevice{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, locationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d outputdevice.OutputDevice
			if err := scanOutputDeviceWithReaderTopic(rows, &d); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list output devices for location: %w", err)
	}
	return out, nil
}

// UpdateOutputDevice applies a partial update. Returns (nil, nil) if no live
// device with that id exists for the org.
func (s *Storage) UpdateOutputDevice(ctx context.Context, orgID, id int, req outputdevice.UpdateOutputDeviceRequest) (*outputdevice.OutputDevice, error) {
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
	if req.BaseURL != nil {
		add("base_url", *req.BaseURL)
	}
	if req.CommandTopic != nil {
		add("command_topic", *req.CommandTopic)
	}
	if req.SwitchID != nil {
		add("switch_id", *req.SwitchID)
	}
	if req.ScanDeviceID != nil {
		add("scan_device_id", *req.ScanDeviceID)
	}
	if req.ClearLocationID {
		setClauses = append(setClauses, "location_id = NULL")
	} else if req.LocationID != nil {
		add("location_id", *req.LocationID)
	}
	if req.IsActive != nil {
		add("is_active", *req.IsActive)
	}
	if req.Metadata != nil {
		add("metadata", *req.Metadata)
	}

	// Nothing to update: return the current row (or nil if missing).
	if len(setClauses) == 0 {
		return s.GetOutputDeviceByID(ctx, orgID, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf(`
		UPDATE trakrf.output_devices
		SET %s
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		RETURNING `+outputDeviceColumns, strings.Join(setClauses, ", "))

	var d outputdevice.OutputDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanOutputDevice(tx.QueryRow(ctx, query, args...), &d)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update output device: %w", err)
	}
	return &d, nil
}

// DeleteOutputDevice soft-deletes the device. Returns false if no live device
// with that id existed.
func (s *Storage) DeleteOutputDevice(ctx context.Context, orgID, id int) (bool, error) {
	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			UPDATE trakrf.output_devices
			   SET deleted_at = NOW()
			 WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("could not delete output device: %w", err)
	}
	return rowsAffected > 0, nil
}

// GPOReaderCheck is the result of validating a scan_device_id a csl_cs463_gpo
// output device wants to bind to. Found/IsCS463/HasPublishTopic are
// independently inspectable so the caller can produce a distinct 400 message
// per failure mode — a single bool can't tell "not yours" from "wrong reader
// type" from "reader has no publish_topic", and an operator needs to know
// which (TRA-1028 hardening).
type GPOReaderCheck struct {
	// Found is true iff a live scan device with this id exists in the org.
	// Runs under the org's RLS GUC, so a device in another org (or one that
	// doesn't exist at all) reads as not-found — the org-scoping is enforced
	// by RLS, not just the WHERE clause. IsCS463 and HasPublishTopic are only
	// meaningful when Found is true.
	Found bool
	// IsCS463 is true iff the reader's type is scandevice.DeviceTypeCS463.
	// The GPO fire path (Gpo.Set over the reader's mqtt-rpc base topic) only
	// exists on the CS463 daemon; any other reader type has nothing listening.
	IsCS463 bool
	// HasPublishTopic is true iff the reader's publish_topic is non-NULL and
	// non-empty. The alarm dispatcher derives the reader's RPC base topic
	// from publish_topic at fire time (stripping a trailing "/reads"); a
	// reader with no publish_topic yields an empty base topic and the
	// dispatcher fail-closed refuses to fire — but that refusal only
	// surfaces at fire time, invisibly, unless caught here at write time.
	HasPublishTopic bool
}

// CheckGPOReader validates the reader a csl_cs463_gpo output device's
// scan_device_id would bind to. One query under the org's RLS GUC (RLS
// remains the org boundary); see GPOReaderCheck for what each field means.
func (s *Storage) CheckGPOReader(ctx context.Context, orgID, scanDeviceID int) (GPOReaderCheck, error) {
	var check GPOReaderCheck
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var devType string
		var publishTopic *string
		err := tx.QueryRow(ctx, `
			SELECT type, publish_topic FROM trakrf.scan_devices
			WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		`, scanDeviceID, orgID).Scan(&devType, &publishTopic)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil
			}
			return err
		}
		check.Found = true
		check.IsCS463 = devType == scandevice.DeviceTypeCS463
		check.HasPublishTopic = publishTopic != nil && *publishTopic != ""
		return nil
	})
	if err != nil {
		return GPOReaderCheck{}, fmt.Errorf("failed to check gpo reader: %w", err)
	}
	return check, nil
}
