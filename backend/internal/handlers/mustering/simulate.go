package mustering

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// simulateRequest is the simulator body: a list of (asset, location) sightings.
type simulateRequest struct {
	Sightings []sighting `json:"sightings"`
}

type sighting struct {
	AssetID    int `json:"asset_id"`
	LocationID int `json:"location_id"`
}

// Simulate synthesizes hardware-identical reads for the given sightings and runs
// them through the REAL ingest pipeline so asset_scans, the Live Reads feed, the
// geofence engine, and the muster engine all react exactly as for a hardware
// read. Returns 422 when a target location has no live scan point or an asset has
// no badge tag.
//
// Pipeline per distinct location (mirrors ingest.Subscriber.handleMessage):
//  1. InsertRawTagScan(ctx, "simulated/{slug}", payload)  — audit provenance
//  2. feed.Publish(orgID, device.publish_topic, reads)    — Live Reads (RSSI)
//  3. PersistReads(...)                                    — derive asset_scans
//  4. evaluator.Evaluate(...)                              — muster + geofence
func (h *Handler) Simulate(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, _, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	var req simulateRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if len(req.Sightings) == 0 {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, "sightings must not be empty", reqID)
		return
	}

	// Org slug for the synthetic audit topic.
	org, err := h.store.GetOrganizationByID(r.Context(), orgID)
	if err != nil || org == nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, "organization lookup failed", reqID)
		return
	}
	slug := org.Identifier
	if slug == "" {
		slug = fmt.Sprintf("org-%d", orgID)
	}

	// Group sightings by location so one synthetic message carries all the reads
	// for that location's device (exactly like a real reader publishes a batch).
	byLocation := map[int][]int{} // locationID -> []assetID
	order := []int{}
	for _, s := range req.Sightings {
		if _, seen := byLocation[s.LocationID]; !seen {
			order = append(order, s.LocationID)
		}
		byLocation[s.LocationID] = append(byLocation[s.LocationID], s.AssetID)
	}

	receivedAt := time.Now()
	inserted := 0

	for _, locationID := range order {
		n, code, simErr := h.runLocationSightings(r.Context(), orgID, slug, locationID, byLocation[locationID], receivedAt)
		if simErr != nil {
			status := http.StatusInternalServerError
			errType := modelerrors.ErrInternal
			if code == http.StatusUnprocessableEntity {
				status, errType = http.StatusUnprocessableEntity, modelerrors.ErrValidation
			}
			httputil.WriteJSONError(w, r, status, errType, simErr.Error(), reqID)
			return
		}
		inserted += n
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"sightings": len(req.Sightings), "asset_scans_inserted": inserted},
	})
}

// runLocationSightings drives the real ingest pipeline for one location's worth
// of sightings (one synthetic message). Returns the number of asset_scans
// inserted and, on failure, an HTTP-ish status hint (422 for unprocessable
// inputs, 500 otherwise) plus the error. Shared by Simulate and the demo seed.
func (h *Handler) runLocationSightings(ctx context.Context, orgID int, slug string, locationID int, assetIDs []int, receivedAt time.Time) (int, int, error) {
	sp, err := h.store.FindSimScanPointForLocation(ctx, orgID, locationID)
	if err != nil {
		return 0, http.StatusInternalServerError, err
	}
	if sp == nil {
		return 0, http.StatusUnprocessableEntity, fmt.Errorf("location %d has no active scan point", locationID)
	}

	reads := make([]scanread.Read, 0, len(assetIDs))
	for i, assetID := range assetIDs {
		value, err := h.store.GetAssetTagValue(ctx, orgID, assetID)
		if err != nil {
			return 0, http.StatusInternalServerError, err
		}
		if value == "" {
			return 0, http.StatusUnprocessableEntity, fmt.Errorf("asset %d has no badge tag", assetID)
		}
		reads = append(reads, scanread.Read{
			EPC:             value,
			AntennaPort:     sp.AntennaPort,
			RSSI:            -45 - (i % 26), // deterministic synthetic RSSI in -45..-70
			ReaderTimestamp: receivedAt,
		})
	}
	if len(reads) == 0 {
		return 0, 0, nil
	}

	// 1. Audit log (synthetic topic carries the org slug for traceability).
	topic := "simulated/" + slug
	payload, _ := json.Marshal(map[string]any{"location_id": locationID, "reads": reads})
	tagScanID, err := h.store.InsertRawTagScan(ctx, topic, payload)
	if err != nil {
		return 0, http.StatusInternalServerError, err
	}

	// 2. Live Reads feed (keyed on the device's real publish_topic so Locate's
	// RSSI indicator works simulator-only). Best-effort; skip when no feed/topic.
	if h.feed != nil && sp.PublishTopic != "" {
		h.feed.Publish(orgID, sp.PublishTopic, reads)
	}

	// 3. Derive asset_scans under org context (RLS-correct).
	res, err := h.store.PersistReads(ctx, orgID, sp.ScanDeviceID, tagScanID, receivedAt, reads)
	if err != nil {
		return 0, http.StatusInternalServerError, err
	}

	// 4. Fan out the membership-passing reads to the evaluator (muster + geofence).
	if h.evaluator != nil && len(res.Resolved) > 0 {
		h.evaluator.Evaluate(ctx, orgID, tagScanID, receivedAt, res.Resolved)
	}
	return res.Inserted, 0, nil
}
