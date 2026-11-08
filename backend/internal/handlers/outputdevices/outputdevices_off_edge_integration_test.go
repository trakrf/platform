//go:build integration

package outputdevices_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// An egress output's only OFF edge is its device-side auto-off, so a device
// whose resolved auto-off is 0 latches on forever. These pin the write path's
// refusal of that state, and of any on-time past the reader's 60 s ceiling.

func TestOffEdge_CreateEgressWithNoAutoOffIsRefused(t *testing.T) {
	r, orgID := newTestServer(t, &fakeDriver{})

	// What the form sent for a blank Auto-off field: mode only.
	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Strobe", "base_url": "http://192.168.50.66", "metadata": map[string]any{"mode": "egress"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "auto_off_seconds is required")

	// No metadata at all resolves to egress too.
	rec = doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Strobe", "base_url": "http://192.168.50.66",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestOffEdge_OrgDefaultSuppliesTheAutoOff(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	r, orgID := newTestServerOn(t, db, &fakeDriver{})
	thirty := 30
	require.NoError(t, db.Store.UpdateOrgGeofenceDefaults(context.Background(), orgID,
		organization.GeofenceDefaults{AutoOffSeconds: &thirty}))

	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Strobe", "base_url": "http://192.168.50.66", "metadata": map[string]any{"mode": "egress"},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestOffEdge_AutoOffOverTheCeilingIsRefused(t *testing.T) {
	r, orgID := newTestServer(t, &fakeDriver{})
	for _, mode := range []string{"egress", "presence"} {
		rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
			"name": "Strobe", "base_url": "http://192.168.50.66",
			"metadata": map[string]any{"mode": mode, "auto_off_seconds": 61},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, "%s: %s", mode, rec.Body.String())
		require.Contains(t, rec.Body.String(), "between 0 and 60")
	}
	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Strobe", "base_url": "http://192.168.50.66",
		"metadata": map[string]any{"mode": "egress", "auto_off_seconds": 60},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestOffEdge_PresenceNeedsNoAutoOff(t *testing.T) {
	r, orgID := newTestServer(t, &fakeDriver{})
	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Horn", "base_url": "http://192.168.50.66", "metadata": map[string]any{"mode": "presence"},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestOffEdge_PatchIsValidatedAgainstTheMergedDevice(t *testing.T) {
	r, orgID := newTestServer(t, &fakeDriver{})
	id := createDevice(t, r, orgID, "http://192.168.50.66")

	// Replacing metadata with one that drops the auto-off strands it.
	rec := doReq(t, r, orgID, http.MethodPatch, "/api/v1/output-devices/"+itoa(id), map[string]any{
		"metadata": map[string]any{"mode": "egress"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// A rename leaves the stored, valid metadata in place.
	rec = doReq(t, r, orgID, http.MethodPatch, "/api/v1/output-devices/"+itoa(id), map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// Rows written before this check exist (preview had two). Editing one is held
// to the rule, but a presence row's meaningless 0 must not block a rename.
func TestOffEdge_LegacyRows(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	r, orgID := newTestServerOn(t, db, &fakeDriver{})
	ctx := context.Background()

	latched, err := db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Latched", BaseURL: "http://192.168.50.66", Metadata: map[string]any{"mode": "egress"},
	})
	require.NoError(t, err)
	rec := doReq(t, r, orgID, http.MethodPatch, "/api/v1/output-devices/"+itoa(latched.ID), map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "auto_off_seconds is required")

	presence, err := db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Horn", BaseURL: "http://192.168.50.67", Metadata: map[string]any{"mode": "presence", "auto_off_seconds": 0},
	})
	require.NoError(t, err)
	rec = doReq(t, r, orgID, http.MethodPatch, "/api/v1/output-devices/"+itoa(presence.ID), map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestDelete_DrivesTheOutputOffFirst(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)
	id := createDevice(t, r, orgID, "http://192.168.50.66")

	rec := doReq(t, r, orgID, http.MethodDelete, "/api/v1/output-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.Equal(t, []setCall{{"http://192.168.50.66", 0, false}}, drv.calls)
}

// A failed OFF keeps the row, so Reset can still release the output once the
// device is reachable. force=true is the way out for a device that is gone.
func TestDelete_UnreachableKeepsTheRowUnlessForced(t *testing.T) {
	drv := &fakeDriver{failURL: "http://192.168.50.66"}
	r, orgID := newTestServer(t, drv)
	id := createDevice(t, r, orgID, "http://192.168.50.66")

	rec := doReq(t, r, orgID, http.MethodDelete, "/api/v1/output-devices/"+itoa(id), nil)
	// 5xx detail is masked (logged, not returned); the status is the contract the
	// UI keys its "delete anyway?" prompt on.
	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
	require.Equal(t, []setCall{{"http://192.168.50.66", 0, false}}, drv.calls, "the off was attempted")
	rec = doReq(t, r, orgID, http.MethodGet, "/api/v1/output-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusOK, rec.Code, "row must survive a failed off")

	rec = doReq(t, r, orgID, http.MethodDelete, "/api/v1/output-devices/"+itoa(id)+"?force=true", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = doReq(t, r, orgID, http.MethodGet, "/api/v1/output-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDelete_MissingIs404WithoutDriving(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)
	rec := doReq(t, r, orgID, http.MethodDelete, "/api/v1/output-devices/99999999", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, drv.calls)
}
