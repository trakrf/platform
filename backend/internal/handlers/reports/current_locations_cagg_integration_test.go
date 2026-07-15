//go:build integration
// +build integration

package reports

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-1022: the report resolves latest-scan-per-asset from the asset_scan_latest
// continuous aggregate. Two scans for one asset that fall in different 1-minute
// time_buckets with different locations must collapse (across buckets) to the
// most recent location — proving the outer last()/max() over the CAGG, not a
// within-bucket accident.
func TestListCurrentLocations_CAGG_CollapsesAcrossBuckets(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	asset := seedAssetForReports(t, pool, orgID, "CAGG-A1", dayAgo, nil)
	oldLoc := seedLocationForReports(t, pool, orgID, "CAGG-L-OLD", dayAgo, nil)
	newLoc := seedLocationForReports(t, pool, orgID, "CAGG-L-NEW", dayAgo, nil)

	// Two scans in different minute buckets; the newer one wins.
	seedScan(t, pool, orgID, asset, oldLoc, now.Add(-10*time.Minute))
	seedScan(t, pool, orgID, asset, newLoc, now.Add(-2*time.Minute))

	// Materialized-only CAGG: force a refresh so the just-seeded rows are visible.
	testutil.RefreshAssetScanLatest(t, pool)

	router := setupTemporalReportsRouter(NewHandler(store))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/asset-locations", nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp currLocResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1, "exactly one asset expected")
	require.Equal(t, 1, resp.TotalCount, "count must also collapse to one")

	item := resp.Data[0]
	assert.Equal(t, "CAGG-A1", item.AssetExternalKey)
	require.NotNil(t, item.LocationExternalKey)
	assert.Equal(t, "CAGG-L-NEW", *item.LocationExternalKey, "latest scan's location must win across buckets")
}
