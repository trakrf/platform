package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/models/asset"
)

type AssetFactory struct {
	OrgID       int
	Identifier  string
	Name        string
	Type        string
	Description string
	ValidFrom   time.Time
	ValidTo     *time.Time
	IsActive    bool
}

func NewAssetFactory(orgID int) *AssetFactory {
	now := time.Now()
	validTo := now.Add(24 * time.Hour)
	return &AssetFactory{
		OrgID:       orgID,
		Identifier:  fmt.Sprintf("TEST-%d", time.Now().UnixNano()%1000000),
		Name:        "Test Asset",
		Type:        "device",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     &validTo,
		IsActive:    true,
	}
}

func (f *AssetFactory) WithIdentifier(id string) *AssetFactory {
	f.Identifier = id
	return f
}

func (f *AssetFactory) WithName(name string) *AssetFactory {
	f.Name = name
	return f
}

func (f *AssetFactory) WithType(t string) *AssetFactory {
	f.Type = t
	return f
}

func (f *AssetFactory) WithDescription(desc string) *AssetFactory {
	f.Description = desc
	return f
}

func (f *AssetFactory) WithValidTo(validTo *time.Time) *AssetFactory {
	f.ValidTo = validTo
	return f
}

func (f *AssetFactory) Build() asset.Asset {
	return asset.Asset{
		OrgID:       f.OrgID,
		Identifier:  f.Identifier,
		Name:        f.Name,
		Type:        f.Type,
		Description: f.Description,
		ValidFrom:   f.ValidFrom,
		ValidTo:     f.ValidTo,
		IsActive:    f.IsActive,
	}
}

func (f *AssetFactory) BuildBatch(count int) []asset.Asset {
	assets := make([]asset.Asset, count)
	for i := 0; i < count; i++ {
		assets[i] = asset.Asset{
			OrgID:       f.OrgID,
			Identifier:  fmt.Sprintf("%s-%d", f.Identifier, i),
			Name:        fmt.Sprintf("%s %d", f.Name, i),
			Type:        f.Type,
			Description: f.Description,
			ValidFrom:   f.ValidFrom,
			ValidTo:     f.ValidTo,
			IsActive:    f.IsActive,
		}
	}
	return assets
}

type CSVFactory struct {
	rows [][]string
}

func NewCSVFactory() *CSVFactory {
	return &CSVFactory{
		rows: [][]string{
			{"identifier", "name", "type", "description", "valid_from", "valid_to", "is_active"},
		},
	}
}

func (f *CSVFactory) AddRow(identifier, name, assetType, description, validFrom, validTo, isActive string) *CSVFactory {
	f.rows = append(f.rows, []string{identifier, name, assetType, description, validFrom, validTo, isActive})
	return f
}

func (f *CSVFactory) Build() [][]string {
	return f.rows
}

func CreateTestAsset(t *testing.T, pool *pgxpool.Pool, orgID int, identifier string) *asset.Asset {
	t.Helper()
	ctx := context.Background()

	now := time.Now()
	var id int
	err := pool.QueryRow(ctx, `
		INSERT INTO trakrf.assets (org_id, identifier, name, type, description, valid_from, valid_to, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, orgID, identifier, "Test Asset", "device", "Test description", now, now.Add(24*time.Hour), true).Scan(&id)

	if err != nil {
		t.Fatalf("Failed to create test asset: %v", err)
	}

	validTo := now.Add(24 * time.Hour)
	return &asset.Asset{
		ID:          id,
		OrgID:       orgID,
		Identifier:  identifier,
		Name:        "Test Asset",
		Type:        "device",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     &validTo,
		IsActive:    true,
	}
}
