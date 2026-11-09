//go:build integration

package outputdevices_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-1314: the manual actions are drives too, and leave the same record as a
// geofence fire. Commissioning proves a horn works with the test action, and
// that proof must not need SSH to the reader.

func TestDrives_TestActionRecordsOnThenOff(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	r, orgID := newTestServerOn(t, db, &fakeDriver{})
	id := createDevice(t, r, orgID, "http://192.168.50.66")

	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices/"+itoa(id)+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	drives, err := db.Store.ListOutputDrives(context.Background(), orgID, id, 10)
	require.NoError(t, err)
	require.Len(t, drives, 2)
	edges := map[string]string{}
	for _, d := range drives {
		edges[d.Edge] = d.Cause + "/" + d.Result
	}
	assert.Equal(t, map[string]string{"on": "test/ok", "off": "test/ok"}, edges)
}

func TestDrives_FailedTestActionRecordsTheError(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	r, orgID := newTestServerOn(t, db, &fakeDriver{failURL: "http://192.168.50.66"})
	id := createDevice(t, r, orgID, "http://192.168.50.66")

	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices/"+itoa(id)+"/test", nil)
	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())

	drives, err := db.Store.ListOutputDrives(context.Background(), orgID, id, 10)
	require.NoError(t, err)
	require.Len(t, drives, 1, "the ON failed, so no OFF was sent")
	assert.Equal(t, "on", drives[0].Edge)
	assert.Equal(t, "error", drives[0].Result)
	require.NotNil(t, drives[0].Error)
	assert.Equal(t, "device unreachable", *drives[0].Error)
}

func TestDrives_ResetRecordsOff(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	r, orgID := newTestServerOn(t, db, &fakeDriver{})
	id := createDevice(t, r, orgID, "http://192.168.50.66")

	rec := doReq(t, r, orgID, http.MethodPost, "/api/v1/output-devices/"+itoa(id)+"/reset", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	drives, err := db.Store.ListOutputDrives(context.Background(), orgID, id, 10)
	require.NoError(t, err)
	require.Len(t, drives, 1)
	assert.Equal(t, "off", drives[0].Edge)
	assert.Equal(t, "reset", drives[0].Cause)
	assert.Equal(t, "ok", drives[0].Result)
}

func TestDrives_DeleteRecordsItsOffEvenWhenItFails(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	r, orgID := newTestServerOn(t, db, &fakeDriver{failURL: "http://192.168.50.66"})
	id := createDevice(t, r, orgID, "http://192.168.50.66")

	rec := doReq(t, r, orgID, http.MethodDelete, "/api/v1/output-devices/"+itoa(id), nil)
	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())

	drives, err := db.Store.ListOutputDrives(context.Background(), orgID, id, 10)
	require.NoError(t, err)
	require.Len(t, drives, 1)
	assert.Equal(t, "off", drives[0].Edge)
	assert.Equal(t, "delete", drives[0].Cause)
	assert.Equal(t, "error", drives[0].Result)
}
