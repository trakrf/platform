//go:build integration

package scandevices_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/scandevices"
	"github.com/trakrf/platform/backend/internal/handlers/scanpoints"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func withOrg(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra899@t.com", CurrentOrgID: &orgID}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
}

func TestScanDevicesHandler_RoundTrip(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	scandevices.NewHandler(db.Store).RegisterRoutes(r)
	scanpoints.NewHandler(db.Store).RegisterRoutes(r)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req := httptest.NewRequest(method, path, &buf)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, withOrg(req, orgID))
		return rec
	}

	// Create
	rec := do(http.MethodPost, "/api/v1/scan-devices", map[string]any{
		"external_key": "cs463-214", "name": "Dock Reader", "type": "csl_cs463",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Data struct {
			ID           int    `json:"id"`
			Transport    string `json:"transport"`
			PublishTopic string `json:"publish_topic"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotZero(t, created.Data.ID)
	require.Equal(t, "mqtt", created.Data.Transport)
	require.Equal(t, "trakrf.id/cs463-214/reads", created.Data.PublishTopic)
	devicePath := "/api/v1/scan-devices/" + itoa(created.Data.ID)

	// Get
	require.Equal(t, http.StatusOK, do(http.MethodGet, devicePath, nil).Code)

	// List
	rec = do(http.MethodGet, "/api/v1/scan-devices", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "cs463-214")

	// Bad enum rejected
	require.Equal(t, http.StatusBadRequest, do(http.MethodPost, "/api/v1/scan-devices", map[string]any{
		"external_key": "x", "name": "x", "type": "not_a_device",
	}).Code)

	// Device create auto-provisioned antenna 1 (cs463-214-1).
	rec = do(http.MethodGet, devicePath+"/scan-points", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "cs463-214-1")

	// Add a second antenna via the nested route.
	rec = do(http.MethodPost, devicePath+"/scan-points", map[string]any{
		"external_key": "cs463-214-2", "name": "Antenna 2", "antenna_port": 2, "is_boundary": true,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var pt struct {
		Data struct {
			ID         int  `json:"id"`
			IsBoundary bool `json:"is_boundary"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pt))
	require.True(t, pt.Data.IsBoundary)

	// List points — both the auto antenna 1 and the added antenna 2.
	rec = do(http.MethodGet, devicePath+"/scan-points", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "cs463-214-1")
	require.Contains(t, rec.Body.String(), "cs463-214-2")

	// Patch device
	rec = do(http.MethodPatch, devicePath, map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "Renamed")

	// Delete point, then device
	require.Equal(t, http.StatusNoContent, do(http.MethodDelete, "/api/v1/scan-points/"+itoa(pt.Data.ID), nil).Code)
	require.Equal(t, http.StatusNoContent, do(http.MethodDelete, devicePath, nil).Code)

	// Gone
	require.Equal(t, http.StatusNotFound, do(http.MethodGet, devicePath, nil).Code)
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// TestScanPoints_UpdateLocationIDPersists pins the geofence-relevant behavior
// for TRA-931: PATCH /scan-points must persist a provided location_id (set it),
// and persist an explicit null (clear it). Regression guard for the handler
// passing location_id in the decoder's read-only `drop` set, which silently
// stripped it from the body so every location_id edit was a no-op 200.
func TestScanPoints_UpdateLocationIDPersists(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	scandevices.NewHandler(db.Store).RegisterRoutes(r)
	scanpoints.NewHandler(db.Store).RegisterRoutes(r)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req := httptest.NewRequest(method, path, &buf)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, withOrg(req, orgID))
		return rec
	}

	// A single-point gateway. Device create auto-provisions scan_point 1.
	rec := do(http.MethodPost, "/api/v1/scan-devices", map[string]any{
		"external_key": "gw-1", "name": "Gateway 1", "type": "gl_s10",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var dev struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dev))
	devicePath := "/api/v1/scan-devices/" + itoa(dev.Data.ID)

	// firstPoint GETs the device's points and returns the (sole) point id and
	// its location_id, decoding into a FRESH struct each call — location_id is
	// `omitempty`, so a cleared value is absent from the response and must not
	// be read off a reused struct.
	firstPoint := func() (int, *int) {
		rec := do(http.MethodGet, devicePath+"/scan-points", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var pts struct {
			Data []struct {
				ID         int  `json:"id"`
				LocationID *int `json:"location_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pts))
		require.Len(t, pts.Data, 1)
		return pts.Data[0].ID, pts.Data[0].LocationID
	}

	// Read back the auto-provisioned point.
	pointID, loc := firstPoint()
	require.Nil(t, loc, "point starts with no location")

	// A location to assign.
	var locID int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from)
		VALUES ($1, 'zone-a', 'Zone A', '', $2) RETURNING id
	`, orgID, time.Now().UTC()).Scan(&locID))

	// Set location_id via PATCH — it must persist.
	rec = do(http.MethodPatch, "/api/v1/scan-points/"+itoa(pointID), map[string]any{
		"location_id": locID,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, loc = firstPoint()
	require.NotNil(t, loc, "location_id must persist after PATCH")
	require.Equal(t, locID, *loc)

	// Explicit null clears it.
	rec = do(http.MethodPatch, "/api/v1/scan-points/"+itoa(pointID), map[string]any{
		"location_id": nil,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, loc = firstPoint()
	require.Nil(t, loc, "explicit null must clear location_id")
}
