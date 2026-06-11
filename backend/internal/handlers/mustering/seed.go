package mustering

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/asset"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanpoint"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// seedResult summarizes what the idempotent seed created vs skipped.
type seedResult struct {
	PersonsCreated      int `json:"persons_created"`
	ZonesCreated        int `json:"zones_created"`
	MusterPointsCreated int `json:"muster_points_created"`
	DevicesCreated      int `json:"devices_created"`
	Skipped             int `json:"skipped"`
}

// Seed idempotently provisions the mustering demo data set (TRA-978): 12 person
// assets with badge tags, 3 zones + 2 muster points, 5 scan devices each with a
// scan point at its location, then one simulate round spreading people across the
// 3 zones. Idempotency: assets/locations keyed on external_key, devices on
// publish_topic — existing rows are skipped.
func (h *Handler) Seed(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, _, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	ctx := r.Context()

	org, err := h.store.GetOrganizationByID(ctx, orgID)
	if err != nil || org == nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, "organization lookup failed", reqID)
		return
	}
	slug := org.Identifier
	if slug == "" {
		httputil.WriteJSONError(w, r, http.StatusUnprocessableEntity, modelerrors.ErrValidation,
			"organization has no identifier; cannot build publish_topics", reqID)
		return
	}

	var res seedResult

	// ── persons + badge tags ───────────────────────────────────────────────────
	personAssetIDs := make([]int, 0, 12)
	for i := 1; i <= 12; i++ {
		extKey := fmt.Sprintf("MUSTER-P-%03d", i)
		name := fmt.Sprintf("Operator %03d", i)
		assetID, created, err := h.ensurePersonAsset(ctx, orgID, extKey, name, i)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		}
		personAssetIDs = append(personAssetIDs, assetID)
		if created {
			res.PersonsCreated++
		} else {
			res.Skipped++
		}
	}

	// ── zones ──────────────────────────────────────────────────────────────────
	zoneNames := []string{"Production Floor", "Warehouse", "Office"}
	zoneIDs := make([]int, 0, 3)
	for i, name := range zoneNames {
		extKey := fmt.Sprintf("MUSTER-Z-%03d", i+1)
		id, created, err := h.ensureLocation(ctx, orgID, extKey, name, false)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		}
		zoneIDs = append(zoneIDs, id)
		if created {
			res.ZonesCreated++
		} else {
			res.Skipped++
		}
	}

	// ── muster points ──────────────────────────────────────────────────────────
	musterNames := []string{"Muster Point A", "Muster Point B"}
	musterIDs := make([]int, 0, 2)
	for i, name := range musterNames {
		extKey := fmt.Sprintf("MUSTER-MP-%03d", i+1)
		id, created, err := h.ensureLocation(ctx, orgID, extKey, name, true)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		}
		musterIDs = append(musterIDs, id)
		if created {
			res.MusterPointsCreated++
		} else {
			res.Skipped++
		}
	}

	// ── scan devices (one per location, scan point bound to that location) ──────
	deviceLocations := append(append([]int{}, zoneIDs...), musterIDs...) // 5 locations
	deviceNames := []string{"Production Floor Reader", "Warehouse Reader", "Office Reader", "Muster Point A Reader", "Muster Point B Reader"}
	for i, locationID := range deviceLocations {
		extKey := fmt.Sprintf("MUSTER-DEV-%03d", i+1)
		topic := fmt.Sprintf("%s/%s/reads", slug, extKey)
		created, err := h.ensureScanDevice(ctx, orgID, deviceNames[i], topic, locationID)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		}
		if created {
			res.DevicesCreated++
		} else {
			res.Skipped++
		}
	}

	// ── initial simulate round: spread persons across the 3 zones ──────────────
	// Distribute the 12 persons round-robin over the zones (group by zone so one
	// synthetic message per zone) so the presence dashboard has data immediately.
	byZone := map[int][]int{}
	for i, assetID := range personAssetIDs {
		zoneID := zoneIDs[i%len(zoneIDs)]
		byZone[zoneID] = append(byZone[zoneID], assetID)
	}
	receivedAt := time.Now()
	for zoneID, assetIDs := range byZone {
		// Best-effort: don't fail the seed on a simulate hiccup — data is provisioned.
		_, _, _ = h.runLocationSightings(ctx, orgID, slug, zoneID, assetIDs, receivedAt)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// ensurePersonAsset returns the asset id for extKey, creating the person asset +
// its badge tag when absent. created reports whether a new asset was inserted.
func (h *Handler) ensurePersonAsset(ctx context.Context, orgID int, extKey, name string, idx int) (int, bool, error) {
	existing, err := h.store.GetAssetByExternalKey(ctx, orgID, extKey)
	if err != nil {
		return 0, false, fmt.Errorf("lookup asset %s: %w", extKey, err)
	}
	if existing != nil {
		return existing.ID, false, nil
	}
	created, err := h.store.CreateAsset(ctx, asset.Asset{
		OrgID:       orgID,
		ExternalKey: extKey,
		Name:        name,
		Metadata:    map[string]any{"person": true},
		IsActive:    true,
	})
	if err != nil {
		return 0, false, fmt.Errorf("create asset %s: %w", extKey, err)
	}
	// Badge tag: synthetic MAC AA:BB:CC:00:00:01..0C (uppercase), type rfid.
	mac := fmt.Sprintf("AA:BB:CC:00:00:%02X", idx)
	rfid := shared.DefaultTagType
	if _, err := h.store.AddTagToAsset(ctx, orgID, created.ID, shared.TagRequest{TagType: &rfid, Value: mac}); err != nil {
		return 0, false, fmt.Errorf("add badge tag to %s: %w", extKey, err)
	}
	return created.ID, true, nil
}

// ensureLocation returns the location id for extKey, creating it (zone or muster
// point) when absent.
func (h *Handler) ensureLocation(ctx context.Context, orgID int, extKey, name string, musterPoint bool) (int, bool, error) {
	existing, err := h.store.GetLocationByExternalKey(ctx, orgID, extKey)
	if err != nil {
		return 0, false, fmt.Errorf("lookup location %s: %w", extKey, err)
	}
	if existing != nil {
		return existing.ID, false, nil
	}
	loc := location.Location{
		OrgID:       orgID,
		ExternalKey: extKey,
		Name:        name,
		IsActive:    true,
	}
	created, err := h.store.CreateLocation(ctx, loc)
	if err != nil {
		return 0, false, fmt.Errorf("create location %s: %w", extKey, err)
	}
	if musterPoint {
		// Stamp metadata.muster_point=true. CreateLocation does not take metadata,
		// so add the marker tag via a metadata PATCH-equivalent: set it directly.
		if err := h.store.SetLocationMusterPoint(ctx, orgID, created.ID); err != nil {
			return 0, false, fmt.Errorf("mark location %s as muster point: %w", extKey, err)
		}
	}
	return created.ID, true, nil
}

// ensureScanDevice creates a gl_s10 mqtt device with the given publish_topic (if
// absent) and binds its auto-created scan point to locationID.
func (h *Handler) ensureScanDevice(ctx context.Context, orgID int, name, topic string, locationID int) (bool, error) {
	exists, err := h.store.ScanDeviceTopicExists(ctx, orgID, topic)
	if err != nil {
		return false, fmt.Errorf("lookup device topic %s: %w", topic, err)
	}
	if exists {
		return false, nil
	}
	dev, err := h.store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name:         name,
		Type:         scandevice.DeviceTypeGLS10,
		Transport:    scandevice.TransportMQTT,
		PublishTopic: &topic,
	})
	if err != nil {
		return false, fmt.Errorf("create device %s: %w", topic, err)
	}
	// CreateScanDevice auto-creates scan point antenna 1 with no location; bind it
	// so PersistReads resolves the location for simulate + hardware reads.
	points, err := h.store.ListScanPointsByDevice(ctx, orgID, dev.ID)
	if err != nil {
		return false, fmt.Errorf("list points for device %s: %w", topic, err)
	}
	for _, p := range points {
		loc := locationID
		if _, err := h.store.UpdateScanPoint(ctx, orgID, p.ID, scanpoint.UpdateScanPointRequest{LocationID: &loc}); err != nil {
			return false, fmt.Errorf("bind scan point %d to location %d: %w", p.ID, locationID, err)
		}
	}
	return true, nil
}
