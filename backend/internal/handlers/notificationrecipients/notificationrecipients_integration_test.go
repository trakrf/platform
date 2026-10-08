//go:build integration

package notificationrecipients_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/handlers/notificationrecipients"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func passThrough(next http.Handler) http.Handler { return next }

func withOrg(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra1275@t.com", CurrentOrgID: &orgID}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
}

func newRouter(t *testing.T) (*chi.Mux, *testutil.TestDB, int) {
	t.Helper()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	notificationrecipients.NewHandler(db.Store).RegisterRoutes(r, passThrough)
	return r, db, orgID
}

func do(t *testing.T, r http.Handler, orgID int, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	return rec
}

func TestNotificationRecipients_RequiresContact(t *testing.T) {
	r, _, orgID := newRouter(t)
	rec := do(t, r, orgID, http.MethodPost, "/api/v1/notification-recipients", map[string]any{"name": "No contact"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestNotificationRecipients_DuplicateContactReturnsConflict(t *testing.T) {
	r, _, orgID := newRouter(t)
	body := map[string]any{"name": "A", "email": "dup@acme.test"}
	require.Equal(t, http.StatusCreated, do(t, r, orgID, http.MethodPost, "/api/v1/notification-recipients", body).Code)
	rec := do(t, r, orgID, http.MethodPost, "/api/v1/notification-recipients", map[string]any{"name": "B", "email": "dup@acme.test"})
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestNotificationSubscriptions_ChannelDefaultsToEmail(t *testing.T) {
	r, db, orgID := newRouter(t)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-H-DEFAULT")

	rec := do(t, r, orgID, http.MethodPost, "/api/v1/notification-recipients", map[string]any{"name": "P", "email": "p@acme.test"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = do(t, r, orgID, http.MethodPost, "/api/v1/assets/"+strconv.Itoa(a.ID)+"/notification-subscriptions",
		map[string]any{"recipient_id": created.Data.ID})
	require.Equal(t, http.StatusCreated, rec.Code)
	var sub struct {
		Data struct {
			Channel string `json:"channel"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sub))
	require.Equal(t, "email", sub.Data.Channel)
}

func TestNotificationSubscriptions_SmsWithoutPhoneIsBadRequest(t *testing.T) {
	r, db, orgID := newRouter(t)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-H-SMS")

	rec := do(t, r, orgID, http.MethodPost, "/api/v1/notification-recipients", map[string]any{"name": "E", "email": "e@acme.test"})
	var created struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = do(t, r, orgID, http.MethodPost, "/api/v1/assets/"+strconv.Itoa(a.ID)+"/notification-subscriptions",
		map[string]any{"recipient_id": created.Data.ID, "channel": "sms"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestNotificationSubscriptions_ZeroSubscribersListsEmpty(t *testing.T) {
	r, db, orgID := newRouter(t)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-H-ZERO")

	rec := do(t, r, orgID, http.MethodGet, "/api/v1/assets/"+strconv.Itoa(a.ID)+"/notification-subscriptions", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Data []any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.NotNil(t, list.Data)
	require.Empty(t, list.Data)
}

func TestNotificationRecipients_CrossOrgReturnsNotFound(t *testing.T) {
	r, db, orgA := newRouter(t)
	var orgB int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Org B Recipients', 'test-org-b-recipients', true)
		 ON CONFLICT (identifier) DO UPDATE SET name = EXCLUDED.name RETURNING id`).Scan(&orgB))

	rec := do(t, r, orgA, http.MethodPost, "/api/v1/notification-recipients", map[string]any{"name": "A", "email": "a-only@acme.test"})
	var created struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = do(t, r, orgB, http.MethodGet, "/api/v1/notification-recipients/"+strconv.Itoa(created.Data.ID), nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func createRecipient(t *testing.T, r http.Handler, orgID int, body map[string]any) int {
	t.Helper()
	rec := do(t, r, orgID, http.MethodPost, "/api/v1/notification-recipients", body)
	require.Equal(t, http.StatusCreated, rec.Code)
	var created struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	return created.Data.ID
}

type subscriptionBody struct {
	Data struct {
		ID       int    `json:"id"`
		Channel  string `json:"channel"`
		IsActive bool   `json:"is_active"`
	} `json:"data"`
}

func TestNotificationSubscriptions_RepeatSubscribeReturnsExisting(t *testing.T) {
	r, db, orgID := newRouter(t)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-H-REPEAT")
	recipientID := createRecipient(t, r, orgID, map[string]any{"name": "R", "email": "repeat@acme.test"})
	path := "/api/v1/assets/" + strconv.Itoa(a.ID) + "/notification-subscriptions"

	rec := do(t, r, orgID, http.MethodPost, path, map[string]any{"recipient_id": recipientID})
	require.Equal(t, http.StatusCreated, rec.Code)
	var first subscriptionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))

	rec = do(t, r, orgID, http.MethodPost, path, map[string]any{"recipient_id": recipientID})
	require.Equal(t, http.StatusOK, rec.Code)
	var again subscriptionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &again))
	require.Equal(t, first.Data.ID, again.Data.ID)
}

func TestNotificationSubscriptions_PatchSwitchesOffAndDeleteIsNotAllowed(t *testing.T) {
	r, db, orgID := newRouter(t)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-H-PATCH")
	recipientID := createRecipient(t, r, orgID, map[string]any{"name": "P", "email": "patch@acme.test"})
	path := "/api/v1/assets/" + strconv.Itoa(a.ID) + "/notification-subscriptions"

	rec := do(t, r, orgID, http.MethodPost, path, map[string]any{"recipient_id": recipientID})
	require.Equal(t, http.StatusCreated, rec.Code)
	var sub subscriptionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sub))
	subPath := path + "/" + strconv.Itoa(sub.Data.ID)

	rec = do(t, r, orgID, http.MethodPatch, subPath, map[string]any{"is_active": false})
	require.Equal(t, http.StatusOK, rec.Code)
	var off subscriptionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &off))
	require.False(t, off.Data.IsActive)

	require.Equal(t, http.StatusMethodNotAllowed, do(t, r, orgID, http.MethodDelete, subPath, nil).Code)

	rec = do(t, r, orgID, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Data []struct {
			IsActive bool `json:"is_active"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Data, 1, "a switched-off subscription is still listed")
	require.False(t, list.Data[0].IsActive)
}

func TestNotificationSubscriptions_PatchSmsWithoutPhoneIsBadRequest(t *testing.T) {
	r, db, orgID := newRouter(t)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-H-PSMS")
	recipientID := createRecipient(t, r, orgID, map[string]any{"name": "E", "email": "psms@acme.test"})
	path := "/api/v1/assets/" + strconv.Itoa(a.ID) + "/notification-subscriptions"

	rec := do(t, r, orgID, http.MethodPost, path, map[string]any{"recipient_id": recipientID})
	var sub subscriptionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sub))

	rec = do(t, r, orgID, http.MethodPatch, path+"/"+strconv.Itoa(sub.Data.ID), map[string]any{"channel": "sms"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, http.StatusNotFound, do(t, r, orgID, http.MethodPatch, path+"/"+strconv.Itoa(sub.Data.ID+1_000_000), map[string]any{"is_active": false}).Code)
}
