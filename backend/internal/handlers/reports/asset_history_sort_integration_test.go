//go:build integration
// +build integration

// TRA-760 / BB53 F1: listAssetHistory must honor `?sort=event_observed_at`
// (ASC) and `?sort=-event_observed_at` (DESC). Default order (no param)
// remains DESC. Regression test pins all three so the handler can't
// silently revert to always-DESC.

package reports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestListAssetHistory_SortHonorsAscAndDesc(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC().Truncate(time.Second)
	yesterday := now.Add(-24 * time.Hour)

	assetID := seedAssetForReports(t, pool, orgID, "H-SORT-A", yesterday, nil)
	locID := seedLocationForReports(t, pool, orgID, "H-SORT-L", yesterday, nil)

	// Seed three scans in non-monotonic insertion order so any
	// insertion-order leakage would be visible.
	t0 := now.Add(-3 * time.Hour)
	t1 := now.Add(-2 * time.Hour)
	t2 := now.Add(-1 * time.Hour)
	seedScan(t, pool, orgID, assetID, locID, t1)
	seedScan(t, pool, orgID, assetID, locID, t0)
	seedScan(t, pool, orgID, assetID, locID, t2)

	handler := NewHandler(store)
	router := setupTemporalReportsRouter(handler)

	get := func(t *testing.T, qs string) []report.PublicAssetHistoryItem {
		t.Helper()
		url := fmt.Sprintf("/api/v1/assets/%d/history%s", assetID, qs)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req = withReportsOrg(req, orgID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var resp historyResp
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Data, 3)
		return resp.Data
	}

	t.Run("default is DESC", func(t *testing.T) {
		rows := get(t, "")
		assert.True(t, rows[0].EventObservedAt.After(rows[1].EventObservedAt.Time))
		assert.True(t, rows[1].EventObservedAt.After(rows[2].EventObservedAt.Time))
	})

	t.Run("sort=event_observed_at returns ASC", func(t *testing.T) {
		rows := get(t, "?sort=event_observed_at")
		assert.True(t, rows[0].EventObservedAt.Before(rows[1].EventObservedAt.Time),
			"expected ASC: first row must be older than second")
		assert.True(t, rows[1].EventObservedAt.Before(rows[2].EventObservedAt.Time),
			"expected ASC: second row must be older than third")
	})

	t.Run("sort=-event_observed_at returns DESC", func(t *testing.T) {
		rows := get(t, "?sort=-event_observed_at")
		assert.True(t, rows[0].EventObservedAt.After(rows[1].EventObservedAt.Time))
		assert.True(t, rows[1].EventObservedAt.After(rows[2].EventObservedAt.Time))
	})
}
