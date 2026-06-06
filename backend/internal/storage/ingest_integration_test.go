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

// registerDevice creates a CS463 device (auto-provisions scan_point
// {externalKey}-1) and returns nothing; publish_topic defaults to
// trakrf.id/{externalKey}/reads.
func registerDevice(t *testing.T, db *testutil.TestDB, orgID int, externalKey string) {
	t.Helper()
	_, err := db.Store.CreateScanDevice(context.Background(), orgID, scandevice.CreateScanDeviceRequest{
		ExternalKey: externalKey, Name: "Test Reader", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)
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

// registerGLS10Device creates a GL-S10 BLE gateway (auto-provisions scan_point
// {externalKey}-1, same TRA-899 invariant as CS463). external_key must equal
// the gateway's dev_ble_mac for parsed reads' {dev_ble_mac}-1 capture point to
// match.
func registerGLS10Device(t *testing.T, db *testutil.TestDB, orgID int, externalKey string) {
	t.Helper()
	_, err := db.Store.CreateScanDevice(context.Background(), orgID, scandevice.CreateScanDeviceRequest{
		ExternalKey: externalKey, Name: "Test Gateway", Type: scandevice.DeviceTypeGLS10,
	})
	require.NoError(t, err)
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
	registerDevice(t, db, orgID, "cs463-214")

	route, found, err := db.Store.ResolveScanTopic(ctx, "trakrf.id/cs463-214/reads")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, orgID, route.OrgID)
	assert.Equal(t, scandevice.DeviceTypeCS463, route.DeviceType)
}

func TestResolveScanTopic_ByExternalKeyDefault(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// Device with an explicitly NULL publish_topic: resolution must fall back to
	// the documented default topic trakrf.id/{external_key}/reads.
	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.scan_devices (org_id, external_key, name, type, transport, publish_topic)
		 VALUES ($1, 'cs463-299', 'Null-Topic Reader', 'csl_cs463', 'mqtt', NULL)`, orgID)
	require.NoError(t, err)

	route, found, err := db.Store.ResolveScanTopic(ctx, "trakrf.id/cs463-299/reads")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, orgID, route.OrgID)
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

// setBoundary marks a scan_point (by external_key) as a geofence boundary and
// optionally sets a per-point RSSI threshold in metadata.
func setBoundary(t *testing.T, db *testutil.TestDB, orgID int, externalKey string) {
	t.Helper()
	_, err := db.AdminPool.Exec(context.Background(),
		`UPDATE trakrf.scan_points SET is_boundary = true WHERE org_id = $1 AND external_key = $2`,
		orgID, externalKey)
	require.NoError(t, err)
}

func TestPersistReads_RegisteredAssetProducesScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	registerDevice(t, db, orgID, "cs463-214")
	registerRFIDTag(t, db, orgID, testEPC)

	tagScanID, err := db.Store.InsertRawTagScan(ctx, "trakrf.id/cs463-214/reads", []byte(`{}`))
	require.NoError(t, err)

	receivedAt := time.Now()
	reads := []scanread.Read{{EPC: testEPC, CapturePointName: "cs463-214-1", AntennaPort: 1, RSSI: -56}}
	res, err := db.Store.PersistReads(ctx, orgID, tagScanID, receivedAt, reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted)
	assert.Empty(t, res.Dropped)
	require.Equal(t, 1, countAssetScans(t, db, orgID))

	// Resolved enrichment for the geofence engine (TRA-901): one membership-passing
	// read, not a boundary by default, carrying the read's RSSI.
	require.Len(t, res.Resolved, 1)
	assert.False(t, res.Resolved[0].IsBoundary)
	assert.Equal(t, testEPC, res.Resolved[0].EPC)
	assert.Equal(t, -56, res.Resolved[0].RSSI)
	assert.Greater(t, res.Resolved[0].ScanPointID, 0)
	assert.Nil(t, res.Resolved[0].RSSIThresholdRaw, "no per-point override set by default")

	// Resolved to the registered scan_point and linked to the source audit row.
	var spExternalKey string
	var gotTagScanID int64
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		SELECT sp.external_key, a.tag_scan_id
		FROM trakrf.asset_scans a
		JOIN trakrf.scan_points sp ON sp.id = a.scan_point_id
		WHERE a.org_id = $1`, orgID).Scan(&spExternalKey, &gotTagScanID))
	assert.Equal(t, "cs463-214-1", spExternalKey)
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
		registerDevice(t, db, orgID, "cs463-214")
		registerRFIDTag(t, db, orgID, shortValue) // registered short

		reads := []scanread.Read{{EPC: fullEPC, CapturePointName: "cs463-214-1", RSSI: -56}}
		res, err := db.Store.PersistReads(ctx, orgID, 1, time.Now(), reads)
		require.NoError(t, err)
		assert.Equal(t, 1, res.Inserted, "full EPC read resolves the short-value tag")
		assert.Empty(t, res.Dropped)
		require.Equal(t, 1, countAssetScans(t, db, orgID))
	})

	t.Run("full tag value matches short EPC read", func(t *testing.T) {
		db := testutil.SetupTestDBFull(t)
		orgID := testutil.CreateTestAccount(t, db.AdminPool)
		registerDevice(t, db, orgID, "cs463-214")
		registerRFIDTag(t, db, orgID, fullEPC) // registered full

		reads := []scanread.Read{{EPC: shortValue, CapturePointName: "cs463-214-1", RSSI: -56}}
		res, err := db.Store.PersistReads(ctx, orgID, 1, time.Now(), reads)
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
		registerGLS10Device(t, db, orgID, "C4DEE229A176")
		registerBLETag(t, db, orgID, "c4dee229a176aa") // lowercase placeholder asset MAC

		// Reader-side parsers emit uppercase MAC; ensure it still resolves.
		reads := []scanread.Read{{EPC: "C4DEE229A176AA", CapturePointName: "C4DEE229A176-1", RSSI: -56}}
		res, err := db.Store.PersistReads(ctx, orgID, 1, time.Now(), reads)
		require.NoError(t, err)
		assert.Equal(t, 1, res.Inserted, "uppercase read resolves lowercase-registered MAC")
		assert.Empty(t, res.Dropped)
	})

	t.Run("non-hex junk value matches nothing", func(t *testing.T) {
		db := testutil.SetupTestDBFull(t)
		orgID := testutil.CreateTestAccount(t, db.AdminPool)
		registerDevice(t, db, orgID, "cs463-214")
		// "X With Space" normalizes to "" (no hex chars) and must not match a read.
		asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "junk-asset")
		_, err := db.AdminPool.Exec(ctx,
			`INSERT INTO trakrf.tags (org_id, asset_id, type, value) VALUES ($1, $2, 'rfid', 'X With Space')`,
			orgID, asset.ID)
		require.NoError(t, err)

		reads := []scanread.Read{{EPC: "AABBCC", CapturePointName: "cs463-214-1"}}
		res, err := db.Store.PersistReads(ctx, orgID, 1, time.Now(), reads)
		require.NoError(t, err)
		assert.Equal(t, 0, res.Inserted)
		assert.Equal(t, 1, res.Dropped["no_asset"], "junk tag value normalizes to empty and never matches")
	})
}

