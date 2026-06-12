//go:build integration

package mustering_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/muster"
)

// TestFloorPlan_RoundTrip seeds the demo data, confirms an unset plan reads as
// empty, then PUTs a plan referencing a real seeded location and reads it back.
func TestFloorPlan_RoundTrip(t *testing.T) {
	r, db, orgID := newMusterServer(t)

	// Seed so we have real locations to pin.
	rr := doJSON(t, r, orgID, http.MethodPost, "/api/v1/mustering/seed", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	// Unset plan reads as empty.
	rr = doJSON(t, r, orgID, http.MethodGet, "/api/v1/mustering/floor-plan", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var got struct {
		Data muster.FloorPlan `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "", got.Data.ImageURL)
	require.Empty(t, got.Data.Pins)

	// Resolve a seeded zone location to pin.
	zone, err := db.Store.GetLocationByExternalKey(context.Background(), orgID, "MUSTER-Z-001")
	require.NoError(t, err)
	require.NotNil(t, zone)

	body := muster.FloorPlan{
		ImageURL: "https://example.com/site.png",
		Pins: []muster.FloorPlanPin{
			{LocationID: zone.ID, XPct: 12.5, YPct: 80},
		},
	}
	rr = doJSON(t, r, orgID, http.MethodPut, "/api/v1/mustering/floor-plan", body)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "https://example.com/site.png", got.Data.ImageURL)
	require.Len(t, got.Data.Pins, 1)
	require.Equal(t, zone.ID, got.Data.Pins[0].LocationID)
	require.InDelta(t, 12.5, got.Data.Pins[0].XPct, 0.001)

	// Read back independently — persisted under org metadata.
	rr = doJSON(t, r, orgID, http.MethodGet, "/api/v1/mustering/floor-plan", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got.Data.Pins, 1)
	require.Equal(t, zone.ID, got.Data.Pins[0].LocationID)

	// Full-replace with no pins clears them but keeps image.
	rr = doJSON(t, r, orgID, http.MethodPut, "/api/v1/mustering/floor-plan",
		muster.FloorPlan{ImageURL: "https://example.com/site2.png"})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "https://example.com/site2.png", got.Data.ImageURL)
	require.Empty(t, got.Data.Pins)
}

// TestFloorPlan_Validation covers the rejection paths.
func TestFloorPlan_Validation(t *testing.T) {
	r, _, orgID := newMusterServer(t)

	// Empty image_url → 400.
	rr := doJSON(t, r, orgID, http.MethodPut, "/api/v1/mustering/floor-plan",
		muster.FloorPlan{ImageURL: ""})
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

	// Non-http/data scheme → 400.
	rr = doJSON(t, r, orgID, http.MethodPut, "/api/v1/mustering/floor-plan",
		muster.FloorPlan{ImageURL: "ftp://example.com/x.png"})
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

	// Out-of-range coordinate → 400.
	rr = doJSON(t, r, orgID, http.MethodPut, "/api/v1/mustering/floor-plan",
		muster.FloorPlan{ImageURL: "https://example.com/x.png", Pins: []muster.FloorPlanPin{{LocationID: 1, XPct: 150, YPct: 0}}})
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

	// Unknown location id → 422.
	rr = doJSON(t, r, orgID, http.MethodPut, "/api/v1/mustering/floor-plan",
		muster.FloorPlan{ImageURL: "https://example.com/x.png", Pins: []muster.FloorPlanPin{{LocationID: 999999999, XPct: 10, YPct: 10}}})
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code, rr.Body.String())
}
