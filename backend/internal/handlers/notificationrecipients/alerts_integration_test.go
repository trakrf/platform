//go:build integration

package notificationrecipients_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationAlerts_EmptyListEnvelope(t *testing.T) {
	r, _, orgID := newRouter(t)
	rec := do(t, r, orgID, http.MethodGet, "/api/v1/notification-alerts?page=2&per_page=5", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"data":[],"pagination":{"page":2,"per_page":5,"total":0}}`, rec.Body.String())
}

func TestNotificationAlerts_RejectsBadQuery(t *testing.T) {
	r, _, orgID := newRouter(t)
	rec := do(t, r, orgID, http.MethodGet, "/api/v1/notification-alerts?type=sms&limit=5", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body struct {
		Error struct {
			Fields []struct {
				Field string `json:"field"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Error.Fields, 1, "unknown params are reported before values are checked: %s", rec.Body.String())
	require.Equal(t, "limit", body.Error.Fields[0].Field)
}
