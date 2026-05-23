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

	// TRA-674: COALESCE(description, '') defends against legacy rows where the
	// nullable text column holds SQL NULL — pgx cannot scan NULL into the
	// non-pointer asset.Asset.Description (`string`) and surfaces a 500.
	query := `
	insert into trakrf.assets
	(name, external_key, description, valid_from, valid_to, metadata, is_active, org_id)
	values ($1, $2, $3, $4, $5, $6, $7, $8)
	returning id, org_id, external_key, name, COALESCE(description, ''), valid_from, valid_to,
	          metadata, is_active, created_at, updated_at, deleted_at
	`
	var asset asset.Asset
	err := s.WithOrgTx(ctx, request.OrgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, request.Name, request.ExternalKey,
			request.Description, request.ValidFrom, request.ValidTo, request.Metadata,
			request.IsActive, request.OrgID,
		).Scan(&asset.ID, &asset.OrgID, &asset.ExternalKey, &asset.Name,
			&asset.Description, &asset.ValidFrom, &asset.ValidTo, &asset.Metadata,
			&asset.IsActive, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt,
		)
	})

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("asset with external_key %s already exists", request.ExternalKey)
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

func (s *Storage) UpdateAsset(ctx context.Context, orgID, id int, request asset.UpdateAssetRequest) (*asset.AssetView, error) {
	setClauses := []string{}
	args := []any{id, orgID}
	argPos := 3
	fields, err := mapReqToFields(request)

	if err != nil {
		return nil, err
	}

	// Nil entries (only from ClearValidTo) pass through as SQL NULL.
	for key, value := range fields {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, argPos))
		args = append(args, value)
		argPos++
	}

	// TRA-783: every accepted PATCH advances updated_at — filesystem `touch`
	// semantics. Pre-TRA-783 the storage layer applied a per-row
	// IS DISTINCT FROM gate that skipped the UPDATE (and thus updated_at)
	// when no settable field's value differed from the current state. That
	// model broke for valid_from/valid_to when storage precision (µs)
	// exceeded wire precision (ms) — server-defaulted or sub-ms client
	// inputs round-tripped to a wire value that compared as "different"
	// against storage but as "identical" from the integrator's POV. The
	// new rule: every accepted PATCH advances updated_at, including empty
	// body (`{}`) and verbatim writable echoes. Removes all edge cases;
	// matches POLS expectations from filesystems where any successful
	// write advances mtime. Concurrency-token semantics on updated_at
	// (echo-current-value check in the handler) are unaffected.
	setClauses = append(setClauses, "updated_at = NOW()")

	query := fmt.Sprintf(`
		update trakrf.assets
		set %s
		where id = $1 and org_id = $2 and deleted_at is null
		returning id
	`, strings.Join(setClauses, ", "))

	var updatedID int
	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&updatedID)
	})

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		// external_key is immutable via UpdateAsset (TRA-664); the only
		// uniqueness collision reachable here would be a future-added
		// unique column. Keep the generic conflict error.
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("asset update conflicts with an existing unique constraint")
		}
		return nil, fmt.Errorf("failed to update asset: %w", err)
	}

	return s.getAssetViewWithTagsByID(ctx, orgID, updatedID)
}

// RenameAsset mutates the asset's external_key (natural / join key). TRA-664
// / BB26 D7: this is the only path for changing external_key — PATCH
// rejects the field as immutable. Uniqueness is enforced by the assets
// table's UNIQUE (org_id, external_key) constraint; collisions surface as
// "already exists" so the handler can map them to 409 conflict, matching
// CreateAsset's behavior.
func (s *Storage) RenameAsset(ctx context.Context, orgID, id int, newExternalKey string) (*asset.AssetView, error) {
	var updatedID int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var currentKey string
		err := tx.QueryRow(ctx, `
			SELECT external_key FROM trakrf.assets
			WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		`, id, orgID).Scan(&currentKey)
		if err != nil {
			return err
		}

		// TRA-731 / BB39 F3: same-value rename does not observably mutate
		// the resource. Skip the UPDATE so updated_at stays stable for
		// integrators following the cached-body PATCH pattern.
		if currentKey == newExternalKey {
			updatedID = id
			return nil
		}

		return tx.QueryRow(ctx, `
			UPDATE trakrf.assets
			SET external_key = $3, updated_at = NOW()
			WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
			RETURNING id
		`, id, orgID, newExternalKey).Scan(&updatedID)
	})

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("asset with external_key %s already exists", newExternalKey)
		}
		return nil, fmt.Errorf("failed to rename asset: %w", err)
	}

	return s.getAssetViewWithTagsByID(ctx, orgID, updatedID)
}

