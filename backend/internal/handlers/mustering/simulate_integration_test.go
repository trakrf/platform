//go:build integration

package mustering_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	musteringhandler "github.com/trakrf/platform/backend/internal/handlers/mustering"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/muster"
	mustering "github.com/trakrf/platform/backend/internal/mustering"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// testUserID is the real users row id created per fixture; muster_events.started_by
// FKs to users, so the session claim must carry a user that actually exists.
var testUserID int

func withOrg(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: testUserID, Email: "tra978@t.com", CurrentOrgID: &orgID}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
}

// newMusterServer wires the real engine + broadcaster + store behind the handler
// so simulate/seed drive the genuine ingest pipeline (PersistReads + Evaluate).
// The evaluator IS the engine (single consumer); the Live Reads feed is nil.
func newMusterServer(t *testing.T) (chi.Router, *testutil.TestDB, int) {
	t.Helper()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// muster_events.started_by FKs to users; create a real user + membership and
	// thread its id into the session claims (reset per fixture; -p 1 serializes).
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ('tra978@t.com', 'TRA-978 Operator', 'hash') RETURNING id`).Scan(&testUserID))
	_, err := db.AdminPool.Exec(context.Background(), `
		INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'operator')`, orgID, testUserID)
	require.NoError(t, err)

	log := zerolog.Nop()
	bc := mustering.NewBroadcaster()
	eng := mustering.NewEngine(db.Store, bc, &log)
	// *mustering.Engine satisfies the handler's (unexported) readEvaluator via its
	// Evaluate method; pass it directly as the simulate fan-out target.
	h := musteringhandler.NewHandler(eng, bc, db.Store, eng, nil)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	h.RegisterRoutes(r)
	return r, db, orgID
}

func doJSON(t *testing.T, r http.Handler, orgID int, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req = withOrg(req, orgID)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// TestSeedThenSimulateDrivesTransition is the end-to-end proof: seed the demo
// data, activate a muster, then simulate everyone arriving at a muster point and
// assert the entries transitioned missing→at_muster through the REAL pipeline.
func TestSeedThenSimulateDrivesTransition(t *testing.T) {
	r, db, orgID := newMusterServer(t)

	// Seed (idempotent). Second call must be a no-op (all skipped).
	rr := doJSON(t, r, orgID, http.MethodPost, "/api/v1/mustering/seed", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	rr2 := doJSON(t, r, orgID, http.MethodPost, "/api/v1/mustering/seed", nil)
	require.Equal(t, http.StatusOK, rr2.Code)
	var seed2 struct {
		Data struct {
			PersonsCreated int `json:"persons_created"`
			Skipped        int `json:"skipped"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &seed2))
	require.Equal(t, 0, seed2.Data.PersonsCreated, "second seed must create nothing")
	require.Greater(t, seed2.Data.Skipped, 0)

	// Status: the seed's initial simulate round placed persons in zones.
	rr = doJSON(t, r, orgID, http.MethodGet, "/api/v1/mustering/status", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	var status mustering.SnapshotPayload
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &status))
	require.Greater(t, status.PersonsOnSite, 0, "seed should have placed persons on-site")

	// Activate a muster event (wide window so the just-seeded persons are in scope).
	rr = doJSON(t, r, orgID, http.MethodPost, "/api/v1/mustering/events", map[string]any{"window_minutes": 60})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var created struct {
		Data muster.Event `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))
	ev := created.Data
	require.Equal(t, "active", ev.Status)
	require.Greater(t, len(ev.Entries), 0)
	require.Equal(t, len(ev.Entries), ev.Counts.Missing)

	// Resolve muster point A's location, then simulate all persons arriving there.
	mp, err := db.Store.GetLocationByExternalKey(context.Background(), orgID, "MUSTER-MP-001")
	require.NoError(t, err)
	require.NotNil(t, mp)

	var sightings []map[string]int
	for _, en := range ev.Entries {
		sightings = append(sightings, map[string]int{"asset_id": en.AssetID, "location_id": mp.ID})
	}
	rr = doJSON(t, r, orgID, http.MethodPost, "/api/v1/mustering/simulate", map[string]any{"sightings": sightings})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	// Re-fetch: every entry should now be at_muster.
	rr = doJSON(t, r, orgID, http.MethodGet, "/api/v1/mustering/status", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &status))
	require.NotNil(t, status.Event)
	require.Equal(t, 0, status.Event.Counts.Missing, "all persons should be at muster")
	require.Equal(t, len(ev.Entries), status.Event.Counts.AtMuster)

	// All-clear computes a report.
	rr = doJSON(t, r, orgID, http.MethodPost, "/api/v1/mustering/events/"+strconv.Itoa(ev.ID)+"/all-clear", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var done struct {
		Data muster.Event `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &done))
	require.Equal(t, "completed", done.Data.Status)
	require.NotNil(t, done.Data.Report)
}

// TestSimulate_UnprocessableNoScanPoint returns 422 when a location has no scan
// point (here, a nonexistent location id).
func TestSimulate_UnprocessableNoScanPoint(t *testing.T) {
	r, _, orgID := newMusterServer(t)
	rr := doJSON(t, r, orgID, http.MethodPost, "/api/v1/mustering/simulate",
		map[string]any{"sightings": []map[string]int{{"asset_id": 1, "location_id": 999999999}}})
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code, rr.Body.String())
}
