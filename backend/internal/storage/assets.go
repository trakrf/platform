package storage

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

func (s *Storage) CreateAsset(ctx context.Context, request asset.Asset) (*asset.Asset, error) {
	// Auto-generate external_key if empty
	if strings.TrimSpace(request.ExternalKey) == "" {
		seq, err := s.GetNextAssetSequence(ctx, request.OrgID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate external_key: %w", err)
		}
		request.ExternalKey = GenerateAssetExternalKey(seq)
	}

	query := `
	insert into trakrf.assets
	(name, external_key, description, current_location_id, valid_from, valid_to, metadata, is_active, org_id)
	values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	returning id, org_id, external_key, name, description, current_location_id, valid_from, valid_to,
	          metadata, is_active, created_at, updated_at, deleted_at
	`
	var asset asset.Asset
	err := s.WithOrgTx(ctx, request.OrgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, request.Name, request.ExternalKey,
			request.Description, request.LocationID, request.ValidFrom, request.ValidTo, request.Metadata,
			request.IsActive, request.OrgID,
		).Scan(&asset.ID, &asset.OrgID, &asset.ExternalKey, &asset.Name,
			&asset.Description, &asset.LocationID, &asset.ValidFrom, &asset.ValidTo, &asset.Metadata,
			&asset.IsActive, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt,
		)
	})

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("asset with external_key %s already exists", request.ExternalKey)
		}
		if strings.Contains(err.Error(), "current_location_id_fkey") {
			return nil, fmt.Errorf("invalid location_id: location does not exist")
		}
		return nil, fmt.Errorf("failed to create asset: %w", err)
	}

	return &asset, nil
}

// GetNextAssetSequence derives the next sequence number for auto-generated asset external_keys.
// It queries the max sequence from existing ASSET-XXXX external_keys for the org.
// Returns 1 if no ASSET-XXXX external_keys exist.
func (s *Storage) GetNextAssetSequence(ctx context.Context, orgID int) (int, error) {
	var maxSeq sql.NullInt64
	query := `
		SELECT MAX(CAST(SUBSTRING(external_key FROM 'ASSET-([0-9]+)') AS INT))
		FROM trakrf.assets
		WHERE org_id = $1
		  AND external_key ~ '^ASSET-[0-9]+$'
		  AND deleted_at IS NULL
	`
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID).Scan(&maxSeq)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get max sequence: %w", err)
	}
	if !maxSeq.Valid {
		return 1, nil // Start at 1 if no existing ASSET-XXXX
	}
	return int(maxSeq.Int64) + 1, nil
}

// GenerateAssetExternalKey creates an external_key in format ASSET-XXXX.
// Zero-pads to 4 digits minimum, grows naturally beyond 9999.
func GenerateAssetExternalKey(seq int) string {
	return fmt.Sprintf("ASSET-%04d", seq)
}

