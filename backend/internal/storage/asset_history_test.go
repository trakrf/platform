package storage

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/report"
)

// TRA-865 (real root cause): GET /api/v1/assets/{id}/history 500'd in every
// deployed environment because ListAssetHistory / CountAssetHistory issued
// their queries on the raw pool instead of through WithOrgTx. The asset-history
// query LEFT JOINs trakrf.locations, which has RLS policy
//
//	org_id = current_setting('app.current_org_id')::bigint
//
// Without WithOrgTx the org GUC is never set on the connection, so the policy
// casts an empty/unset setting to bigint and the scan aborts
// (SQLSTATE 22P02 / 42704). The integration suite missed this because it
// connects as a superuser, which bypasses RLS. These mock tests pin the
// contract at the layer the bug lives in: the query MUST run inside the
// org-scoped transaction (SET LOCAL app.current_org_id) like every other
// RLS-touching storage call.

func TestListAssetHistory_RunsInOrgContext(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	const assetID = 201939693350237
	const orgID = 781048918750452
	filter := report.AssetHistoryFilter{Limit: 50, Offset: 0}

	now := time.Now()
	dur := 314829
	locID := 1980148728433683
	locName := "Main Warehouse"
	locKey := "WHS-01"
	rows := pgxmock.NewRows([]string{
		"timestamp", "location_id", "location_name", "location_external_key", "duration_seconds",
	}).AddRow(now, &locID, &locName, &locKey, &dur)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 781048918750452`).
		WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`LEAD\(s.timestamp\)`).
		WithArgs(assetID, orgID, filter.From, filter.To, filter.Limit, filter.Offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	items, err := storage.ListAssetHistory(context.Background(), assetID, orgID, filter)

	assert.NoError(t, err)
	require.Len(t, items, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountAssetHistory_RunsInOrgContext(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	const assetID = 201939693350237
	const orgID = 781048918750452
	filter := report.AssetHistoryFilter{}

	rows := pgxmock.NewRows([]string{"count"}).AddRow(25)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 781048918750452`).
		WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs(assetID, orgID, filter.From, filter.To).
		WillReturnRows(rows)
	mock.ExpectCommit()

	count, err := storage.CountAssetHistory(context.Background(), assetID, orgID, filter)

	assert.NoError(t, err)
	assert.Equal(t, 25, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}
