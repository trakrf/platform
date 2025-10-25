package storage

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/asset"
)

func TestCreateAsset(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	request := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    []byte(`{"key":"value"}`),
		IsActive:    true,
		AccountID:   1,
	}

	rows := pgxmock.NewRows([]string{
		"id", "account_id", "identifier", "name", "type", "description",
		"valid_from", "valid_to", "metadata", "is_active",
		"created_at", "updated_at", "deleted_at",
	}).AddRow(
		1, request.AccountID, request.Identifier, request.Name,
		request.Type, request.Description, request.ValidFrom, request.ValidTo,
		request.Metadata, request.IsActive, now, now, nil,
	)

	mock.ExpectQuery(`insert into trakrf.assets`).
		WithArgs(
			request.Name, request.Identifier, request.Type,
			request.Description, request.ValidFrom, request.ValidTo,
			request.Metadata, request.IsActive, request.AccountID,
		).
		WillReturnRows(rows)

	result, err := storage.CreateAsset(context.Background(), request)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, request.Name, result.Name)
	assert.Equal(t, request.Identifier, result.Identifier)

	assert.NoError(t, mock.ExpectationsWereMet())
}