func (s *Storage) UpdateAsset(ctx context.Context, orgID, id int, request asset.UpdateAssetRequest) (*asset.AssetWithLocation, error) {
	updates := []string{}
	args := []any{id, orgID}
	argPos := 3
	fields, err := mapReqToFields(request)

	if err != nil {
		return nil, err
	}

	// Nil entries (only from ClearValidTo) pass through as SQL NULL.
	for key, value := range fields {
		updates = append(updates, fmt.Sprintf("%s = $%d", key, argPos))
		args = append(args, value)
		argPos++
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		update trakrf.assets
		set %s, updated_at = now()
		where id = $1 and org_id = $2 and deleted_at is null
		returning id
	`, strings.Join(updates, ", "))

	var updatedID int
	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&updatedID)
	})

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			externalKey := "unknown"
			if request.ExternalKey != nil {
				externalKey = *request.ExternalKey
			}
			return nil, fmt.Errorf("asset with external_key %s already exists", externalKey)
		}
		if strings.Contains(err.Error(), "current_location_id_fkey") {
			return nil, fmt.Errorf("invalid location_id: location does not exist")
		}
		return nil, fmt.Errorf("failed to update asset: %w", err)
	}

	return s.getAssetWithLocationByID(ctx, orgID, updatedID)
}

func (s *Storage) GetAssetByID(ctx context.Context, orgID int, id *int) (*asset.Asset, error) {
	query := `
	select id, org_id, external_key, name, description, current_location_id, valid_from, valid_to,
	       metadata, is_active, created_at, updated_at, deleted_at
	from trakrf.assets
	where id = $1 and org_id = $2 and deleted_at is null
	`
	var asset asset.Asset
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, orgID).Scan(&asset.ID, &asset.OrgID,
			&asset.ExternalKey, &asset.Name, &asset.Description,
			&asset.LocationID, &asset.ValidFrom, &asset.ValidTo, &asset.Metadata, &asset.IsActive,
			&asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt,
		)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get asset by id: %w", err)
	}
	return &asset, nil
}

// GetAssetsByIDs fetches multiple assets by their IDs (batch fetch), scoped
// to the caller's organization. The org_id fence is required because
// tags.asset_id is a plain FK that does not enforce same-org — see
// TRA-431 for the cross-tenant leak this prevents.
func (s *Storage) GetAssetsByIDs(ctx context.Context, orgID int, ids []int) ([]*asset.Asset, error) {
	if len(ids) == 0 {
		return []*asset.Asset{}, nil
	}

	query := `
	SELECT id, org_id, external_key, name, description, current_location_id, valid_from, valid_to,
	       metadata, is_active, created_at, updated_at, deleted_at
	FROM trakrf.assets
	WHERE org_id = $1 AND id = ANY($2) AND deleted_at IS NULL
	`

	assets := []*asset.Asset{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, ids)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a asset.Asset
			if err := rows.Scan(&a.ID, &a.OrgID, &a.ExternalKey, &a.Name,
				&a.Description, &a.LocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata, &a.IsActive,
				&a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
			); err != nil {
				return fmt.Errorf("failed to scan asset: %w", err)
			}
			assets = append(assets, &a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to batch fetch assets: %w", err)
	}

	return assets, nil
}

func (s *Storage) ListAllAssets(ctx context.Context, orgID int, limit int, offset int) ([]asset.Asset, error) {
	query := `
		select id, org_id, external_key, name, description, current_location_id, valid_from, valid_to,
		       metadata, is_active, created_at, updated_at, deleted_at
		from trakrf.assets
		where org_id = $1 and deleted_at is null
		order by created_at desc
		limit $2 offset $3
	`
	assets := []asset.Asset{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a asset.Asset
			if err := rows.Scan(&a.ID, &a.OrgID, &a.ExternalKey, &a.Name,
				&a.Description, &a.LocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata, &a.IsActive,
				&a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
			); err != nil {
				return fmt.Errorf("failed to scan asset: %w", err)
			}
			assets = append(assets, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list assets: %w", err)
	}

	return assets, nil
}

// CountAllAssets returns the total count of non-deleted assets for a specific org
func (s *Storage) CountAllAssets(ctx context.Context, orgID int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM trakrf.assets
		WHERE org_id = $1 AND deleted_at IS NULL
	`

	var count int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count assets: %w", err)
	}

	return count, nil
}

func (s *Storage) DeleteAsset(ctx context.Context, orgID, id int) (bool, error) {
	query := `update trakrf.assets set deleted_at = now() where id = $1 and org_id = $2 and deleted_at is null`
	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, query, id, orgID)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("could not delete asset: %w", err)
	}
	return rowsAffected > 0, nil
}

