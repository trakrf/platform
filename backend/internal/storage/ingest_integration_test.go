//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/ingest"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-900: the Go subscriber replaces the process_tag_scans trigger. These
// tests exercise the storage half — topic→org resolution (SECURITY DEFINER) and
// the tag-based, no-auto-create asset_scans derivation — on the non-superuser
// RLS role, which is what proves the GUC-unset landmine is gone.

const testEPC = "E2801190A503006543E21224"

// publishTopic builds the routing key for a device key (TRA-956: publish_topic
// is set directly; there is no external_key default anymore).
func publishTopic(key string) string { return "trakrf.id/" + key + "/reads" }

// registerDevice creates a CS463 device publishing on trakrf.id/{key}/reads
// (auto-provisions antenna-1 scan_point) and returns it so the caller can pass
// its id to PersistReads.
func registerDevice(t *testing.T, db *testutil.TestDB, orgID int, key string) *scandevice.ScanDevice {
	t.Helper()
	topic := publishTopic(key)
	d, err := db.Store.CreateScanDevice(context.Background(), orgID, scandevice.CreateScanDeviceRequest{
		Name: "Test Reader", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	return d
}

// registerRFIDTag links an rfid tag value (EPC) to a new asset.
func registerRFIDTag(t *testing.T, db *testutil.TestDB, orgID int, epc string) {
	t.Helper()
	registerTag(t, db, orgID, "rfid", epc)
}

// registerBLETag links a ble tag value (a MAC) to a new asset — the natural
// registration for a BLE gateway's asset identity (TRA-927).
func registerBLETag(t *testing.T, db *testutil.TestDB, orgID int, mac string) {
	t.Helper()
	registerTag(t, db, orgID, "ble", mac)
}

// registerTag links a tag of the given type/value to a new asset.
func registerTag(t *testing.T, db *testutil.TestDB, orgID int, tagType, value string) {
	t.Helper()
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "asset-"+value)
	_, err := db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.tags (org_id, asset_id, type, value) VALUES ($1, $2, $3, $4)`,
		orgID, asset.ID, tagType, value)
	require.NoError(t, err)
}

// registerGLS10Device creates a GL-S10 BLE gateway publishing on
// trakrf.id/{key}/reads (auto-provisions antenna-1 scan_point, same TRA-899
// invariant as CS463) and returns it. The gateway is a single capture point, so
// its reads resolve to antenna 1 (TRA-956).
func registerGLS10Device(t *testing.T, db *testutil.TestDB, orgID int, key string) *scandevice.ScanDevice {
	t.Helper()
	topic := publishTopic(key)
	d, err := db.Store.CreateScanDevice(context.Background(), orgID, scandevice.CreateScanDeviceRequest{
		Name: "Test Gateway", Type: scandevice.DeviceTypeGLS10, PublishTopic: &topic,
	})
	require.NoError(t, err)
	return d
}

func countAssetScans(t *testing.T, db *testutil.TestDB, orgID int) int {
	t.Helper()
	var n int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`SELECT count(*) FROM trakrf.asset_scans WHERE org_id = $1`, orgID).Scan(&n))
	return n
}

func TestResolveScanTopic_ByPublishTopic(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "cs463-214")

	route, found, err := db.Store.ResolveScanTopic(ctx, "trakrf.id/cs463-214/reads")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, orgID, route.OrgID)
	assert.Equal(t, dev.ID, route.ScanDeviceID)
	assert.Equal(t, scandevice.DeviceTypeCS463, route.DeviceType)
}

func TestListScanTopics(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	devA := registerDevice(t, db, orgID, "cs463-a")
	devB := registerGLS10Device(t, db, orgID, "gls10-b")
	// A web_ble (handheld) device has no MQTT topic and must be excluded.
	_, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Handheld", Type: scandevice.DeviceTypeCS463, Transport: scandevice.TransportWebBLE,
	})
	require.NoError(t, err)

	topics, err := db.Store.ListScanTopics(ctx)
	require.NoError(t, err)

	rA, okA := topics["trakrf.id/cs463-a/reads"]
	require.True(t, okA, "device A topic must be listed")
	assert.Equal(t, orgID, rA.OrgID)
	assert.Equal(t, devA.ID, rA.ScanDeviceID)
	assert.Equal(t, scandevice.DeviceTypeCS463, rA.DeviceType)

	rB, okB := topics["trakrf.id/gls10-b/reads"]
	require.True(t, okB, "device B topic must be listed")
	assert.Equal(t, devB.ID, rB.ScanDeviceID)
	assert.Equal(t, scandevice.DeviceTypeGLS10, rB.DeviceType)

	// Exactly the two mqtt topics — web_ble device excluded.
	assert.Len(t, topics, 2)

	// Soft-deleting a device drops it from the active set.
	ok, err := db.Store.DeleteScanDevice(ctx, orgID, devA.ID)
	require.NoError(t, err)
	require.True(t, ok)
	topics, err = db.Store.ListScanTopics(ctx)
	require.NoError(t, err)
	_, stillThere := topics["trakrf.id/cs463-a/reads"]
	assert.False(t, stillThere, "soft-deleted device must drop out")
	assert.Len(t, topics, 1)
}

func TestResolveScanTopic_UnknownTopic(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	_, found, err := db.Store.ResolveScanTopic(ctx, "trakrf.id/does-not-exist/reads")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestInsertRawTagScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	id, err := db.Store.InsertRawTagScan(ctx, "trakrf.id/cs463-214/reads", []byte(`{"tags":[]}`))
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

func TestPersistReads_RegisteredAssetProducesScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "cs463-214")
	registerRFIDTag(t, db, orgID, testEPC)

	tagScanID, err := db.Store.InsertRawTagScan(ctx, "trakrf.id/cs463-214/reads", []byte(`{}`))
	require.NoError(t, err)

	receivedAt := time.Now()
	reads := []scanread.Read{{EPC: testEPC, AntennaPort: 1, RSSI: -56}}
	res, err := db.Store.PersistReads(ctx, orgID, dev.ID, tagScanID, receivedAt, reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted)
	assert.Empty(t, res.Dropped)
	require.Equal(t, 1, countAssetScans(t, db, orgID))

	// Resolved enrichment for the geofence engine (TRA-901): one membership-passing
	// read carrying the read's RSSI. Rule config (mode/age-out/rssi) lives on the
	// output device now (TRA-943), not on the resolved read.
	require.Len(t, res.Resolved, 1)
	assert.Equal(t, testEPC, res.Resolved[0].EPC)
	assert.Equal(t, -56, res.Resolved[0].RSSI)
	assert.Greater(t, res.Resolved[0].ScanPointID, 0)

	// Resolved to the device's antenna-1 scan_point and linked to the source audit row.
	var spDeviceID int
	var spAntenna int
	var gotTagScanID int64
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		SELECT sp.scan_device_id, sp.antenna_port, a.tag_scan_id
		FROM trakrf.asset_scans a
		JOIN trakrf.scan_points sp ON sp.id = a.scan_point_id
		WHERE a.org_id = $1`, orgID).Scan(&spDeviceID, &spAntenna, &gotTagScanID))
	assert.Equal(t, dev.ID, spDeviceID)
	assert.Equal(t, 1, spAntenna)
	assert.Equal(t, tagScanID, gotTagScanID)
}

