//go:build integration
// +build integration

// TRA-865: GET /api/v1/assets/{asset_id}/history must not return 500 when the
// gap between two consecutive scans exceeds the int4 range. The duration
// projection computes EXTRACT(EPOCH FROM (next - current)) which overflows a
// 32-bit integer once the interval passes ~68 years (2^31 seconds). A single
// bad/sentinel timestamp in the seeded scan history is enough to crash the
// whole query for that asset, which is the convergent bb-2.1 F3 symptom.
//
// This pins the contract: a wide gap surfaces a 200 with a well-formed
// response, never a 500.

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

	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestListAssetHistory_WideGapDoesNotOverflow(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	// Fixed timestamps keep the assertions deterministic across runs.
	validFrom := time.Date(1899, 1, 1, 0, 0, 0, 0, time.UTC)

	assetID := seedAssetForReports(t, pool, orgID, "H-OVF-A", validFrom, nil)
	locID := seedLocationForReports(t, pool, orgID, "H-OVF-L", validFrom, nil)

	// Two scans ~126 years apart: the gap is well past the 2^31-second
	// (~68 year) int4 ceiling that EXTRACT(EPOCH ...)::INT hits. A single
	// sentinel/bad timestamp in real seeded history produces exactly this.
	ancient := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedScan(t, pool, orgID, assetID, locID, ancient)
	seedScan(t, pool, orgID, assetID, locID, recent)

	handler := NewHandler(store)
	router := setupTemporalReportsRouter(handler)

	url := fmt.Sprintf("/api/v1/assets/%d/history", assetID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "wide-gap history must not 500; body: %s", w.Body.String())

	var resp historyResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)

	// Exactly one row carries the wide-gap duration (the most-recent row has a
	// null duration via LEAD). That duration must exceed the int4 ceiling —
	// proving the BIGINT cast: an INT cast would have raised SQLSTATE 22003.
	const int32Max = 2147483647
	var withDuration int
	for _, row := range resp.Data {
		if row.DurationSeconds != nil {
			withDuration++
			assert.Greater(t, *row.DurationSeconds, int32Max,
				"the ~126-year gap must surface as a value beyond int4 range, which int4 could not hold")
		}
	}
	assert.Equal(t, 1, withDuration, "exactly one row should carry a duration")
}