// BatchCreateAssets atomically inserts multiple assets in a single transaction.
// This is an all-or-nothing operation: if ANY asset fails to insert,
// the entire transaction is rolled back and ZERO assets are saved.
// Returns the number of successful inserts and a slice of errors (with row numbers).
func (s *Storage) BatchCreateAssets(ctx context.Context, assets []asset.Asset) (int, []error) {
	if len(assets) == 0 {
		return 0, nil
	}

	// Defensive: all assets in batch must share the same OrgID. The prior
	// implementation assumed this silently; make it enforceable so that a
	// WithOrgTx-wrapped batch cannot accidentally mix tenants.
	orgID := assets[0].OrgID
	for _, a := range assets {
		if a.OrgID != orgID {
			return 0, []error{fmt.Errorf("BatchCreateAssets: heterogeneous OrgIDs in batch (expected %d, got %d)", orgID, a.OrgID)}
		}
	}

	// Auto-generate external_keys for assets with empty external_keys.
	seq, err := s.GetNextAssetSequence(ctx, orgID)
	if err != nil {
		return 0, []error{fmt.Errorf("failed to get sequence for auto-generation: %w", err)}
	}

	for i := range assets {
		if strings.TrimSpace(assets[i].ExternalKey) == "" {
			assets[i].ExternalKey = GenerateAssetExternalKey(seq)
			seq++
		}
	}

	// TRA-475: BatchCreateAssets is documented and tested as all-or-nothing
	// insert — any duplicate external_key rolls the whole transaction back.
	// The partial UNIQUE(org_id, external_key) WHERE deleted_at IS NULL fires
	// on duplicate, the substring sniff converts it to a row-numbered
	// error, and WithOrgTx rolls the transaction back. Upsert-on-bulk-import
	// is intentionally out of scope (see TRA-475 spec).
	query := `
		INSERT INTO trakrf.assets
		(name, external_key, description, current_location_id, valid_from, valid_to, metadata, is_active, org_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		for i, a := range assets {
			if _, err := tx.Exec(ctx, query,
				a.Name, a.ExternalKey, a.Description, a.LocationID,
				a.ValidFrom, a.ValidTo, a.Metadata, a.IsActive, a.OrgID,
			); err != nil {
				if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
					return fmt.Errorf("row %d: asset with external_key %s already exists", i, a.ExternalKey)
				}
				return fmt.Errorf("row %d: %w", i, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, []error{err}
	}

	return len(assets), nil
}

// CheckDuplicateExternalKeys checks if any of the provided external_keys already exist in the database.
// Returns a map of external_key -> bool where true means the external_key exists.
func (s *Storage) CheckDuplicateExternalKeys(ctx context.Context, orgID int, externalKeys []string) (map[string]bool, error) {
	if len(externalKeys) == 0 {
		return make(map[string]bool), nil
	}

	query := `
		SELECT external_key
		FROM trakrf.assets
		WHERE org_id = $1 AND external_key = ANY($2) AND deleted_at IS NULL
	`

	existing := make(map[string]bool)
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, externalKeys)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var ek string
			if err := rows.Scan(&ek); err != nil {
				return fmt.Errorf("failed to scan external_key: %w", err)
			}
			existing[ek] = true
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to check duplicate external_keys: %w", err)
	}

	return existing, nil
}

func mapReqToFields(req asset.UpdateAssetRequest) (map[string]any, error) {
	fields := make(map[string]any)

	// Note: OrgID is intentionally NOT writable via UpdateAssetRequest.
	// The owning org is fixed at creation; ownership transfers must use
	// dedicated tooling, never a public PUT body.
	if req.ExternalKey != nil {
		fields["external_key"] = *req.ExternalKey
	}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	// description: explicit null on PUT clears to empty string. The DB column
	// is nullable but ToPublicAssetView projects "" → null on read, so storing
	// "" preserves the null-on-read contract without forcing every scan call
	// to handle SQL NULL into a Go string. (TRA-614 / BB19 §S1.)
	if req.ClearDescription {
		fields["description"] = ""
	} else if req.Description != nil {
		fields["description"] = *req.Description
	}
	// current_location_id is *int → SQL NULL is the natural representation.
	if req.ClearLocationID {
		fields["current_location_id"] = nil
	} else if req.LocationID != nil {
		fields["current_location_id"] = *req.LocationID
	}
	if req.ValidFrom != nil && !req.ValidFrom.IsZero() {
		fields["valid_from"] = req.ValidFrom.ToTime()
	}
	if req.ClearValidTo {
		fields["valid_to"] = nil
	} else if req.ValidTo != nil && !req.ValidTo.IsZero() {
		fields["valid_to"] = req.ValidTo.ToTime()
	}
	if req.Metadata != nil {
		fields["metadata"] = *req.Metadata
	}
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}

	return fields, nil
}

func (s *Storage) CreateAssetWithTags(ctx context.Context, request asset.CreateAssetWithTagsRequest) (*asset.AssetWithLocation, error) {
	// Auto-generate external_key if empty
	if strings.TrimSpace(request.ExternalKey) == "" {
		seq, err := s.GetNextAssetSequence(ctx, request.OrgID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate external_key: %w", err)
		}
		request.ExternalKey = GenerateAssetExternalKey(seq)
	}

	tagsJSON, err := tagsToJSON(request.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize tags: %w", err)
	}

	// Handler normally applies defaults; storage re-applies as a safety net
	// for direct (non-handler) callers such as bulkimport.
	var validFrom time.Time
	if request.ValidFrom != nil && !request.ValidFrom.IsZero() {
		validFrom = request.ValidFrom.ToTime()
	} else {
		validFrom = time.Now().UTC()
	}
	var validTo *time.Time
	if request.ValidTo != nil && !request.ValidTo.IsZero() {
		t := request.ValidTo.ToTime()
		validTo = &t
	}
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}

	query := `SELECT * FROM trakrf.create_asset_with_tags($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	var assetID int
	var tagIDs []int

	err = s.WithOrgTx(ctx, request.OrgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			request.OrgID,
			request.ExternalKey,
			request.Name,
			request.Description,
			request.LocationID,
			validFrom,
			validTo,
			isActive,
			request.Metadata,
			tagsJSON,
		).Scan(&assetID, &tagIDs)
	})

	if err != nil {
		return nil, parseAssetWithTagsError(err, request.ExternalKey)
	}

	return s.getAssetWithLocationByID(ctx, request.OrgID, assetID)
}

