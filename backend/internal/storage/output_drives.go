package storage

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// DriveCause says what drove an output device. The values match the
// output_drives.cause check.
type DriveCause string

const (
	DriveCauseAlarm   DriveCause = "alarm"   // the ON a geofence fire sent
	DriveCauseRefresh DriveCause = "refresh" // a held presence output's re-ON
	DriveCauseAgeOut  DriveCause = "age_out" // a presence output's OFF on age-out
	DriveCauseTest    DriveCause = "test"    // the test action's ON and OFF
	DriveCauseReset   DriveCause = "reset"   // the reset action's OFF
	DriveCauseDelete  DriveCause = "delete"  // the OFF sent before a delete
)

// maxDriveErrorBytes caps the stored error text, so a transport that returns
// a whole response body cannot bloat the table.
const maxDriveErrorBytes = 1000

// recordDriveTimeout bounds the write of one drive row. The write is detached
// from the caller's context, so this is the only bound it has.
const recordDriveTimeout = 5 * time.Second

// OutputDriveRow is one drive to record: Err nil means it worked.
type OutputDriveRow struct {
	OutputDeviceID int
	AlarmEventID   *int64 // the fire this ON delivered; nil for every other cause
	On             bool
	Cause          DriveCause
	Err            error
	Latency        time.Duration
	DrivenAt       time.Time
}

// OutputDrive is one recorded drive, as read back.
type OutputDrive struct {
	ID             int64
	OutputDeviceID int
	AlarmEventID   *int64
	Edge           string // on | off
	Cause          string
	Result         string // ok | error
	Error          *string
	LatencyMs      int
	DrivenAt       time.Time
}

// RecordOutputDrive writes one drive row under org context (RLS). Callers
// treat it as best-effort: a failure here is logged and never changes what
// the drive did. The write is detached from ctx's cancellation, because the
// drive has already happened: an HTTP client hanging up after a test fire
// must not erase the record of it.
func (s *Storage) RecordOutputDrive(ctx context.Context, orgID int, row OutputDriveRow) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordDriveTimeout)
	defer cancel()

	edge := "off"
	if row.On {
		edge = "on"
	}
	result := "ok"
	var errText *string
	if row.Err != nil {
		result = "error"
		t := truncateDriveError(row.Err.Error())
		errText = &t
	}
	latencyMs := max(0, row.Latency.Milliseconds())

	return s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO trakrf.output_drives
			   (org_id, output_device_id, alarm_event_id, edge, cause, result, error, latency_ms, driven_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			orgID, row.OutputDeviceID, row.AlarmEventID, edge, string(row.Cause), result, errText, latencyMs, row.DrivenAt,
		)
		if err != nil {
			return fmt.Errorf("insert output_drive: %w", err)
		}
		return nil
	})
}

// ListOutputDrives reads one output device's most recent drives, newest
// first: the per-output activity log.
func (s *Storage) ListOutputDrives(ctx context.Context, orgID, outputDeviceID, limit int) ([]OutputDrive, error) {
	var out []OutputDrive
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, output_device_id, alarm_event_id, edge, cause, result, error, latency_ms, driven_at
			   FROM trakrf.output_drives
			  WHERE org_id = $1 AND output_device_id = $2
			  ORDER BY driven_at DESC, id DESC
			  LIMIT $3`,
			orgID, outputDeviceID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d OutputDrive
			if err := rows.Scan(&d.ID, &d.OutputDeviceID, &d.AlarmEventID, &d.Edge, &d.Cause,
				&d.Result, &d.Error, &d.LatencyMs, &d.DrivenAt); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list output drives: %w", err)
	}
	return out, nil
}

// truncateDriveError caps s at maxDriveErrorBytes without splitting a rune.
func truncateDriveError(s string) string {
	if len(s) <= maxDriveErrorBytes {
		return s
	}
	cut := maxDriveErrorBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