func TestPersistReads_UnregisteredEPCDropsRead(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	registerDevice(t, db, orgID, "cs463-214")
	// No rfid tag registered for testEPC.

	reads := []scanread.Read{{EPC: testEPC, CapturePointName: "cs463-214-1"}}
	res, err := db.Store.PersistReads(ctx, orgID, 1, time.Now(), reads)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Inserted)
	assert.Equal(t, 1, res.Dropped["no_asset"])
	assert.Equal(t, 0, countAssetScans(t, db, orgID), "membership filter: unregistered EPC records nothing")
}

func TestPersistReads_UnknownScanPointDropsRead(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	registerDevice(t, db, orgID, "cs463-214")
	registerRFIDTag(t, db, orgID, testEPC)

	reads := []scanread.Read{{EPC: testEPC, CapturePointName: "cs463-214-9"}} // not a registered capture point
	res, err := db.Store.PersistReads(ctx, orgID, 1, time.Now(), reads)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Inserted)
	assert.Equal(t, 1, res.Dropped["no_scan_point"])
	assert.Equal(t, 0, countAssetScans(t, db, orgID))
}

func TestPersistReads_DuplicateEPCInBatchDedups(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	registerDevice(t, db, orgID, "cs463-214")
	registerRFIDTag(t, db, orgID, testEPC)

	receivedAt := time.Now()
	reads := []scanread.Read{
		{EPC: testEPC, CapturePointName: "cs463-214-1"},
		{EPC: testEPC, CapturePointName: "cs463-214-1"},
	}
	res, err := db.Store.PersistReads(ctx, orgID, 1, receivedAt, reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted)
	assert.Equal(t, 1, res.Dropped["conflict"], "same (timestamp, org, asset) dedups on the content PK")
	assert.Equal(t, 1, countAssetScans(t, db, orgID))
	// Both reads passed membership, so both appear in Resolved even though the
	// second was an asset_scans dedup conflict — presence is the geofence signal.
	assert.Len(t, res.Resolved, 2, "conflict-deduped read still counts as a boundary observation")
}