func (s *Storage) GetAssetViewByID(ctx context.Context, orgID, id int) (*asset.AssetView, error) {
	baseAsset, err := s.GetAssetByID(ctx, orgID, &id)
	if err != nil {
		return nil, err
	}
	if baseAsset == nil {
		return nil, nil
	}

	tags, err := s.GetTagsByAssetID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	return &asset.AssetView{
		Asset: *baseAsset,
		Tags:  tags,
	}, nil
}

// getAssetWithLocationByID returns an AssetWithLocation by surrogate id,
// performing the LEFT JOIN on current location and fetching tags. Used by
// CreateAssetWithTags and UpdateAsset to emit the public write-response
// shape. Returns (nil, nil) if the asset doesn't exist or is soft-deleted.
func (s *Storage) getAssetWithLocationByID(ctx context.Context, orgID, id int) (*asset.AssetWithLocation, error) {
	// TRA-576: align with ListAssetsFiltered. Latest scan wins; explicit
	// current_location_id is the fallback (TRA-495). Selecting the
	// coalesced expression for both the int and the JOIN guarantees the
	// FK pair is always derived from the same row — current_location_id
	// and current_location_external_key are populated or null together.
	query := `
		WITH latest_scan AS (
			SELECT s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $2 AND s.asset_id = $1
			ORDER BY s.timestamp DESC
			LIMIT 1
		)
		SELECT
			a.id, a.org_id, a.external_key, a.name, a.description,
			COALESCE(ls.location_id, a.current_location_id),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.external_key
		FROM trakrf.assets a
		LEFT JOIN latest_scan ls ON true
		LEFT JOIN trakrf.locations l
			ON l.id = COALESCE(ls.location_id, a.current_location_id)
			AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE a.id = $1 AND a.org_id = $2 AND a.deleted_at IS NULL
		LIMIT 1
	`
	var (
		a         asset.Asset
		locExtKey *string
	)
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, orgID).Scan(
			&a.ID, &a.OrgID, &a.ExternalKey, &a.Name, &a.Description,
			&a.LocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata,
			&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
			&locExtKey,
		)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get asset with location by id: %w", err)
	}

	tags, err := s.GetTagsByAssetID(ctx, orgID, a.ID)
	if err != nil {
		return nil, err
	}

	return &asset.AssetWithLocation{
		AssetView: asset.AssetView{
			Asset: a,
			Tags:  tags,
		},
		LocationExternalKey: locExtKey,
	}, nil
}

func (s *Storage) ListAssetViews(ctx context.Context, orgID, limit, offset int) ([]asset.AssetView, error) {
	assets, err := s.ListAllAssets(ctx, orgID, limit, offset)
	if err != nil {
		return nil, err
	}

	if len(assets) == 0 {
		return []asset.AssetView{}, nil
	}

	assetIDs := make([]int, len(assets))
	for i, a := range assets {
		assetIDs[i] = a.ID
	}

	tagMap, err := s.getTagsForAssets(ctx, orgID, assetIDs)
	if err != nil {
		return nil, err
	}

	views := make([]asset.AssetView, len(assets))
	for i, a := range assets {
		ids := tagMap[a.ID]
		if ids == nil {
			ids = []shared.Tag{}
		}
		views[i] = asset.AssetView{
			Asset: a,
			Tags:  ids,
		}
	}

	return views, nil
}