// TRA-944: a tag registered by its short barcode value resolves a full-width EPC
// read (and the reverse), matching the handheld getMatchingKey normalization.
func TestPersistReads_LeadingZeroNormalizedMatch(t *testing.T) {
	ctx := context.Background()

	const fullEPC = "000000000000000000010023"
	const shortValue = "10023"

	// Fresh DB per subtest: CreateTestAccount uses a fixed org identifier, so each
	// scenario needs its own database (one account per DB). Separate orgs also keep
	// the short and full registrations from colliding on normalized_value.
	t.Run("short tag value matches full EPC read", func(t *testing.T) {
		db := testutil.SetupTestDBFull(t)
		orgID := testutil.CreateTestAccount(t, db.AdminPool)
		dev := registerDevice(t, db, orgID, "cs463-214")
		registerRFIDTag(t, db, orgID, shortValue) // registered short

		reads := []scanread.Read{{EPC: fullEPC, AntennaPort: 1, RSSI: -56}}
		res, err := db.Store.PersistReads(ctx, orgID, dev.ID, 1, time.Now(), reads)
		require.NoError(t, err)
		assert.Equal(t, 1, res.Inserted, "full EPC read resolves the short-value tag")
		assert.Empty(t, res.Dropped)
		require.Equal(t, 1, countAssetScans(t, db, orgID))
	})

	t.Run("full tag value matches short EPC read", func(t *testing.T) {
		db := testutil.SetupTestDBFull(t)
		orgID := testutil.CreateTestAccount(t, db.AdminPool)
		dev := registerDevice(t, db, orgID, "cs463-214")
		registerRFIDTag(t, db, orgID, fullEPC) // registered full

		reads := []scanread.Read{{EPC: shortValue, AntennaPort: 1, RSSI: -56}}
		res, err := db.Store.PersistReads(ctx, orgID, dev.ID, 1, time.Now(), reads)
		require.NoError(t, err)
		assert.Equal(t, 1, res.Inserted, "short EPC read resolves the full-value tag")
		assert.Empty(t, res.Dropped)
	})
}

