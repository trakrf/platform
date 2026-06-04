//go:build integration

package scandevices_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

	// Nested scan point create
	rec = do(http.MethodPost, devicePath+"/scan-points", map[string]any{
		"external_key": "cs463-214-1", "name": "Antenna 1", "antenna_port": 1, "is_boundary": true,
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

	// List points
	rec = do(http.MethodGet, devicePath+"/scan-points", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "cs463-214-1")

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