func TestPersistReads_BoundaryAndPerPointThresholdResolved(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	registerDevice(t, db, orgID, "cs463-214")
	registerRFIDTag(t, db, orgID, testEPC)
	setBoundary(t, db, orgID, "cs463-214-1")
	_, err := db.AdminPool.Exec(ctx,
		`UPDATE trakrf.scan_points SET metadata = '{"rssi_threshold":"-55"}'::jsonb
		 WHERE org_id = $1 AND external_key = $2`, orgID, "cs463-214-1")
	require.NoError(t, err)

	reads := []scanread.Read{{EPC: testEPC, CapturePointName: "cs463-214-1", RSSI: -50}}
	res, err := db.Store.PersistReads(ctx, orgID, 1, time.Now(), reads)
	require.NoError(t, err)
	require.Len(t, res.Resolved, 1)
	assert.True(t, res.Resolved[0].IsBoundary, "scan_point marked boundary must surface IsBoundary")
	require.NotNil(t, res.Resolved[0].RSSIThresholdRaw)
	assert.Equal(t, "-55", *res.Resolved[0].RSSIThresholdRaw)
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
	registerGLS10Device(t, db, orgID, "C4DEE229A176") // auto scan_point C4DEE229A176-1
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
	res, err := db.Store.PersistReads(ctx, orgID, tagScanID, time.Now(), reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted, "registered asset MAC lands as a scan")
	assert.Equal(t, 1, res.Dropped["no_asset"], "unregistered BLE noise drops at membership")
	require.Equal(t, 1, countAssetScans(t, db, orgID))

	// The asset_scan resolved to the gateway's single auto-provisioned capture point.
	var spExternalKey string
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		SELECT sp.external_key FROM trakrf.asset_scans a
		JOIN trakrf.scan_points sp ON sp.id = a.scan_point_id
		WHERE a.org_id = $1`, orgID).Scan(&spExternalKey))
	assert.Equal(t, "C4DEE229A176-1", spExternalKey)
}

// TestGLS10_BLETagProducesScan mirrors TestGLS10_ParseToAssetScan but registers
// the asset MAC as a type='ble' tag — the natural registration for a BLE
// gateway — instead of the rfid workaround. Membership must resolve non-rfid tag
// classes; before TRA-927 the rfid-only predicate dropped this read silently.
func TestGLS10_BLETagProducesScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	registerGLS10Device(t, db, orgID, "C4DEE229A176") // auto scan_point C4DEE229A176-1
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
	res, err := db.Store.PersistReads(ctx, orgID, tagScanID, time.Now(), reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted, "ble-registered asset MAC lands as a scan")
	assert.Equal(t, 1, res.Dropped["no_asset"], "unregistered BLE noise still drops at membership")
	require.Equal(t, 1, countAssetScans(t, db, orgID))
}
