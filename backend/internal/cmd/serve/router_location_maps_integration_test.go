//go:build integration

package serve

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// TestLocationMaps_ThroughTheRealRouter checks the wiring the handler tests
// cannot: session auth, the JSON ContentType gate on the base64 upload, the
// admin gate on writes, and reads open to any member with no capability grant.
func TestLocationMaps_ThroughTheRealRouter(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-key")
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	r := setupRealRouter(t, db.Store)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	member := func(email, role string) string {
		var uid int
		require.NoError(t, db.AdminPool.QueryRow(ctx, `
			INSERT INTO trakrf.users (email, name, password_hash) VALUES ($1, $1, 'hash') RETURNING id`, email).Scan(&uid))
		_, err := db.AdminPool.Exec(ctx, `INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, $3)`, orgID, uid, role)
		require.NoError(t, err)
		tok, err := jwt.Generate(uid, email, &orgID)
		require.NoError(t, err)
		return tok
	}
	admin := member("maps-admin@t.com", "admin")
	viewer := member("maps-viewer@t.com", "viewer")

	var site int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'SITE', 'Site') RETURNING id`, orgID).Scan(&site))

	do := func(tok, method, path, ct string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+tok)
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	img := map[string]string{"data_base64": base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nrouter"))}
	imagePath := fmt.Sprintf("/api/v1/location-maps/%d/image", site)

	rec := do(viewer, http.MethodPut, imagePath, "application/json", img)
	require.Equal(t, http.StatusForbidden, rec.Code, "viewer cannot upload: %s", rec.Body.String())

	rec = do(admin, http.MethodPut, imagePath, "image/png", img)
	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code, "upload is JSON only")

	rec = do(admin, http.MethodPut, imagePath, "application/json", img)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = do(viewer, http.MethodGet, imagePath, "", nil)
	require.Equal(t, http.StatusOK, rec.Code, "any member reads the image")
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))

	rec = do(viewer, http.MethodGet, fmt.Sprintf("/api/v1/location-maps/%d/presence", site), "", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = do(viewer, http.MethodDelete, fmt.Sprintf("/api/v1/location-maps/%d", site), "", nil)
	require.Equal(t, http.StatusForbidden, rec.Code)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/location-maps", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code, "no session, no maps")
}