// GetAssetByExternalKey returns the live (non-deleted) asset with the given
// natural key for the given org, plus the current location's external_key.
// Returns (nil, nil) if no match.
func (s *Storage) GetAssetByExternalKey(
	ctx context.Context, orgID int, externalKey string,
) (*asset.AssetWithLocation, error) {
	// TRA-576: same scan-first / FK-fallback expression as
	// ListAssetsFiltered and getAssetWithLocationByID, so all read paths
	// return identical (current_location_id, current_location_external_key)
	// pairs.
	query := `
		WITH latest_scan AS (
			SELECT s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1 AND s.asset_id = (
				SELECT id FROM trakrf.assets
				WHERE org_id = $1 AND external_key = $2 AND deleted_at IS NULL
				LIMIT 1
			)
			ORDER BY s.timestamp DESC
			LIMIT 1
		)
		SELECT
			a.id, a.org_id, a.external_key, a.name, a.description,
			COALESCE(ls.location_id, a.current_location_id),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.external_key
		FROM trakrf.assets a
		LEFT JOIN latest_scan ls ON true
		LEFT JOIN trakrf.locations l
			ON l.id = COALESCE(ls.location_id, a.current_location_id)
			AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE a.org_id = $1 AND a.external_key = $2 AND a.deleted_at IS NULL
		LIMIT 1
	`
	var (
		a         asset.Asset
		locExtKey *string
	)
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, externalKey).Scan(
			&a.ID, &a.OrgID, &a.ExternalKey, &a.Name, &a.Description,
			&a.LocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata,
			&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
			&locExtKey,
		)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get asset by external_key: %w", err)
	}

	tags, err := s.GetTagsByAssetID(ctx, orgID, a.ID)
	if err != nil {
		return nil, err
	}

	return &asset.AssetWithLocation{
		AssetView: asset.AssetView{
			Asset: a,
			Tags:  tags,
		},
		LocationExternalKey: locExtKey,
	}, nil
}

// GetAssetIDsByExternalKeys resolves a batch of natural external_keys to
// internal surrogate IDs for one org. Returns a map keyed by external_key;
// entries not found in the org are absent from the map. Empty/nil input
// returns an empty map without querying.
//
// Used by inventory/save (TRA-448) to convert public-API external_key lists
// to the numeric IDs the storage layer expects.
func (s *Storage) GetAssetIDsByExternalKeys(
	ctx context.Context, orgID int, externalKeys []string,
) (map[string]int, error) {
	if len(externalKeys) == 0 {
		return map[string]int{}, nil
	}

	query := `
		SELECT external_key, id
		FROM trakrf.assets
		WHERE org_id = $1 AND external_key = ANY($2) AND deleted_at IS NULL
	`
	out := make(map[string]int, len(externalKeys))
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, externalKeys)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var (
				ek string
				id int
			)
			if err := rows.Scan(&ek, &id); err != nil {
				return fmt.Errorf("scan asset external_key row: %w", err)
			}
			out[ek] = id
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate asset external_key rows: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get asset ids by external_keys: %w", err)
	}
	return out, nil
}

