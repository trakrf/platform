//go:build integration

package outputdevices_test

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

	"github.com/trakrf/platform/backend/internal/handlers/outputdevices"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// passThrough is a no-op paid-gate middleware for tests (entitlement is
// enforced elsewhere; these tests exercise the handler logic).
func passThrough(next http.Handler) http.Handler { return next }

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

func (d *fakeDriver) Set(_ context.Context, dev outputdevice.OutputDevice, on bool, _ int) error {
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
	// testPulse 0: no blocking sleep in tests. Pass-through paid gate.
	outputdevices.NewHandler(db.Store, drv, 0).RegisterRoutes(r, passThrough)
	return r, orgID
}

func TestOutputDevicesHandler_RoundTrip(t *testing.T) {
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
	rec := do(http.MethodPost, "/api/v1/output-devices", map[string]any{
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
	rec = do(http.MethodGet, "/api/v1/output-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// List.
	rec = do(http.MethodGet, "/api/v1/output-devices", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Update.
	rec = do(http.MethodPatch, "/api/v1/output-devices/"+itoa(id), map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Delete.
	rec = do(http.MethodDelete, "/api/v1/output-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusNoContent, rec.Code)

	// Get after delete -> 404.
	rec = do(http.MethodGet, "/api/v1/output-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// TRA-940: an explicit `location_id: null` in a PATCH detaches the location;
// omitting the field leaves it unchanged.
func TestOutputDevicesHandler_ClearLocation(t *testing.T) {
	drv := &fakeDriver{}
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	outputdevices.NewHandler(db.Store, drv, 0).RegisterRoutes(r, passThrough)

	loc, err := db.Store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "dock-1", Name: "Dock 1",
	})
	require.NoError(t, err)

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

	type locResp struct {
		Data struct {
			ID         int  `json:"id"`
			LocationID *int `json:"location_id"`
		} `json:"data"`
	}

	// Create an output device bound to the location.
	rec := do(http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Bound Strobe", "base_url": "http://192.168.50.66", "location_id": loc.ID,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created locResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotNil(t, created.Data.LocationID)
	require.Equal(t, loc.ID, *created.Data.LocationID)
	id := created.Data.ID

	// PATCH omitting location_id leaves it attached.
	rec = do(http.MethodPatch, "/api/v1/output-devices/"+itoa(id), map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var kept locResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &kept))
	require.NotNil(t, kept.Data.LocationID, "omitting location_id leaves the binding")

	// PATCH with an explicit null detaches the location.
	rec = do(http.MethodPatch, "/api/v1/output-devices/"+itoa(id), map[string]any{"location_id": nil})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var cleared locResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cleared))
	require.Nil(t, cleared.Data.LocationID, "explicit location_id:null detaches the location")
}

func TestOutputDevicesHandler_CreateValidation(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)
	// Missing name + bad base_url.
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{"base_url": "not-a-url"}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestOutputDevicesHandler_CreateMQTT(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{
		"name": "Dock Strobe", "transport": "mqtt", "command_topic": "trakrf.id/dock-strobe",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices", &buf)
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

func TestOutputDevicesHandler_MQTTRequiresCommandTopic(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{
		"name": "Dock Strobe", "transport": "mqtt", // no command_topic
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestOutputDevicesHandler_HTTPRequiresBaseURL(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{
		"name": "Dock Strobe", // default http transport, no base_url
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestOutputDevicesHandler_UpdateMQTTIgnoresEmptyBaseURL reproduces TRA-928:
// editing an mqtt-transport output device while the client still sends an empty
// base_url (as the form historically did) must succeed — base_url is not
// applicable to mqtt and must not be validated as a URL.
func TestOutputDevicesHandler_UpdateMQTTIgnoresEmptyBaseURL(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	id := createMQTTDevice(t, r, orgID, "trakrf.id/dock-strobe")

	rec := doReq(t, r, orgID, http.MethodPatch, "/api/v1/output-devices/"+itoa(id), map[string]any{
		"transport": "mqtt", "command_topic": "trakrf.id/dock-strobe", "base_url": "",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestOutputDevicesHandler_UpdateHTTPRejectsInvalidBaseURL asserts the other half
// of TRA-928: http transport still requires a valid base_url on update.
func TestOutputDevicesHandler_UpdateHTTPRejectsInvalidBaseURL(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	id := createDevice(t, r, orgID, "http://192.168.50.66")

	rec := doReq(t, r, orgID, http.MethodPatch, "/api/v1/output-devices/"+itoa(id), map[string]any{
		"base_url": "not-a-url",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestOutputDevicesHandler_TestFirePulses(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	id := createDevice(t, r, orgID, "http://192.168.50.66")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices/"+itoa(id)+"/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Pulse = on then off.
	require.Equal(t, []setCall{
		{"http://192.168.50.66", 0, true},
		{"http://192.168.50.66", 0, false},
	}, drv.calls)
}

func TestOutputDevicesHandler_TestFireUnreachableIs502(t *testing.T) {
	drv := &fakeDriver{failURL: "http://192.168.50.66"}
	r, orgID := newTestServer(t, drv)

	id := createDevice(t, r, orgID, "http://192.168.50.66")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices/"+itoa(id)+"/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
}

func TestOutputDevicesHandler_ResetTurnsOff(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	id := createDevice(t, r, orgID, "http://192.168.50.66")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices/"+itoa(id)+"/reset", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []setCall{{"http://192.168.50.66", 0, false}}, drv.calls)
}

// createOrg creates a second organization beyond the default test_org, for
// cross-tenant tests (testutil.CreateTestAccount hardcodes identifier=
// "test-org", which is UNIQUE, so a second org needs a distinct one).
func createOrg(t *testing.T, db *testutil.TestDB, name, identifier string) int {
	t.Helper()
	var orgID int
	err := db.AdminPool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ($1, $2, true) RETURNING id`,
		name, identifier,
	).Scan(&orgID)
	require.NoError(t, err)
	return orgID
}

// TestOutputDevicesHandler_CreateGPORequiresScanDeviceID covers TRA-1028: a
// csl_cs463_gpo device create with no scan_device_id at all must 400 — the
// reader FK is how the device is addressed, so it cannot be omitted.
func TestOutputDevicesHandler_CreateGPORequiresScanDeviceID(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Reader GPO", "type": "csl_cs463_gpo", "transport": "mqtt", "switch_id": 1,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestOutputDevicesHandler_CreateGPORejectsForeignScanDeviceID covers the
// cross-org actuation hole this task closes: a scan_device_id that does not
// exist at all, and one that belongs to a different org, must both 400 rather
// than let the create succeed and (per Task 7/dispatcher) silently resolve to
// an empty ReaderBaseTopic at fire time.
func TestOutputDevicesHandler_CreateGPORejectsForeignScanDeviceID(t *testing.T) {
	drv := &fakeDriver{}
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	outputdevices.NewHandler(db.Store, drv, 0).RegisterRoutes(r, passThrough)

	// Nonexistent id.
	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Reader GPO", "type": "csl_cs463_gpo", "transport": "mqtt", "switch_id": 1,
		"scan_device_id": 99999999,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// A reader that exists, but in a different org.
	orgB := createOrg(t, db, "Org B GPO", "org-b-gpo")
	publishTopic := "trakrf.id/cs463-999/reads"
	readerB, err := db.Store.CreateScanDevice(context.Background(), orgB, scandevice.CreateScanDeviceRequest{
		Name: "Org B Reader", Type: "csl_cs463", PublishTopic: &publishTopic,
	})
	require.NoError(t, err)

	rec = doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Reader GPO", "type": "csl_cs463_gpo", "transport": "mqtt", "switch_id": 1,
		"scan_device_id": readerB.ID,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestOutputDevicesHandler_CreateGPOWithValidScanDeviceID is the happy path:
// a scan_device_id referencing a live reader in the org must succeed.
func TestOutputDevicesHandler_CreateGPOWithValidScanDeviceID(t *testing.T) {
	drv := &fakeDriver{}
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	outputdevices.NewHandler(db.Store, drv, 0).RegisterRoutes(r, passThrough)

	publishTopic := "trakrf.id/cs463-212/reads"
	reader, err := db.Store.CreateScanDevice(context.Background(), orgID, scandevice.CreateScanDeviceRequest{
		Name: "CS463-212", Type: "csl_cs463", PublishTopic: &publishTopic,
	})
	require.NoError(t, err)

	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Reader GPO", "type": "csl_cs463_gpo", "transport": "mqtt", "switch_id": 1,
		"scan_device_id": reader.ID,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

// TestOutputDevicesHandler_UpdateGPOFlipTypeWithoutScanDeviceID covers a PATCH
// that flips an existing shelly device's type to csl_cs463_gpo without
// resending scan_device_id: since the stored row has none, this must 400 —
// merging over the stored value must not let the FK requirement be skipped.
func TestOutputDevicesHandler_UpdateGPOFlipTypeWithoutScanDeviceID(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)

	id := createMQTTDevice(t, r, orgID, "trakrf.id/dock-strobe")

	rec := doReq(t, r, orgID, http.MethodPatch, "/api/v1/output-devices/"+itoa(id), map[string]any{
		"type": "csl_cs463_gpo", "switch_id": 1,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestOutputDevicesHandler_UpdateGPOKeepsStoredScanDeviceID covers the other
// half of the merge: a PATCH that touches unrelated fields on an existing GPO
// device, without resending scan_device_id, must keep passing validation
// against the stored (already-validated) FK rather than re-demand it.
func TestOutputDevicesHandler_UpdateGPOKeepsStoredScanDeviceID(t *testing.T) {
	drv := &fakeDriver{}
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	outputdevices.NewHandler(db.Store, drv, 0).RegisterRoutes(r, passThrough)

	publishTopic := "trakrf.id/cs463-212/reads"
	reader, err := db.Store.CreateScanDevice(context.Background(), orgID, scandevice.CreateScanDeviceRequest{
		Name: "CS463-212", Type: "csl_cs463", PublishTopic: &publishTopic,
	})
	require.NoError(t, err)

	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "Reader GPO", "type": "csl_cs463_gpo", "transport": "mqtt", "switch_id": 1,
		"scan_device_id": reader.ID,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = doReq(t, r, orgID, http.MethodPatch, "/api/v1/output-devices/"+itoa(created.Data.ID), map[string]any{
		"name": "Renamed GPO",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestOutputDevicesHandler_TestFireMissingIs404(t *testing.T) {
	drv := &fakeDriver{}
	r, orgID := newTestServer(t, drv)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices/99999999/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, drv.calls)
}

// helpers

func doReq(t *testing.T, r *chi.Mux, orgID int, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
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

func createMQTTDevice(t *testing.T, r *chi.Mux, orgID int, topic string) int {
	t.Helper()
	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices", map[string]any{
		"name": "D", "transport": "mqtt", "command_topic": topic,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	return created.Data.ID
}

func createDevice(t *testing.T, r *chi.Mux, orgID int, baseURL string) int {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{"name": "D", "base_url": baseURL}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices", &buf)
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