func (s *Storage) GetAssetByID(ctx context.Context, orgID int, id *int) (*asset.Asset, error) {
	// TRA-674: COALESCE(description, '') — see CreateAsset comment.
	query := `
	select id, org_id, external_key, name, COALESCE(description, ''), valid_from, valid_to,
	       metadata, is_active, created_at, updated_at, deleted_at
	from trakrf.assets
	where id = $1 and org_id = $2 and deleted_at is null
	`
	var asset asset.Asset
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, orgID).Scan(&asset.ID, &asset.OrgID,
			&asset.ExternalKey, &asset.Name, &asset.Description,
			&asset.ValidFrom, &asset.ValidTo, &asset.Metadata, &asset.IsActive,
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

	// TRA-674: COALESCE(description, '') — see CreateAsset comment.
	query := `
	SELECT id, org_id, external_key, name, COALESCE(description, ''), valid_from, valid_to,
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
				&a.Description, &a.ValidFrom, &a.ValidTo, &a.Metadata, &a.IsActive,
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
	// TRA-674: COALESCE(description, '') — see CreateAsset comment.
	query := `
		select id, org_id, external_key, name, COALESCE(description, ''), valid_from, valid_to,
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
				&a.Description, &a.ValidFrom, &a.ValidTo, &a.Metadata, &a.IsActive,
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

// DeleteAsset soft-deletes an asset and cascades the same deleted_at to any
// attached tag rows in one transaction. TRA-816: without the cascade the
// orphan tag row keeps the (org_id, type, value) unique slot occupied, so the
// value cannot be reattached elsewhere.
func (s *Storage) DeleteAsset(ctx context.Context, orgID, id int) (bool, error) {
	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			UPDATE trakrf.assets
			   SET deleted_at = NOW()
			 WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		`, id, orgID)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		if rowsAffected == 0 {
			return nil
		}
		_, err = tx.Exec(ctx, `
			UPDATE trakrf.tags
			   SET deleted_at = (SELECT deleted_at FROM trakrf.assets WHERE id = $1 AND org_id = $2)
			 WHERE asset_id = $1 AND org_id = $2 AND deleted_at IS NULL
		`, id, orgID)
		return err
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
		(name, external_key, description, valid_from, valid_to, metadata, is_active, org_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		for i, a := range assets {
			if _, err := tx.Exec(ctx, query,
				a.Name, a.ExternalKey, a.Description,
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
	// dedicated tooling, never a public PATCH body. external_key is also
	// not writable here (TRA-664 / BB26 D7); see RenameAsset for that path.
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	// description: explicit null on PATCH clears to empty string. The DB column
	// is nullable but ToPublicAssetView projects "" → null on read, so storing
	// "" preserves the null-on-read contract without forcing every scan call
	// to handle SQL NULL into a Go string. (TRA-614 / BB19 §S1.)
	if req.ClearDescription {
		fields["description"] = ""
	} else if req.Description != nil {
		fields["description"] = *req.Description
	}
	// TRA-799: asset location is not part of the asset resource — it is
	// scan-derived fact data. location_id / location_external_key are
	// pre-decode-rejected by the PATCH handler and never reach this struct.
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

func (s *Storage) CreateAssetWithTags(ctx context.Context, request asset.CreateAssetWithTagsRequest) (*asset.AssetView, error) {
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

	query := `SELECT * FROM trakrf.create_asset_with_tags($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	var assetID int
	var tagIDs []int

	description := ""
	if request.Description != nil {
		description = *request.Description
	}

	// TRA-734 (BB40 F3) / TRA-799: asset location is scan/operational fact
	// data, not part of the asset resource. create_asset_with_tags no longer
	// takes a location parameter (migration 000043).
	err = s.WithOrgTx(ctx, request.OrgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			request.OrgID,
			request.ExternalKey,
			request.Name,
			description,
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

	return s.getAssetViewWithTagsByID(ctx, request.OrgID, assetID)
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

// getAssetViewWithTagsByID returns an AssetView (asset row + tags) by
// surrogate id. Used by CreateAssetWithTags / UpdateAsset / RenameAsset to
// emit the public write-response shape. Returns (nil, nil) if the asset
// doesn't exist or is soft-deleted.
func (s *Storage) getAssetViewWithTagsByID(ctx context.Context, orgID, id int) (*asset.AssetView, error) {
	query := `
		SELECT
			a.id, a.org_id, a.external_key, a.name, COALESCE(a.description, ''),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at
		FROM trakrf.assets a
		WHERE a.id = $1 AND a.org_id = $2 AND a.deleted_at IS NULL
		LIMIT 1
	`
	var a asset.Asset
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, orgID).Scan(
			&a.ID, &a.OrgID, &a.ExternalKey, &a.Name, &a.Description,
			&a.ValidFrom, &a.ValidTo, &a.Metadata,
			&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
		)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get asset view by id: %w", err)
	}

	tags, err := s.GetTagsByAssetID(ctx, orgID, a.ID)
	if err != nil {
		return nil, err
	}

	return &asset.AssetView{
		Asset: a,
		Tags:  tags,
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
// natural key for the given org. Returns (nil, nil) if no match.
func (s *Storage) GetAssetByExternalKey(
	ctx context.Context, orgID int, externalKey string,
) (*asset.AssetView, error) {
	query := `
		SELECT
			a.id, a.org_id, a.external_key, a.name, COALESCE(a.description, ''),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at
		FROM trakrf.assets a
		WHERE a.org_id = $1 AND a.external_key = $2 AND a.deleted_at IS NULL
		LIMIT 1
	`
	var a asset.Asset
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, externalKey).Scan(
			&a.ID, &a.OrgID, &a.ExternalKey, &a.Name, &a.Description,
			&a.ValidFrom, &a.ValidTo, &a.Metadata,
			&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
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

	return &asset.AssetView{
		Asset: a,
		Tags:  tags,
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

// ListAssetsFiltered returns assets matching the filter. Sort fields
// allowlisted by handler.
func (s *Storage) ListAssetsFiltered(
	ctx context.Context, orgID int, f asset.ListFilter,
) ([]asset.AssetView, error) {
	where, args := buildAssetsWhere(orgID, f)
	orderBy := buildAssetsOrderBy(f.Sorts)

	query := fmt.Sprintf(`
		SELECT
			a.id, a.org_id, a.external_key, a.name, COALESCE(a.description, ''),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at
		FROM trakrf.assets a
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, where, orderBy, len(args)+1, len(args)+2)

	args = append(args, clampAssetListLimit(f.Limit), f.Offset)

	out := []asset.AssetView{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a asset.Asset
			if err := rows.Scan(
				&a.ID, &a.OrgID, &a.ExternalKey, &a.Name, &a.Description,
				&a.ValidFrom, &a.ValidTo, &a.Metadata,
				&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
			); err != nil {
				return fmt.Errorf("scan asset: %w", err)
			}
			out = append(out, asset.AssetView{Asset: a, Tags: nil})
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
		SELECT COUNT(*)
		FROM trakrf.assets a
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
	// TRA-659 / BB25 A3: include_deleted relaxes the soft-delete filter so
	// callers reconciling against an external system of record can enumerate
	// deleted rows alongside live ones. Temporal validity still applies.
	// Orthogonal to is_active.
	clauses := []string{
		"a.org_id = $1",
		temporallyEffective("a"),
	}
	if !f.IncludeDeleted {
		clauses = append(clauses, "a.deleted_at IS NULL")
	}
	args := []any{orgID}

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
				"WHERE i.asset_id = a.id AND i.is_active = true "+
				"AND i.deleted_at IS NULL AND "+temporallyEffective("i")+
				" AND i.value ILIKE $%d))",
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

	return fmt.Errorf("failed to create asset with tags: %w", err)
}

// GetAssetViewWithTagsByID exposes getAssetViewWithTagsByID so handlers (and
// integration tests) can fetch an asset plus its tags in one round-trip. Used
// by the PATCH handler's echo check against current state.
func (s *Storage) GetAssetViewWithTagsByID(ctx context.Context, orgID, id int) (*asset.AssetView, error) {
	return s.getAssetViewWithTagsByID(ctx, orgID, id)
}
