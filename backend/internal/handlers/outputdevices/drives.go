package outputdevices

import (
	"context"
	"time"

	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// set drives dev and records the drive as one output_drives row (TRA-1314),
// so a test, reset or delete leaves the same record a geofence fire does. It
// returns the drive's own error. The record is best-effort: a failed write is
// logged and never changes the response.
func (h *Handler) set(ctx context.Context, orgID int, dev outputdevice.OutputDevice, on bool, offAfterSec int, cause storage.DriveCause) error {
	drivenAt := time.Now()
	err := h.actuator.Set(ctx, dev, on, offAfterSec)
	if rerr := h.storage.RecordOutputDrive(ctx, orgID, storage.OutputDriveRow{
		OutputDeviceID: dev.ID,
		On:             on,
		Cause:          cause,
		Err:            err,
		Unconfirmed:    !dev.ConfirmsDrives(),
		Latency:        time.Since(drivenAt),
		DrivenAt:       drivenAt,
	}); rerr != nil {
		logger.Get().Error().Err(rerr).Int("org_id", orgID).Int("output_device_id", dev.ID).
			Str("cause", string(cause)).Msg("output_drives write failed")
	}
	return err
}
