package assetevent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/storage"
)

// manualStore is the storage surface a handheld save's recording needs.
type manualStore interface {
	PreviousAssetLocationsTx(ctx context.Context, tx pgx.Tx, orgID int, assetIDs []int, before time.Time) (map[int]storage.PreviousLocation, error)
}

// ManualSave records, inside the save transaction, each asset a handheld save
// put at a different location than its previous scan, and queues the owners'
// alerts. It holds no state.
type ManualSave struct {
	store manualStore
	rec   changeRecorder
}

func NewManualSave(store manualStore, rec changeRecorder) *ManualSave {
	return &ManualSave{store: store, rec: rec}
}

// Record is a storage.SaveRecordFunc. A freshly inserted row is compared with
// the asset's last sighting strictly before the saved minute: a first sighting
// or an unchanged location records nothing. A same-minute row overwritten with
// a different location is a correction of what was there, recorded from the
// overwritten location at the wall clock, so it sorts after the row it
// corrected and does not collide with it. Two saves of one asset whose wall
// clocks fall in the same microsecond would collide on (org, asset, time) and
// the second correction is dropped by the recorder, not reported.
//
// Errors are returned, not logged: the storage layer logs once and rolls the
// savepoint back.
func (m *ManualSave) Record(ctx context.Context, tx pgx.Tx, saved storage.SavedInventory) error {
	if err := m.recordInserted(ctx, tx, saved); err != nil {
		return err
	}
	return m.recordOverridden(ctx, tx, saved)
}

func (m *ManualSave) recordInserted(ctx context.Context, tx pgx.Tx, saved storage.SavedInventory) error {
	if len(saved.InsertedAssetIDs) == 0 {
		return nil
	}
	prev, err := m.store.PreviousAssetLocationsTx(ctx, tx, saved.OrgID, saved.InsertedAssetIDs, saved.Timestamp)
	if err != nil {
		return fmt.Errorf("previous location lookup failed: %w", err)
	}
	for _, assetID := range saved.InsertedAssetIDs {
		p, ok := prev[assetID]
		if !ok {
			continue // first sighting
		}
		if p.LocationID != nil && *p.LocationID == saved.LocationID {
			continue
		}
		err := m.record(ctx, tx, notificationevent.LocationChange{
			OrgID:          saved.OrgID,
			AssetID:        assetID,
			FromLocationID: p.LocationID,
			ToLocationID:   saved.LocationID,
			Origin:         notificationevent.OriginManualSave,
			DetectedAt:     saved.Timestamp,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *ManualSave) recordOverridden(ctx context.Context, tx pgx.Tx, saved storage.SavedInventory) error {
	for assetID, from := range saved.OverriddenFrom {
		err := m.record(ctx, tx, notificationevent.LocationChange{
			OrgID:          saved.OrgID,
			AssetID:        assetID,
			FromLocationID: from,
			ToLocationID:   saved.LocationID,
			Origin:         notificationevent.OriginManualOverride,
			DetectedAt:     time.Now(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *ManualSave) record(ctx context.Context, tx pgx.Tx, c notificationevent.LocationChange) error {
	if _, err := m.rec.RecordChange(ctx, tx, c); err != nil {
		return fmt.Errorf("failed to record %s for asset %d: %w", c.Origin, c.AssetID, err)
	}
	return nil
}
