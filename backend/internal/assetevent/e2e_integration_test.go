//go:build integration

package assetevent_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/assetevent"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	webhookmodel "github.com/trakrf/platform/backend/internal/models/webhook"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/webhook"
)

// End-to-end: real Postgres, real detection, real webhook sink, an httptest
// server standing in for the customer's endpoint. Both asset_scans writers are
// driven exactly the way production drives them — PersistReads + Evaluate for
// the MQTT path, SaveInventoryScans + EvaluateScans for the handheld path.

const epcA = "E2801190A503006543E21224"

type delivery struct {
	body    []byte
	headers http.Header
}

// harness owns the fixture: an org with a webhook pointing at a capture server,
// a reader, and the wired detection stack.
type harness struct {
	db         *testutil.TestDB
	orgID      int
	evaluator  *assetevent.Evaluator
	dispatcher *assetevent.Dispatcher
	deliveries chan delivery
	device     *scandevice.ScanDevice
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	deliveries := make(chan delivery, 32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		deliveries <- delivery{body: body, headers: r.Header.Clone()}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	_, err := db.Store.CreateWebhook(context.Background(), orgID, srv.URL, "whsec_e2e", true)
	require.NoError(t, err)

	log := zerolog.Nop()
	sink := webhook.NewSink(db.Store, webhook.NewClient(true), &log)
	dispatcher := assetevent.NewDispatcher([]assetevent.Sink{sink}, &log)
	dispatcher.Start()
	t.Cleanup(dispatcher.Stop)

	topic := "trakrf.id/e2e-reader/reads"
	device, err := db.Store.CreateScanDevice(context.Background(), orgID, scandevice.CreateScanDeviceRequest{
		Name: "E2E Reader", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)

	return &harness{
		db:         db,
		orgID:      orgID,
		evaluator:  assetevent.NewEvaluator(db.Store, dispatcher, &log),
		dispatcher: dispatcher,
		deliveries: deliveries,
		device:     device,
	}
}

// location creates a location and returns its id.
func (h *harness) location(t *testing.T, key, name string) int {
	t.Helper()
	var id int
	err := h.db.AdminPool.QueryRow(context.Background(),
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, $2, $3) RETURNING id`,
		h.orgID, key, name).Scan(&id)
	require.NoError(t, err)
	return id
}

// asset creates an asset with an rfid tag and returns its id.
func (h *harness) asset(t *testing.T, externalKey, epc string) int {
	t.Helper()
	a := testutil.CreateTestAsset(t, h.db.AdminPool, h.orgID, externalKey)
	_, err := h.db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.tags (org_id, asset_id, type, value) VALUES ($1, $2, 'rfid', $3)`,
		h.orgID, a.ID, epc)
	require.NoError(t, err)
	return a.ID
}

// pointAt binds the reader's antenna-1 scan point to a location, so reads
// through it resolve there.
func (h *harness) pointAt(t *testing.T, locationID int) {
	t.Helper()
	_, err := h.db.AdminPool.Exec(context.Background(),
		`UPDATE trakrf.scan_points SET location_id = $1 WHERE org_id = $2 AND scan_device_id = $3`,
		locationID, h.orgID, h.device.ID)
	require.NoError(t, err)
}

// readAt drives the MQTT path: persist the read, then run detection on the
// resolved reads, exactly as the subscriber does.
func (h *harness) readAt(t *testing.T, epc string, at time.Time) {
	t.Helper()
	res, err := h.db.Store.PersistReads(context.Background(), h.orgID, h.device.ID, 0, at,
		[]scanread.Read{{EPC: epc, RSSI: -55, AntennaPort: 1}})
	require.NoError(t, err)
	h.evaluator.Evaluate(context.Background(), h.orgID, 0, at, res.Resolved)
}

// saveAt drives the handheld path.
func (h *harness) saveAt(t *testing.T, assetIDs []int, locationID int) {
	t.Helper()
	res, err := h.db.Store.SaveInventoryScans(context.Background(), h.orgID,
		storage.SaveInventoryRequest{LocationID: locationID, AssetIDs: assetIDs})
	require.NoError(t, err)
	h.evaluator.EvaluateScans(context.Background(), h.orgID, assetIDs, locationID, res.Timestamp)
}

// expectDelivery waits for one delivery and decodes its envelope.
func (h *harness) expectDelivery(t *testing.T) map[string]any {
	t.Helper()
	select {
	case d := <-h.deliveries:
		var env map[string]any
		require.NoError(t, json.Unmarshal(d.body, &env), string(d.body))
		return env
	case <-time.After(5 * time.Second):
		t.Fatal("expected a webhook delivery, got none")
		return nil
	}
}

// expectNoDelivery asserts silence. Short window: detection is synchronous up
// to the enqueue, so a delivery that is going to happen happens promptly.
func (h *harness) expectNoDelivery(t *testing.T) {
	t.Helper()
	select {
	case d := <-h.deliveries:
		t.Fatalf("expected no webhook delivery, got %s", string(d.body))
	case <-time.After(750 * time.Millisecond):
	}
}

func TestHandheldSaveEmitsAssetMoved(t *testing.T) {
	h := newHarness(t)
	dock := h.location(t, "E2E-DOCK", "Dock")
	assetID := h.asset(t, "E2E-FORK", epcA)

	h.saveAt(t, []int{assetID}, dock)

	env := h.expectDelivery(t)
	require.Equal(t, "asset.moved", env["event"])
	data := env["data"].(map[string]any)
	require.Equal(t, float64(assetID), data["asset"].(map[string]any)["id"])
	require.Equal(t, "E2E-FORK", data["asset"].(map[string]any)["external_key"])
	require.Nil(t, data["from_location"], "first-ever sighting has a null origin")
	require.Equal(t, "Dock", data["to_location"].(map[string]any)["name"])
}

func TestHandheldSameLocationEmitsNothing(t *testing.T) {
	h := newHarness(t)
	dock := h.location(t, "E2E-DOCK", "Dock")
	assetID := h.asset(t, "E2E-FORK", epcA)

	h.saveAt(t, []int{assetID}, dock)
	h.expectDelivery(t)

	// Rescanned in the same place: pure delta means silence, not a heartbeat.
	h.saveAt(t, []int{assetID}, dock)
	h.expectNoDelivery(t)
}

func TestHandheldMoveEmitsFromAndTo(t *testing.T) {
	h := newHarness(t)
	dock := h.location(t, "E2E-DOCK", "Dock")
	bay := h.location(t, "E2E-BAY", "Bay 3")
	assetID := h.asset(t, "E2E-FORK", epcA)

	h.saveAt(t, []int{assetID}, dock)
	h.expectDelivery(t)

	h.saveAt(t, []int{assetID}, bay)
	env := h.expectDelivery(t)
	data := env["data"].(map[string]any)
	require.Equal(t, "Dock", data["from_location"].(map[string]any)["name"])
	require.Equal(t, "Bay 3", data["to_location"].(map[string]any)["name"])
}

func TestFixedReaderPathEmitsAssetMoved(t *testing.T) {
	h := newHarness(t)
	doorway := h.location(t, "E2E-DOOR", "Doorway")
	h.pointAt(t, doorway)
	h.asset(t, "E2E-TAGGED", epcA)

	h.readAt(t, epcA, time.Now())

	env := h.expectDelivery(t)
	data := env["data"].(map[string]any)
	require.Equal(t, "Doorway", data["to_location"].(map[string]any)["name"])
	require.Equal(t, "E2E-TAGGED", data["asset"].(map[string]any)["external_key"])

	// The wire carries logical data only.
	body, _ := json.Marshal(env)
	require.NotContains(t, string(body), epcA)
	require.NotContains(t, string(body), "scan_point")
}

// The case that fails SILENTLY without the base-table tail. The continuous
// aggregate lags ~1-1.5 minutes and is never refreshed here, so if detection
// read only the CAGG the second move's origin would still be the first
// location — and when a stale origin happens to equal the new location, a real
// move is suppressed with no error anywhere.
func TestMoveInsideCAGGLagWindowIsStillDetected(t *testing.T) {
	h := newHarness(t)
	dock := h.location(t, "E2E-DOCK", "Dock")
	bay := h.location(t, "E2E-BAY", "Bay 3")
	assetID := h.asset(t, "E2E-FORK", epcA)

	// Three saves seconds apart, well inside the CAGG's materialization lag.
	h.saveAt(t, []int{assetID}, dock)
	first := h.expectDelivery(t)
	require.Nil(t, first["data"].(map[string]any)["from_location"])

	h.saveAt(t, []int{assetID}, bay)
	second := h.expectDelivery(t)
	require.Equal(t, "Dock", second["data"].(map[string]any)["from_location"].(map[string]any)["name"])

	// Back to the dock: the origin must be Bay 3, which only the base-table
	// tail knows about.
	h.saveAt(t, []int{assetID}, dock)
	third := h.expectDelivery(t)
	require.Equal(t, "Bay 3", third["data"].(map[string]any)["from_location"].(map[string]any)["name"])
	require.Equal(t, "Dock", third["data"].(map[string]any)["to_location"].(map[string]any)["name"])

	// And a rescan at the dock is silent — proving the tail did not merely
	// invent a fresh origin every time.
	h.saveAt(t, []int{assetID}, dock)
	h.expectNoDelivery(t)
}

func TestSignatureVerifiesEndToEnd(t *testing.T) {
	h := newHarness(t)
	dock := h.location(t, "E2E-DOCK", "Dock")
	assetID := h.asset(t, "E2E-FORK", epcA)

	h.saveAt(t, []int{assetID}, dock)

	select {
	case d := <-h.deliveries:
		ts := d.headers.Get("X-TrakRF-Timestamp")
		require.NotEmpty(t, ts)
		require.Equal(t, webhook.Sign("whsec_e2e", ts, d.body), d.headers.Get("X-TrakRF-Signature"))
		require.NotEmpty(t, d.headers.Get("X-TrakRF-Delivery"))
	case <-time.After(5 * time.Second):
		t.Fatal("expected a webhook delivery, got none")
	}
}

// An org that registered a webhook during a trial and then stopped paying must
// stop receiving events. Delivery is outbound, so no middleware sees it — the
// sink's entitlement check is the only thing standing here.
func TestUnentitledOrgReceivesNothing(t *testing.T) {
	h := newHarness(t)
	dock := h.location(t, "E2E-DOCK", "Dock")
	assetID := h.asset(t, "E2E-FORK", epcA)

	_, err := h.db.AdminPool.Exec(context.Background(),
		`UPDATE trakrf.organizations SET subscription_enabled = false WHERE id = $1`, h.orgID)
	require.NoError(t, err)

	h.saveAt(t, []int{assetID}, dock)
	h.expectNoDelivery(t)

	// Re-entitling resumes delivery on the NEXT qualifying scan, with no
	// backlog from the lapsed window.
	_, err = h.db.AdminPool.Exec(context.Background(),
		`UPDATE trakrf.organizations SET subscription_enabled = true WHERE id = $1`, h.orgID)
	require.NoError(t, err)

	bay := h.location(t, "E2E-BAY", "Bay 3")
	h.saveAt(t, []int{assetID}, bay)
	env := h.expectDelivery(t)
	require.Equal(t, "Bay 3", env["data"].(map[string]any)["to_location"].(map[string]any)["name"])
	h.expectNoDelivery(t)
}

func TestDisabledWebhookReceivesNothing(t *testing.T) {
	h := newHarness(t)
	dock := h.location(t, "E2E-DOCK", "Dock")
	assetID := h.asset(t, "E2E-FORK", epcA)

	wh, err := h.db.Store.GetWebhook(context.Background(), h.orgID)
	require.NoError(t, err)
	off := false
	_, err = h.db.Store.UpdateWebhook(context.Background(), h.orgID, wh.ID, webhookmodel.UpdateRequest{Enabled: &off})
	require.NoError(t, err)

	h.saveAt(t, []int{assetID}, dock)
	h.expectNoDelivery(t)
}

func TestNoWebhookRegisteredIsHarmless(t *testing.T) {
	h := newHarness(t)
	dock := h.location(t, "E2E-DOCK", "Dock")
	assetID := h.asset(t, "E2E-FORK", epcA)

	wh, err := h.db.Store.GetWebhook(context.Background(), h.orgID)
	require.NoError(t, err)
	_, err = h.db.Store.DeleteWebhook(context.Background(), h.orgID, wh.ID)
	require.NoError(t, err)

	require.NotPanics(t, func() { h.saveAt(t, []int{assetID}, dock) })
	h.expectNoDelivery(t)
}
