package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/asset"
)

func (s *Storage) CreateAsset(ctx context.Context, request asset.Asset) (*asset.Asset, error) {
	query := `
	insert into trakrf.assets
	(name, identifier, type, description, valid_from, valid_to, metadata, is_active, org_id)
	values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	returning id, org_id, identifier, name, type, description, valid_from, valid_to,
	          metadata, is_active, created_at, updated_at, deleted_at
	`
	var asset asset.Asset
	err := s.pool.QueryRow(ctx, query, request.Name, request.Identifier, request.Type,
		request.Description, request.ValidFrom, request.ValidTo, request.Metadata,
		request.IsActive, request.OrgID,
	).Scan(&asset.ID, &asset.OrgID, &asset.Identifier, &asset.Name, &asset.Type,
		&asset.Description, &asset.ValidFrom, &asset.ValidTo, &asset.Metadata,
		&asset.IsActive, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt,
	)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("asset with identifier %s already exists", request.Identifier)
		}
		return nil, fmt.Errorf("failed to create asset: %w", err)
	}

	return &asset, nil
}

func (s *Storage) UpdateAsset(ctx context.Context, id int, request asset.UpdateAssetRequest) (*asset.Asset, error) {
	updates := []string{}
	args := []any{id}
	argPos := 2
	fields, err := mapReqToFields(request)

	if err != nil {
		return nil, err
	}

	for key, value := range fields {
		if value != nil {
			updates = append(updates, fmt.Sprintf("%s = $%d", key, argPos))
			args = append(args, value)
			argPos++
		}
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		update trakrf.assets
		set %s, updated_at = now()
		where id = $1
		returning id, org_id, identifier, name, type, description, valid_from, valid_to,
		          metadata, is_active, created_at, updated_at, deleted_at
	`, strings.Join(updates, ", "))

	var asset asset.Asset
	err = s.pool.QueryRow(ctx, query, args...).Scan(&asset.ID, &asset.OrgID,
		&asset.Identifier, &asset.Name, &asset.Type, &asset.Description,
		&asset.ValidFrom, &asset.ValidTo, &asset.Metadata, &asset.IsActive,
		&asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update asset: %w", err)
	}

	return &asset, nil
}

func (s *Storage) GetAssetByID(ctx context.Context, id *int) (*asset.Asset, error) {
	query := `
	select id, org_id, identifier, name, type, description, valid_from, valid_to,
	       metadata, is_active, created_at, updated_at, deleted_at
	from trakrf.assets
	where id = $1 and deleted_at is null
	`
	var asset asset.Asset
	err := s.pool.QueryRow(ctx, query, id).Scan(&asset.ID, &asset.OrgID,
		&asset.Identifier, &asset.Name, &asset.Type, &asset.Description,
		&asset.ValidFrom, &asset.ValidTo, &asset.Metadata, &asset.IsActive,
		&asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get asset by id: %w", err)
	}
	return &asset, nil
}

func (s *Storage) ListAllAssets(ctx context.Context, limit int, offset int) ([]asset.Asset, error) {
	query := `
		select id, org_id, identifier, name, type, description, valid_from, valid_to,
		       metadata, is_active, created_at, updated_at, deleted_at
		from trakrf.assets
		where deleted_at is null
		order by created_at desc
		limit $1 offset $2
	`
	rows, err := s.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset
	for rows.Next() {
		var a asset.Asset
		err := rows.Scan(&a.ID, &a.OrgID, &a.Identifier, &a.Name, &a.Type,
			&a.Description, &a.ValidFrom, &a.ValidTo, &a.Metadata, &a.IsActive,
			&a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset: %w", err)
		}
		assets = append(assets, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating assets: %w", err)
	}

	return assets, nil
}

func (s *Storage) DeleteAsset(ctx context.Context, id *int) (bool, error) {
	query := `update trakrf.assets set deleted_at = now() where id = $1 and deleted_at is null`
	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("could not delete asset: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

// BatchCreateAssets creates multiple assets in a single transaction.
// If any asset fails to insert, the entire transaction is rolled back and errors are returned.
// Returns the number of successful inserts and a slice of errors (with row numbers).
func (s *Storage) BatchCreateAssets(ctx context.Context, assets []asset.Asset) (int, []error) {
	if len(assets) == 0 {
		return 0, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, []error{fmt.Errorf("failed to begin transaction: %w", err)}
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO trakrf.assets
		(name, identifier, type, description, valid_from, valid_to, metadata, is_active, org_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	var errors []error
	successCount := 0

	for i, a := range assets {
		_, err := tx.Exec(ctx, query,
			a.Name, a.Identifier, a.Type, a.Description,
			a.ValidFrom, a.ValidTo, a.Metadata, a.IsActive, a.OrgID,
		)

		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
				errors = append(errors, fmt.Errorf("row %d: asset with identifier %s already exists", i, a.Identifier))
			} else {
				errors = append(errors, fmt.Errorf("row %d: %w", i, err))
			}
		} else {
			successCount++
		}
	}

	if len(errors) > 0 {
		tx.Rollback(ctx)
		return 0, errors
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, []error{fmt.Errorf("failed to commit transaction: %w", err)}
	}

	return successCount, nil
}

func mapReqToFields(req asset.UpdateAssetRequest) (map[string]any, error) {
	fields := make(map[string]any)

	if req.OrgID != nil {
		fields["org_id"] = *req.OrgID
	}
	if req.Identifier != nil {
		fields["identifier"] = *req.Identifier
	}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Type != nil {
		fields["type"] = *req.Type
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.ValidFrom != nil {
		fields["valid_from"] = *req.ValidFrom
	}
	if req.ValidTo != nil {
		fields["valid_to"] = *req.ValidTo
	}
	if req.Metadata != nil {
		fields["metadata"] = *req.Metadata
	}
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}

	return fields, nil
}
