//go:build integration
// +build integration

// TRA-1007: handler-level busy→409 coverage. When GetOperProfile or SetOperProfile
// returns a *readerrpc.BusyError the handler must map it to HTTP 409 with a JSON
// body containing "reader_busy" and the holder IP. These tests exercise the full
// HTTP dispatch path (handler + storage + org-context middleware) so that the
// errors.As mapping and respondBusy helper are proven to fire at the HTTP layer,
// not just in unit stubs.

package readerconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/readerrpc"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// busyHolder is the IP address returned by the fakeRPC in busy tests.
const busyHolder = "192.168.50.203"

// setupReaderConfigRouter mounts Get and Set on a chi router with RequestID
// middleware and chi URL params, mirroring the production registration.
func setupReaderConfigRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/scan-devices/{scan_device_id}/reader-config", h.Get)
	r.Patch("/api/v1/scan-devices/{scan_device_id}/reader-config", h.Set)
	return r
}

// withReaderConfigOrgContext injects a JWT claims value into the request context
// so middleware.GetRequestOrgID resolves the given orgID — same pattern used
// across all handler integration tests.
func withReaderConfigOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra1007-busy@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

// seedScanDeviceWithTopic inserts a scan device with the given publish_topic and
// returns its ID so handler tests can route to a real device row.
func seedScanDeviceWithTopic(t *testing.T, store interface {
	CreateScanDevice(ctx context.Context, orgID int, req scandevice.CreateScanDeviceRequest) (*scandevice.ScanDevice, error)
}, orgID int, topic string) int {
	t.Helper()
	d, err := store.CreateScanDevice(context.Background(), orgID, scandevice.CreateScanDeviceRequest{
		Name:         "Test CS463 Busy",
		Type:         scandevice.DeviceTypeCS463,
		PublishTopic: &topic,
	})
	require.NoError(t, err)
	return d.ID
}

// TestGet_ReaderBusy_Returns409 drives Handler.Get with a fakeRPC whose
// GetOperProfile returns a *readerrpc.BusyError (GetCapabilities succeeds).
// The handler must respond HTTP 409 with a JSON body containing "reader_busy"
// and the holder IP.
func TestGet_ReaderBusy_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	topic := "trakrf.id/cs463-test/reads"
	deviceID := seedScanDeviceWithTopic(t, store, orgID, topic)

	fake := &fakeRPC{
		// capsErr is nil → GetCapabilities uses shared err (nil) → succeeds
		getErr: &readerrpc.BusyError{HeldBy: busyHolder},
	}
	h := NewHandler(store, fake)
	router := setupReaderConfigRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/scan-devices/%d/reader-config", deviceID), nil)
	req = withReaderConfigOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code,
		"busy GetOperProfile must yield 409, got %d: %s", rr.Code, rr.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "reader_busy", body["error"],
		"body.error must be reader_busy: %s", rr.Body.String())
	assert.True(t, strings.Contains(rr.Body.String(), busyHolder),
		"body must contain the holder IP %q: %s", busyHolder, rr.Body.String())
}

// TestSet_ReaderBusy_Returns409 drives Handler.Set with a fakeRPC whose
// SetOperProfile returns a *readerrpc.BusyError. The handler must respond
// HTTP 409 with a JSON body containing "reader_busy" and the holder IP.
func TestSet_ReaderBusy_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	topic := "trakrf.id/cs463-test-set/reads"
	deviceID := seedScanDeviceWithTopic(t, store, orgID, topic)

	fake := &fakeRPC{
		setErr: &readerrpc.BusyError{HeldBy: busyHolder},
	}
	h := NewHandler(store, fake)
	router := setupReaderConfigRouter(h)

	// Send an antennas payload so it reaches SetOperProfile (valid power keeps
	// validateTxPower from rejecting before the RPC call).
	body := bytes.NewBufferString(`{"antennas":[{"antenna":1,"enabled":true,"power_dbm":22.0}]}`)
	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/scan-devices/%d/reader-config", deviceID), body)
	req.Header.Set("Content-Type", "application/json")
	req = withReaderConfigOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code,
		"busy SetOperProfile must yield 409, got %d: %s", rr.Code, rr.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "reader_busy", resp["error"],
		"body.error must be reader_busy: %s", rr.Body.String())
	assert.True(t, strings.Contains(rr.Body.String(), busyHolder),
		"body must contain the holder IP %q: %s", busyHolder, rr.Body.String())
}
