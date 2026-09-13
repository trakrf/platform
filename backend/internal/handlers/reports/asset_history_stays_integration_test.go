//go:build integration
// +build integration

// GET /api/v1/assets/{asset_id}/history returns STAYS, not scans: one item per
// unbroken run of observations at one location. A tag parked in front of a
// reader writes a row every minute, and a per-scan history rendered that as a
// timeline of identical one-minute entries.
//
// Every item is described independently of the requested window. A stay that
// began before `from` reports when it really began, and a stay still running at
// `to` reports when the asset was next seen somewhere else — or null when it
// never was. The window only decides WHICH stays are listed.
//
// History reads the asset_scan_latest continuous aggregate plus the raw fresh
// tail, so tests that seed older than the tail must refresh the aggregate.

package reports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// getHistory runs the history endpoint with the given query and decodes it.
func getHistory(t *testing.T, h *Handler, orgID, assetID int, query url.Values) historyResp {
	t.Helper()
	target := fmt.Sprintf("/api/v1/assets/%d/history", assetID)
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	setupTemporalReportsRouter(h).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp historyResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

type staysFixture struct {
	store   *storage.Storage
	pool    *pgxpool.Pool
	orgID   int
	assetID int
	base    time.Time
	loc     func(key string) int
}

func newStaysFixture(t *testing.T, assetKey string) (*staysFixture, func()) {
	t.Helper()
	store, cleanup := testutil.SetupTestDB(t)
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	base := dwellBase()
	validFrom := base.Add(-24 * time.Hour)
	assetID := seedAssetForReports(t, pool, orgID, assetKey, validFrom, nil)

	locs := map[string]int{}
	f := &staysFixture{
		store: store, pool: pool, orgID: orgID, assetID: assetID, base: base,
		loc: func(key string) int {
			if id, ok := locs[key]; ok {
				return id
			}
			id := seedLocationForReports(t, pool, orgID, assetKey+"-"+key, validFrom, nil)
			locs[key] = id
			return id
		},
	}
	return f, cleanup
}

func (f *staysFixture) scan(t *testing.T, locKey string, minute int) {
	t.Helper()
	seedScan(t, f.pool, f.orgID, f.assetID, f.loc(locKey), f.base.Add(time.Duration(minute)*time.Minute))
}

func (f *staysFixture) at(minute int) time.Time {
	return f.base.Add(time.Duration(minute) * time.Minute)
}

func TestAssetHistory_ConsecutiveScansAtOneLocationAreOneStay(t *testing.T) {
	f, cleanup := newStaysFixture(t, "ST-PARKED")
	defer cleanup()

	for m := 0; m < 10; m++ {
		f.scan(t, "L1", m)
	}
	testutil.RefreshAssetScanLatest(t, f.pool)

	resp := getHistory(t, NewHandler(f.store), f.orgID, f.assetID, nil)

	require.Len(t, resp.Data, 1, "ten minutes parked at one location is one stay")
	assert.Equal(t, 1, resp.TotalCount)
	stay := resp.Data[0]
	assert.WithinDuration(t, f.at(0), stay.EventObservedAt.Time, time.Second, "the stay starts at the first scan")
	assert.Nil(t, stay.DurationSeconds, "never seen anywhere else since, so the stay is open")
	require.NotNil(t, stay.LocationID)
	assert.Equal(t, f.loc("L1"), *stay.LocationID)
}

func TestAssetHistory_ReturningToAnEarlierLocationIsANewStay(t *testing.T) {
	f, cleanup := newStaysFixture(t, "ST-RETURN")
	defer cleanup()

	for _, m := range []int{0, 1, 2} {
		f.scan(t, "L1", m)
	}
	for _, m := range []int{3, 4} {
		f.scan(t, "L2", m)
	}
	for _, m := range []int{5, 6, 7, 8} {
		f.scan(t, "L1", m)
	}
	testutil.RefreshAssetScanLatest(t, f.pool)

	resp := getHistory(t, NewHandler(f.store), f.orgID, f.assetID, nil)

	require.Len(t, resp.Data, 3)
	assert.Equal(t, 3, resp.TotalCount)

	// Default order is newest first.
	assert.Equal(t, f.loc("L1"), *resp.Data[0].LocationID)
	assert.WithinDuration(t, f.at(5), resp.Data[0].EventObservedAt.Time, time.Second)
	assert.Nil(t, resp.Data[0].DurationSeconds)

	assert.Equal(t, f.loc("L2"), *resp.Data[1].LocationID)
	assert.WithinDuration(t, f.at(3), resp.Data[1].EventObservedAt.Time, time.Second)
	require.NotNil(t, resp.Data[1].DurationSeconds)
	assert.Equal(t, 120, *resp.Data[1].DurationSeconds, "L2 lasted until the asset was next seen at L1")

	assert.Equal(t, f.loc("L1"), *resp.Data[2].LocationID)
	assert.WithinDuration(t, f.at(0), resp.Data[2].EventObservedAt.Time, time.Second)
	require.NotNil(t, resp.Data[2].DurationSeconds)
	assert.Equal(t, 180, *resp.Data[2].DurationSeconds)
}

func TestAssetHistory_ScanWithNoLocationBreaksAStay(t *testing.T) {
	f, cleanup := newStaysFixture(t, "ST-NULL")
	defer cleanup()

	f.scan(t, "L1", 0)
	seedScanNoLocation(t, f.pool, f.orgID, f.assetID, f.at(1))
	f.scan(t, "L1", 2)
	testutil.RefreshAssetScanLatest(t, f.pool)

	resp := getHistory(t, NewHandler(f.store), f.orgID, f.assetID, nil)

	require.Len(t, resp.Data, 3, "a scan that resolved to no location is its own state")
	assert.Nil(t, resp.Data[1].LocationID)
}

func TestAssetHistory_TotalCountAndPagingCountStays(t *testing.T) {
	f, cleanup := newStaysFixture(t, "ST-PAGE")
	defer cleanup()

	f.scan(t, "L1", 0)
	f.scan(t, "L1", 1)
	f.scan(t, "L2", 2)
	f.scan(t, "L2", 3)
	f.scan(t, "L3", 4)
	f.scan(t, "L3", 5)
	testutil.RefreshAssetScanLatest(t, f.pool)
	h := NewHandler(f.store)

	walk := func(sort string) []int {
		var got []int
		for offset := 0; offset < 3; offset++ {
			q := url.Values{"limit": {"1"}, "offset": {fmt.Sprint(offset)}, "sort": {sort}}
			resp := getHistory(t, h, f.orgID, f.assetID, q)
			assert.Equal(t, 3, resp.TotalCount, "total_count counts stays, not scans")
			require.Len(t, resp.Data, 1)
			got = append(got, *resp.Data[0].LocationID)
		}
		return got
	}

	assert.Equal(t, []int{f.loc("L1"), f.loc("L2"), f.loc("L3")}, walk("event_observed_at"))
	assert.Equal(t, []int{f.loc("L3"), f.loc("L2"), f.loc("L1")}, walk("-event_observed_at"))
}

func TestAssetHistory_StayStartedBeforeFromReportsItsRealStart(t *testing.T) {
	f, cleanup := newStaysFixture(t, "ST-FROM")
	defer cleanup()

	f.scan(t, "L2", 0)
	for m := 10; m <= 60; m += 10 {
		f.scan(t, "L1", m)
	}
	testutil.RefreshAssetScanLatest(t, f.pool)

	q := url.Values{"from": {f.at(30).Format(time.RFC3339)}}
	resp := getHistory(t, NewHandler(f.store), f.orgID, f.assetID, q)

	require.Len(t, resp.Data, 1, "only the stay observed inside the window is listed")
	assert.Equal(t, f.loc("L1"), *resp.Data[0].LocationID)
	assert.WithinDuration(t, f.at(10), resp.Data[0].EventObservedAt.Time, time.Second,
		"the stay began at minute 10, before the window opened — not at the window edge")
}

func TestAssetHistory_StayOutlivingToEndsAtTheNextMove(t *testing.T) {
	f, cleanup := newStaysFixture(t, "ST-TO-MOVED")
	defer cleanup()

	f.scan(t, "L1", 0)
	f.scan(t, "L1", 10)
	f.scan(t, "L1", 20)
	f.scan(t, "L2", 40)
	testutil.RefreshAssetScanLatest(t, f.pool)

	q := url.Values{"to": {f.at(30).Format(time.RFC3339)}}
	resp := getHistory(t, NewHandler(f.store), f.orgID, f.assetID, q)

	require.Len(t, resp.Data, 1)
	require.NotNil(t, resp.Data[0].DurationSeconds,
		"the asset moved after `to`; a window that ends early must not turn a closed stay into an ongoing one")
	assert.Equal(t, 2400, *resp.Data[0].DurationSeconds, "L1 lasted until L2 at minute 40")
}

func TestAssetHistory_StayStillRunningPastToIsOpen(t *testing.T) {
	f, cleanup := newStaysFixture(t, "ST-TO-STAYED")
	defer cleanup()

	f.scan(t, "L1", 0)
	f.scan(t, "L1", 20)
	f.scan(t, "L1", 40)
	testutil.RefreshAssetScanLatest(t, f.pool)

	q := url.Values{"to": {f.at(10).Format(time.RFC3339)}}
	resp := getHistory(t, NewHandler(f.store), f.orgID, f.assetID, q)

	require.Len(t, resp.Data, 1)
	assert.Nil(t, resp.Data[0].DurationSeconds, "never seen anywhere else, even after `to`")
}

// The aggregate lags the raw table by up to ~2 minutes. A move recorded only in
// the raw tail must still appear, and a stay that spans the aggregate and the
// tail must still be one stay.
func TestAssetHistory_FreshTailJoinsMaterializedHistory(t *testing.T) {
	f, cleanup := newStaysFixture(t, "ST-TAIL")
	defer cleanup()

	now := time.Now().UTC().Truncate(time.Minute)
	parkedSince := now.Add(-30 * time.Minute)
	seedScan(t, f.pool, f.orgID, f.assetID, f.loc("L1"), parkedSince)
	testutil.RefreshAssetScanLatest(t, f.pool)

	// Not materialized: only the fresh tail can see these.
	seedScan(t, f.pool, f.orgID, f.assetID, f.loc("L1"), now.Add(-1*time.Minute))
	h := NewHandler(f.store)

	resp := getHistory(t, h, f.orgID, f.assetID, nil)
	require.Len(t, resp.Data, 1, "the same stay continues across the aggregate/tail boundary")
	assert.WithinDuration(t, parkedSince, resp.Data[0].EventObservedAt.Time, time.Second)

	seedScan(t, f.pool, f.orgID, f.assetID, f.loc("L2"), now)
	resp = getHistory(t, h, f.orgID, f.assetID, nil)
	require.Len(t, resp.Data, 2, "a move seen only in the tail is visible immediately")
	assert.Equal(t, f.loc("L2"), *resp.Data[0].LocationID)
	require.NotNil(t, resp.Data[1].DurationSeconds)
	assert.Equal(t, int(now.Sub(parkedSince).Seconds()), *resp.Data[1].DurationSeconds)
}
