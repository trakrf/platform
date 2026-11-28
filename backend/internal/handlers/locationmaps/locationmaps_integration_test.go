//go:build integration

package locationmaps_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/locationmaps"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/locationmap"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

var pngImage = []byte("\x89PNG\r\n\x1a\nintegration-test-png")

func passThrough(next http.Handler) http.Handler { return next }

type fixture struct {
	t     *testing.T
	r     chi.Router
	db    *testutil.TestDB
	orgID int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	locationmaps.NewHandler(db.Store).RegisterRoutes(r, passThrough, passThrough)
	return &fixture{t: t, r: r, db: db, orgID: orgID}
}

func (f *fixture) do(method, path string, body any, hdr map[string]string) *httptest.ResponseRecorder {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(f.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	orgID := f.orgID
	claims := &jwt.Claims{UserID: 1, Email: "maps@t.com", CurrentOrgID: &orgID}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
	rr := httptest.NewRecorder()
	f.r.ServeHTTP(rr, req)
	return rr
}

func (f *fixture) location(key string, parent *int) int {
	f.t.Helper()
	var id int
	require.NoError(f.t, f.db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, parent_location_id)
		VALUES ($1, $2, $2, $3) RETURNING id`, f.orgID, key, parent).Scan(&id))
	return id
}

func (f *fixture) uploadPNG(locID int) {
	f.t.Helper()
	rr := f.do(http.MethodPut, fmt.Sprintf("/api/v1/location-maps/%d/image", locID),
		map[string]string{"data_base64": base64.StdEncoding.EncodeToString(pngImage)}, nil)
	require.Equal(f.t, http.StatusOK, rr.Code, rr.Body.String())
}

func TestLocationMaps_ImageUploadAndServe(t *testing.T) {
	f := newFixture(t)
	site := f.location("SITE", nil)

	rr := f.do(http.MethodGet, fmt.Sprintf("/api/v1/location-maps/%d", site), nil, nil)
	require.Equal(t, http.StatusNotFound, rr.Code)

	f.uploadPNG(site)

	rr = f.do(http.MethodGet, fmt.Sprintf("/api/v1/location-maps/%d", site), nil, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var got struct{ Data locationmap.Map }
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "image/png", got.Data.ContentType)
	require.Equal(t, len(pngImage), got.Data.ByteSize)
	require.Empty(t, got.Data.Pins)

	rr = f.do(http.MethodGet, fmt.Sprintf("/api/v1/location-maps/%d/image", site), nil, nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, pngImage, rr.Body.Bytes())
	require.Equal(t, "image/png", rr.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	etag := rr.Header().Get("ETag")
	require.Equal(t, `"`+got.Data.SHA256+`"`, etag)

	rr = f.do(http.MethodGet, fmt.Sprintf("/api/v1/location-maps/%d/image", site), nil, map[string]string{"If-None-Match": etag})
	require.Equal(t, http.StatusNotModified, rr.Code)
	require.Empty(t, rr.Body.Bytes())

	rr = f.do(http.MethodGet, "/api/v1/location-maps", nil, nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `"location_name":"SITE"`)

	rr = f.do(http.MethodDelete, fmt.Sprintf("/api/v1/location-maps/%d", site), nil, nil)
	require.Equal(t, http.StatusNoContent, rr.Code)
	rr = f.do(http.MethodGet, fmt.Sprintf("/api/v1/location-maps/%d/image", site), nil, nil)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestLocationMaps_ImageRejections(t *testing.T) {
	f := newFixture(t)
	site := f.location("SITE", nil)
	put := func(data string) *httptest.ResponseRecorder {
		return f.do(http.MethodPut, fmt.Sprintf("/api/v1/location-maps/%d/image", site),
			map[string]string{"data_base64": data}, nil)
	}

	require.Equal(t, http.StatusBadRequest, put("not base64!!").Code)
	require.Equal(t, http.StatusBadRequest, put(base64.StdEncoding.EncodeToString([]byte("plain text"))).Code)
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
	require.Equal(t, http.StatusBadRequest, put(base64.StdEncoding.EncodeToString([]byte(svg))).Code)

	big := append(append([]byte{}, pngImage...), make([]byte, locationmap.MaxImageBytes)...)
	rr := put(base64.StdEncoding.EncodeToString(big))
	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code, rr.Body.String())

	rr = f.do(http.MethodPut, "/api/v1/location-maps/999999999/image",
		map[string]string{"data_base64": base64.StdEncoding.EncodeToString(pngImage)}, nil)
	require.Equal(t, http.StatusNotFound, rr.Code)

	var n int
	require.NoError(t, f.db.AdminPool.QueryRow(context.Background(), `SELECT count(*) FROM trakrf.location_maps`).Scan(&n))
	require.Zero(t, n, "nothing stored by a rejected upload")
}

func TestLocationMaps_Pins(t *testing.T) {
	f := newFixture(t)
	site := f.location("SITE", nil)
	room := f.location("ROOM-A", &site)
	other := f.location("ELSEWHERE", nil)
	pinsPath := fmt.Sprintf("/api/v1/location-maps/%d/pins", site)
	body := func(pins ...locationmap.Pin) map[string]any { return map[string]any{"pins": pins} }

	rr := f.do(http.MethodPut, pinsPath, body(locationmap.Pin{LocationID: room, XPct: 10, YPct: 10}), nil)
	require.Equal(t, http.StatusNotFound, rr.Code, "no map yet")

	f.uploadPNG(site)

	rr = f.do(http.MethodPut, pinsPath, body(locationmap.Pin{LocationID: room, XPct: 101, YPct: 10}), nil)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	rr = f.do(http.MethodPut, pinsPath, body(locationmap.Pin{LocationID: other, XPct: 10, YPct: 10}), nil)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code, "not below the map location")

	rr = f.do(http.MethodPut, pinsPath, body(locationmap.Pin{LocationID: room, XPct: 25.5, YPct: 75}), nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var got struct{ Data locationmap.Map }
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, []locationmap.Pin{{LocationID: room, XPct: 25.5, YPct: 75}}, got.Data.Pins)

	rr = f.do(http.MethodPut, pinsPath, body(), nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Empty(t, got.Data.Pins, "empty list clears the pins")
}

func TestLocationMaps_Presence(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	site := f.location("SITE", nil)
	roomA := f.location("ROOM-A", &site)
	cupboard := f.location("CUPBOARD", &roomA)
	roomB := f.location("ROOM-B", &site)
	hall := f.location("HALL", &site)
	f.uploadPNG(site)
	rr := f.do(http.MethodPut, fmt.Sprintf("/api/v1/location-maps/%d/pins", site), map[string]any{"pins": []locationmap.Pin{
		{LocationID: roomA, XPct: 20, YPct: 20}, {LocationID: roomB, XPct: 80, YPct: 20},
	}}, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	badge := testutil.CreateTestAsset(t, f.db.AdminPool, f.orgID, "BADGE-1")
	_, err := f.db.AdminPool.Exec(ctx, `UPDATE trakrf.assets SET metadata = '{"person": "true"}' WHERE id = $1`, badge.ID)
	require.NoError(t, err)
	pallet := testutil.CreateTestAsset(t, f.db.AdminPool, f.orgID, "PALLET-1")
	stale := testutil.CreateTestAsset(t, f.db.AdminPool, f.orgID, "STALE-1")
	wanderer := testutil.CreateTestAsset(t, f.db.AdminPool, f.orgID, "HALL-1")

	scan := func(assetID, locID int, at time.Time) {
		_, err := f.db.AdminPool.Exec(ctx, `
			INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id) VALUES ($1, $2, $3, $4)`,
			at, f.orgID, assetID, locID)
		require.NoError(t, err)
	}
	now := time.Now()
	scan(badge.ID, roomB, now.Add(-30*time.Minute)) // moved…
	scan(badge.ID, roomA, now.Add(-time.Minute))    // …to Room A
	scan(pallet.ID, cupboard, now.Add(-2*time.Minute))
	scan(stale.ID, roomB, now.Add(-3*time.Hour))
	scan(wanderer.ID, hall, now.Add(-time.Minute))
	testutil.RefreshAssetScanLatest(t, f.db.AdminPool)

	get := func(q string) locationmap.Presence {
		rr := f.do(http.MethodGet, fmt.Sprintf("/api/v1/location-maps/%d/presence%s", site, q), nil, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var out struct{ Data locationmap.Presence }
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
		return out.Data
	}

	p := get("")
	require.Len(t, p.Zones, 2)
	zone := map[int]locationmap.ZoneOccupancy{}
	for _, z := range p.Zones {
		zone[z.LocationID] = z
	}
	ids := func(occ []locationmap.Occupant) []int {
		out := []int{}
		for _, o := range occ {
			out = append(out, o.AssetID)
		}
		return out
	}
	require.ElementsMatch(t, []int{badge.ID, pallet.ID}, ids(zone[roomA].Occupants), "badge's latest zone + cupboard rolled up")
	require.Empty(t, zone[roomB].Occupants, "badge moved out; stale asset excluded")
	require.Equal(t, []int{wanderer.ID}, ids(p.Unplaced))
	for _, o := range zone[roomA].Occupants {
		require.Equal(t, o.AssetID == badge.ID, o.IsPerson)
		if o.AssetID == pallet.ID {
			require.Equal(t, cupboard, o.LocationID)
			require.Equal(t, "CUPBOARD", o.LocationName)
		}
	}

	all := get("?seen_within=0")
	for _, z := range all.Zones {
		if z.LocationID == roomB {
			require.Equal(t, []int{stale.ID}, ids(z.Occupants))
		}
	}

	rr = f.do(http.MethodGet, fmt.Sprintf("/api/v1/location-maps/%d/presence?seen_within=-5", site), nil, nil)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}