// TRA-944: BLE MAC registered lowercase resolves an uppercase read, and non-hex
// junk tag values never match anything.
func TestPersistReads_NormalizationEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("ble mac case-insensitive", func(t *testing.T) {
		db := testutil.SetupTestDBFull(t)
		orgID := testutil.CreateTestAccount(t, db.AdminPool)
		dev := registerGLS10Device(t, db, orgID, "C4DEE229A176")
		registerBLETag(t, db, orgID, "c4dee229a176aa") // lowercase placeholder asset MAC

		// Reader-side parsers emit uppercase MAC; ensure it still resolves.
		reads := []scanread.Read{{EPC: "C4DEE229A176AA", AntennaPort: 1, RSSI: -56}}
		res, err := db.Store.PersistReads(ctx, orgID, dev.ID, 1, time.Now(), reads)
		require.NoError(t, err)
		assert.Equal(t, 1, res.Inserted, "uppercase read resolves lowercase-registered MAC")
		assert.Empty(t, res.Dropped)
	})

	t.Run("non-hex junk value matches nothing", func(t *testing.T) {
		db := testutil.SetupTestDBFull(t)
		orgID := testutil.CreateTestAccount(t, db.AdminPool)
		dev := registerDevice(t, db, orgID, "cs463-214")
		// "X With Space" normalizes to "" (no hex chars) and must not match a read.
		asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "junk-asset")
		_, err := db.AdminPool.Exec(ctx,
			`INSERT INTO trakrf.tags (org_id, asset_id, type, value) VALUES ($1, $2, 'rfid', 'X With Space')`,
			orgID, asset.ID)
		require.NoError(t, err)

		reads := []scanread.Read{{EPC: "AABBCC", AntennaPort: 1}}
		res, err := db.Store.PersistReads(ctx, orgID, dev.ID, 1, time.Now(), reads)
		require.NoError(t, err)
		assert.Equal(t, 0, res.Inserted)
		assert.Equal(t, 1, res.Dropped["no_asset"], "junk tag value normalizes to empty and never matches")
	})
}

func TestPersistReads_UnregisteredEPCDropsRead(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "cs463-214")
	// No rfid tag registered for testEPC.

	reads := []scanread.Read{{EPC: testEPC, AntennaPort: 1}}
	res, err := db.Store.PersistReads(ctx, orgID, dev.ID, 1, time.Now(), reads)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Inserted)
	assert.Equal(t, 1, res.Dropped["no_asset"])
	assert.Equal(t, 0, countAssetScans(t, db, orgID), "membership filter: unregistered EPC records nothing")
}

func TestPersistReads_UnknownScanPointDropsRead(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "cs463-214")
	registerRFIDTag(t, db, orgID, testEPC)

	// Device only has the auto-provisioned antenna 1; a read on antenna 9 has no
	// scan_point and is a clean no_scan_point miss (TRA-956).
	reads := []scanread.Read{{EPC: testEPC, AntennaPort: 9}}
	res, err := db.Store.PersistReads(ctx, orgID, dev.ID, 1, time.Now(), reads)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Inserted)
	assert.Equal(t, 1, res.Dropped["no_scan_point"])
	assert.Equal(t, 0, countAssetScans(t, db, orgID))
}