// ListAssetsFiltered returns assets matching the filter, joined with their
// current location's natural key. Sort fields allowlisted by handler.
func (s *Storage) ListAssetsFiltered(
	ctx context.Context, orgID int, f asset.ListFilter,
) ([]asset.AssetWithLocation, error) {
	where, args := buildAssetsWhere(orgID, f)
	orderBy := buildAssetsOrderBy(f.Sorts)

	// Latest scan wins (TRA-465: ?location filter follows scans, not the
	// stale current_location_id column). When an asset has no scan history,
	// fall back to the explicit FK so create-only assets still surface a
	// location (TRA-495).
	query := fmt.Sprintf(`
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT
			a.id, a.org_id, a.external_key, a.name, a.description,
			COALESCE(ls.location_id, a.current_location_id),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.external_key
		FROM trakrf.assets a
		LEFT JOIN latest_scans ls ON ls.asset_id = a.id
		LEFT JOIN trakrf.locations l
			ON l.id = COALESCE(ls.location_id, a.current_location_id)
			AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, where, orderBy, len(args)+1, len(args)+2)

	args = append(args, clampAssetListLimit(f.Limit), f.Offset)

	out := []asset.AssetWithLocation{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var (
				a         asset.Asset
				locExtKey *string
			)
			if err := rows.Scan(
				&a.ID, &a.OrgID, &a.ExternalKey, &a.Name, &a.Description,
				&a.LocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata,
				&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
				&locExtKey,
			); err != nil {
				return fmt.Errorf("scan asset: %w", err)
			}
			out = append(out, asset.AssetWithLocation{
				AssetView:           asset.AssetView{Asset: a, Tags: nil},
				LocationExternalKey: locExtKey,
			})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list assets filtered: %w", err)
	}

	// Bulk-fetch tags for the returned assets.
	if len(out) > 0 {
		ids := make([]int, len(out))
		for i, a := range out {
			ids[i] = a.ID
		}
		tagMap, err := s.getTagsForAssets(ctx, orgID, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Tags = tagMap[out[i].ID]
			if out[i].Tags == nil {
				out[i].Tags = []shared.Tag{}
			}
		}
	}

	return out, nil
}

// CountAssetsFiltered returns total matching count (ignores limit/offset/sort).
func (s *Storage) CountAssetsFiltered(
	ctx context.Context, orgID int, f asset.ListFilter,
) (int, error) {
	where, args := buildAssetsWhere(orgID, f)
	query := fmt.Sprintf(`
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT COUNT(*)
		FROM trakrf.assets a
		LEFT JOIN latest_scans ls ON ls.asset_id = a.id
		LEFT JOIN trakrf.locations l
			ON l.id = COALESCE(ls.location_id, a.current_location_id)
			AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE %s
	`, where)

	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("count assets filtered: %w", err)
	}
	return n, nil
}

func buildAssetsWhere(orgID int, f asset.ListFilter) (string, []any) {
	clauses := []string{
		"a.org_id = $1",
		"a.deleted_at IS NULL",
		temporallyEffective("a"),
	}
	args := []any{orgID}

	// location_id and location_external_key combine with OR semantics — a row
	// matches if its current location appears in either set.
	hasIDs := len(f.LocationIDs) > 0
	hasExtKeys := len(f.LocationExternalKeys) > 0
	if hasIDs && hasExtKeys {
		args = append(args, f.LocationIDs)
		idIdx := len(args)
		args = append(args, f.LocationExternalKeys)
		ekIdx := len(args)
		clauses = append(clauses, fmt.Sprintf("(l.id = ANY($%d::int[]) OR l.external_key = ANY($%d::text[]))", idIdx, ekIdx))
	} else if hasIDs {
		args = append(args, f.LocationIDs)
		clauses = append(clauses, fmt.Sprintf("l.id = ANY($%d::int[])", len(args)))
	} else if hasExtKeys {
		args = append(args, f.LocationExternalKeys)
		clauses = append(clauses, fmt.Sprintf("l.external_key = ANY($%d::text[])", len(args)))
	}

	if len(f.ExternalKeys) > 0 {
		args = append(args, f.ExternalKeys)
		clauses = append(clauses, fmt.Sprintf("a.external_key = ANY($%d::text[])", len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		clauses = append(clauses, fmt.Sprintf("a.is_active = $%d", len(args)))
	}
	if f.Q != nil {
		args = append(args, "%"+*f.Q+"%")
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(a.name ILIKE $%d OR a.external_key ILIKE $%d OR a.description ILIKE $%d "+
				"OR EXISTS (SELECT 1 FROM trakrf.tags i "+
				"WHERE i.asset_id = a.id AND i.deleted_at IS NULL "+
				"AND "+temporallyEffective("i")+" AND i.value ILIKE $%d))",
			idx, idx, idx, idx))
	}
	return strings.Join(clauses, " AND "), args
}

func buildAssetsOrderBy(sorts []asset.ListSort) string {
	if len(sorts) == 0 {
		return "a.external_key ASC"
	}
	out := make([]string, 0, len(sorts))
	for _, s := range sorts {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		out = append(out, "a."+s.Field+" "+dir)
	}
	return strings.Join(out, ", ")
}

func clampAssetListLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

func parseAssetWithTagsError(err error, externalKey string) error {
	errStr := err.Error()

	if strings.Contains(errStr, "assets_org_id_external_key") ||
		(strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "assets")) {
		return fmt.Errorf("asset with external_key %s already exists", externalKey)
	}

	if strings.Contains(errStr, "tags_org_id_type_value") ||
		(strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "tags")) {
		return fmt.Errorf("one or more tags already exist")
	}

	if strings.Contains(errStr, "current_location_id_fkey") {
		return fmt.Errorf("invalid location_id: location does not exist")
	}

	return fmt.Errorf("failed to create asset with tags: %w", err)
}

// GetAssetWithLocationByIDForTest exposes getAssetWithLocationByID to integration
// tests in the same package. Production code must use GetAssetByExternalKey or
// the CreateAssetWithTags / UpdateAsset return values.
func (s *Storage) GetAssetWithLocationByIDForTest(ctx context.Context, orgID, id int) (*asset.AssetWithLocation, error) {
	return s.getAssetWithLocationByID(ctx, orgID, id)
}
