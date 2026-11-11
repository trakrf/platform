//go:build integration

package notificationrecipients_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/testutil"
)

const subscribedAssetCountKey = "subscribed_asset_count"

func TestNotificationRecipients_ReturnSubscribedAssetCount(t *testing.T) {
	r, db, orgID := newRouter(t)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-H-COUNT")
	base := "/api/v1/notification-recipients"

	// Writes keep their existing shape.
	rec := do(t, r, orgID, http.MethodPost, base, map[string]any{"name": "C", "email": "count@acme.test"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotContains(t, created.Data, subscribedAssetCountKey)
	var id int
	require.NoError(t, json.Unmarshal(created.Data["id"], &id))
	path := base + "/" + strconv.Itoa(id)

	rec = do(t, r, orgID, http.MethodPatch, path, map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, rec.Code)
	var patched struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &patched))
	require.NotContains(t, patched.Data, subscribedAssetCountKey)

	// Zero is sent explicitly, so the UI never has to guess at a missing field.
	requireCount := func(want string) {
		t.Helper()
		rec := do(t, r, orgID, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var single struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &single))
		require.JSONEq(t, want, string(single.Data[subscribedAssetCountKey]))

		rec = do(t, r, orgID, http.MethodGet, base, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var list struct {
			Data []map[string]json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list.Data, 1)
		require.JSONEq(t, want, string(list.Data[0][subscribedAssetCountKey]))
	}
	requireCount("0")

	subs := "/api/v1/assets/" + strconv.Itoa(a.ID) + "/notification-subscriptions"
	require.Equal(t, http.StatusCreated, do(t, r, orgID, http.MethodPost, subs, map[string]any{"recipient_id": id}).Code)
	requireCount("1")
}