func TestPersistReads_DuplicateEPCInBatchDedups(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "cs463-214")
	registerRFIDTag(t, db, orgID, testEPC)

	receivedAt := time.Now()
	reads := []scanread.Read{
		{EPC: testEPC, AntennaPort: 1},
		{EPC: testEPC, AntennaPort: 1},
	}
	res, err := db.Store.PersistReads(ctx, orgID, dev.ID, 1, receivedAt, reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted)
	assert.Equal(t, 1, res.Dropped["conflict"], "same (timestamp, org, asset) dedups on the content PK")
	assert.Equal(t, 1, countAssetScans(t, db, orgID))
	// Both reads passed membership, so both appear in Resolved even though the
	// second was an asset_scans dedup conflict — presence is the geofence signal.
	assert.Len(t, res.Resolved, 2, "conflict-deduped read still counts as a boundary observation")
}

// TestGLS10_ParseToAssetScan exercises the full GL-S10 path end to end (TRA-925):
// a real-shaped BLE gateway payload parses via ingest.Parse, and the read whose
// MAC is a registered rfid tag lands in asset_scans on the gateway's
// auto-provisioned {dev_ble_mac}-1 capture point, while unregistered BLE noise
// drops at membership. Pins both GL-S10 provisioning contracts.
func TestGLS10_ParseToAssetScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerGLS10Device(t, db, orgID, "C4DEE229A176") // auto antenna-1 scan_point
	const assetMAC = "F95BC0EC4E56"
	registerRFIDTag(t, db, orgID, assetMAC)

	// Two BLE devices seen: the registered asset MAC and unregistered noise.
	payload := []byte(`{"dev_ble_mac":"C4DEE229A176","dev_list":[
		{"mac":"F95BC0EC4E56","ad":"0201","ts":1780625164824,"rssi":-57},
		{"mac":"DEADBEEFCAFE","ad":"0201","ts":1780625164900,"rssi":-80}
	]}`)
	reads, err := ingest.Parse(scandevice.DeviceTypeGLS10, payload)
	require.NoError(t, err)
	require.Len(t, reads, 2)

	tagScanID, err := db.Store.InsertRawTagScan(ctx, "trakrf.id/C4DEE229A176/reads", payload)
	require.NoError(t, err)
	res, err := db.Store.PersistReads(ctx, orgID, dev.ID, tagScanID, time.Now(), reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted, "registered asset MAC lands as a scan")
	assert.Equal(t, 1, res.Dropped["no_asset"], "unregistered BLE noise drops at membership")
	require.Equal(t, 1, countAssetScans(t, db, orgID))

	// The asset_scan resolved to the gateway's single auto-provisioned antenna-1 scan_point.
	var spDeviceID, spAntenna int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		SELECT sp.scan_device_id, sp.antenna_port FROM trakrf.asset_scans a
		JOIN trakrf.scan_points sp ON sp.id = a.scan_point_id
		WHERE a.org_id = $1`, orgID).Scan(&spDeviceID, &spAntenna))
	assert.Equal(t, dev.ID, spDeviceID)
	assert.Equal(t, 1, spAntenna)
}

// TestGLS10_BLETagProducesScan mirrors TestGLS10_ParseToAssetScan but registers
// the asset MAC as a type='ble' tag — the natural registration for a BLE
// gateway — instead of the rfid workaround. Membership must resolve non-rfid tag
// classes; before TRA-927 the rfid-only predicate dropped this read silently.
func TestGLS10_BLETagProducesScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerGLS10Device(t, db, orgID, "C4DEE229A176") // auto antenna-1 scan_point
	const assetMAC = "F95BC0EC4E56"
	registerBLETag(t, db, orgID, assetMAC) // registered as ble, not rfid

	payload := []byte(`{"dev_ble_mac":"C4DEE229A176","dev_list":[
		{"mac":"F95BC0EC4E56","ad":"0201","ts":1780625164824,"rssi":-57},
		{"mac":"DEADBEEFCAFE","ad":"0201","ts":1780625164900,"rssi":-80}
	]}`)
	reads, err := ingest.Parse(scandevice.DeviceTypeGLS10, payload)
	require.NoError(t, err)
	require.Len(t, reads, 2)

	tagScanID, err := db.Store.InsertRawTagScan(ctx, "trakrf.id/C4DEE229A176/reads", payload)
	require.NoError(t, err)
	res, err := db.Store.PersistReads(ctx, orgID, dev.ID, tagScanID, time.Now(), reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted, "ble-registered asset MAC lands as a scan")
	assert.Equal(t, 1, res.Dropped["no_asset"], "unregistered BLE noise still drops at membership")
	require.Equal(t, 1, countAssetScans(t, db, orgID))
}
