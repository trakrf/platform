//go:build integration
// +build integration

package readerconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/readercontrol"
	"github.com/trakrf/platform/backend/internal/readerrelease"
	"github.com/trakrf/platform/backend/internal/readerrpc"
	"github.com/trakrf/platform/backend/internal/testutil"
)

var benchManifest = readerrpc.DaemonUpdateParams{
	Version: "v2", URL: "https://example.test/mqtt-rpcd", SHA256: "ab", Signature: "c2ln",
}

func softwareFixture(t *testing.T, d *fakeDaemon, rel *fakeReleases) (http.Handler, int, int) {
	t.Helper()
	store, cleanup := testutil.SetupTestDB(t)
	t.Cleanup(cleanup)
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	t.Cleanup(func() { testutil.CleanupTestAccounts(t, pool) })
	deviceID := seedScanDeviceWithTopic(t, store, orgID, "trakrf.id/cs463-software/reads")
	return softwareRouter(NewHandler(store, nil).WithSoftware(d, rel)), orgID, deviceID
}

func do(t *testing.T, h http.Handler, orgID int, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, withOrg(req, orgID))
	return rr
}

func TestGetSoftware_ReportsDaemonVersionAndUpdateSupport(t *testing.T) {
	d := &fakeDaemon{caps: readerrpc.Capabilities{
		Supports: []string{readerrpc.MethodGetCapabilities, readerrpc.MethodDaemonUpdate},
		Daemon: &readerrpc.DaemonInfo{Version: "v1", Update: &readerrpc.UpdateInfo{
			State: readerrpc.UpdateStateRolledBack, Target: "v2", Error: "crash_loop",
		}},
	}}
	h, org, dev := softwareFixture(t, d, &fakeReleases{})

	rr := do(t, h, org, http.MethodGet, fmt.Sprintf("/api/v1/scan-devices/%d/reader-software", dev), "")
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var body struct {
		Data struct {
			Daemon         readerrpc.DaemonInfo `json:"daemon"`
			SupportsUpdate bool                 `json:"supports_update"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "v1", body.Data.Daemon.Version)
	assert.Equal(t, "crash_loop", body.Data.Daemon.Update.Error)
	assert.True(t, body.Data.SupportsUpdate)
}

func TestGetSoftware_ReaderOfflineIs502(t *testing.T) {
	h, org, dev := softwareFixture(t, &fakeDaemon{capsErr: errors.New("reader did not respond (offline?)")}, &fakeReleases{})
	rr := do(t, h, org, http.MethodGet, fmt.Sprintf("/api/v1/scan-devices/%d/reader-software", dev), "")
	assert.Equal(t, http.StatusBadGateway, rr.Code)
}

func TestUpdateSoftware_RelaysTheSignedManifestToTheReader(t *testing.T) {
	d := &fakeDaemon{updRes: readerrpc.DaemonUpdateResult{Accepted: true, From: "v1", To: "v2"}}
	rel := &fakeReleases{m: benchManifest}
	h, org, dev := softwareFixture(t, d, rel)

	rr := do(t, h, org, http.MethodPost, fmt.Sprintf("/api/v1/scan-devices/%d/reader-software/update", dev), `{"version":"v2"}`)
	require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())
	assert.Equal(t, []string{"v2"}, rel.asked)
	assert.Equal(t, []readerrpc.DaemonUpdateParams{benchManifest}, d.got)
	assert.Contains(t, rr.Body.String(), `"accepted":true`)
}

func TestUpdateSoftware_RefusedIs422WithTheReason(t *testing.T) {
	d := &fakeDaemon{updErr: &readerrpc.UpdateRefusedError{Reason: "an update is already in progress"}}
	h, org, dev := softwareFixture(t, d, &fakeReleases{m: benchManifest})
	rr := do(t, h, org, http.MethodPost, fmt.Sprintf("/api/v1/scan-devices/%d/reader-software/update", dev), `{"version":"v2"}`)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code, rr.Body.String())
	// The standard envelope, like every other error this API returns.
	var body struct {
		Error struct {
			Type   string `json:"type"`
			Status int    `json:"status"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "validation_error", body.Error.Type)
	assert.Equal(t, http.StatusUnprocessableEntity, body.Error.Status)
	assert.Equal(t, "an update is already in progress", body.Error.Detail)
}

// The request reached the reader but its reply was lost: the update may well be
// running. 504 says "outcome unknown", distinct from 502 "never sent".
func TestUpdateSoftware_LostReplyIs504(t *testing.T) {
	d := &fakeDaemon{updErr: fmt.Errorf("readercontrol: Daemon.Update on trakrf.id/x: %w", readercontrol.ErrNoReply)}
	h, org, dev := softwareFixture(t, d, &fakeReleases{m: benchManifest})
	rr := do(t, h, org, http.MethodPost, fmt.Sprintf("/api/v1/scan-devices/%d/reader-software/update", dev), `{"version":"v2"}`)
	assert.Equal(t, http.StatusGatewayTimeout, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), `"type":"internal_error"`)
}

func TestUpdateSoftware_UnsentRequestIs502(t *testing.T) {
	d := &fakeDaemon{updErr: errors.New("readercontrol: publish request: not connected")}
	h, org, dev := softwareFixture(t, d, &fakeReleases{m: benchManifest})
	rr := do(t, h, org, http.MethodPost, fmt.Sprintf("/api/v1/scan-devices/%d/reader-software/update", dev), `{"version":"v2"}`)
	assert.Equal(t, http.StatusBadGateway, rr.Code, rr.Body.String())
}

func TestUpdateSoftware_UnknownReleaseIs404AndNeverReachesTheReader(t *testing.T) {
	d := &fakeDaemon{}
	h, org, dev := softwareFixture(t, d, &fakeReleases{err: readerrelease.ErrNotFound})
	rr := do(t, h, org, http.MethodPost, fmt.Sprintf("/api/v1/scan-devices/%d/reader-software/update", dev), `{"version":"v9"}`)
	assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	assert.Empty(t, d.got)
}

func TestUpdateSoftware_UnknownDeviceIs404(t *testing.T) {
	h, org, _ := softwareFixture(t, &fakeDaemon{}, &fakeReleases{m: benchManifest})
	rr := do(t, h, org, http.MethodPost, "/api/v1/scan-devices/999999/reader-software/update", `{"version":"v2"}`)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}
