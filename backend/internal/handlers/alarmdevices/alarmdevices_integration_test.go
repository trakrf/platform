//go:build integration

package alarmdevices_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/alarmdevices"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/alarmdevice"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func withOrg(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra903@t.com", CurrentOrgID: &orgID}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
}

type setCall struct {
	baseURL  string
	switchID int
	on       bool
}

type fakeDriver struct {
	calls   []setCall
	failURL string
}

func (d *fakeDriver) Set(_ context.Context, dev alarmdevice.AlarmDevice, on bool) error {
	d.calls = append(d.calls, setCall{dev.BaseURL, dev.SwitchID, on})
	if dev.BaseURL == d.failURL {
		return errors.New("device unreachable")
	}
	return nil
}

func newTestServer(t *testing.T, drv *fakeDriver) (*chi.Mux, int) {
	t.Helper()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// testPulse 0: no blocking sleep in tests.
	alarmdevices.NewHandler(db.Store, drv, 0).RegisterRoutes(r)
	return r, orgID
}

func TestAlarmDevicesHandler_RoundTrip(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

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

	// Create with defaults.
	rec := do(http.MethodPost, "/api/v1/alarm-devices", map[string]any{
		"name": "Demo Strobe", "base_url": "http://192.168.50.66",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Data struct {
			ID       int    `json:"id"`
			Type     string `json:"type"`
			SwitchID int    `json:"switch_id"`
			IsActive bool   `json:"is_active"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotZero(t, created.Data.ID)
	require.Equal(t, "shelly_gen4", created.Data.Type)
	require.True(t, created.Data.IsActive)
	id := created.Data.ID

	// Get.
	rec = do(http.MethodGet, "/api/v1/alarm-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// List.
	rec = do(http.MethodGet, "/api/v1/alarm-devices", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Update.
	rec = do(http.MethodPatch, "/api/v1/alarm-devices/"+itoa(id), map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Delete.
	rec = do(http.MethodDelete, "/api/v1/alarm-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusNoContent, rec.Code)

	// Get after delete -> 404.
	rec = do(http.MethodGet, "/api/v1/alarm-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAlarmDevicesHandler_CreateValidation(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)
	// Missing name + bad base_url.
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{"base_url": "not-a-url"}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestAlarmDevicesHandler_CreateMQTT(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{
		"name": "Dock Strobe", "transport": "mqtt", "command_topic": "trakrf.id/dock-strobe",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created struct {
		Data struct {
			Transport    string `json:"transport"`
			CommandTopic string `json:"command_topic"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "mqtt", created.Data.Transport)
	require.Equal(t, "trakrf.id/dock-strobe", created.Data.CommandTopic)
}

func TestAlarmDevicesHandler_MQTTRequiresCommandTopic(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{
		"name": "Dock Strobe", "transport": "mqtt", // no command_topic
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestAlarmDevicesHandler_HTTPRequiresBaseURL(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{
		"name": "Dock Strobe", // default http transport, no base_url
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestAlarmDevicesHandler_TestFirePulses(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	id := createDevice(t, r, orgID, "http://192.168.50.66")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices/"+itoa(id)+"/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Pulse = on then off.
	require.Equal(t, []setCall{
		{"http://192.168.50.66", 0, true},
		{"http://192.168.50.66", 0, false},
	}, drv.calls)
}

func TestAlarmDevicesHandler_TestFireUnreachableIs502(t *testing.T) {
	drv := &fakeDriver{failURL: "http://192.168.50.66"}
	r, orgID := newTestServer(t, drv)

	id := createDevice(t, r, orgID, "http://192.168.50.66")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices/"+itoa(id)+"/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
}

func TestAlarmDevicesHandler_ResetTurnsOff(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	id := createDevice(t, r, orgID, "http://192.168.50.66")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices/"+itoa(id)+"/reset", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []setCall{{"http://192.168.50.66", 0, false}}, drv.calls)
}

func TestAlarmDevicesHandler_TestFireMissingIs404(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices/99999999/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, drv.calls)
}

// helpers

func createDevice(t *testing.T, r *chi.Mux, orgID int, baseURL string) int {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{"name": "D", "base_url": baseURL}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarm-devices", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	return created.Data.ID
}

func itoa(i int) string { return strconv.Itoa(i) }
