//go:build integration

package orgs_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/orgs"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TestPatchGeofenceDefaults_RefusesToStrandAnEgressOutput drives the real handler
// and storage: clearing the org auto-off that an egress output relies on is
// refused with the device named, and the stored defaults are left unchanged.
// Membership and capability middleware are covered by the router tests; this
// calls the handler directly so only the strand check is under test.
func TestPatchGeofenceDefaults_RefusesToStrandAnEgressOutput(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	ctx := context.Background()
	h := orgs.NewHandler(db.Store, nil, nil)

	thirty := 30
	require.NoError(t, db.Store.UpdateOrgGeofenceDefaults(ctx, orgID, organization.GeofenceDefaults{AutoOffSeconds: &thirty}))
	_, err := db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Dock Strobe", BaseURL: "http://192.168.50.66", Metadata: map[string]any{"mode": "egress"},
	})
	require.NoError(t, err)

	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/orgs/"+strconv.Itoa(orgID)+"/geofence-defaults",
			bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", strconv.Itoa(orgID))
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		middleware.RequestID(http.HandlerFunc(h.PatchGeofenceDefaults)).ServeHTTP(rec, req)
		return rec
	}

	rec := patch(`{"rssi_threshold": -50}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "Dock Strobe")

	got, err := db.Store.GetOrgGeofenceDefaults(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, got.AutoOffSeconds, "refused change must not be written")
	require.Equal(t, 30, *got.AutoOffSeconds)

	// Keeping the auto-off makes the same edit fine.
	rec = patch(`{"rssi_threshold": -50, "auto_off_seconds": 30}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// And the ceiling holds at this tier too.
	rec = patch(`{"auto_off_seconds": 61}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}
