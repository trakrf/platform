package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

func (s *Storage) GetTagsByAssetID(ctx context.Context, orgID, assetID int) ([]shared.Tag, error) {
	query := `
		SELECT id, type, value
		FROM trakrf.tags i
		WHERE asset_id = $1 AND org_id = $2 AND deleted_at IS NULL
		  AND ` + temporallyEffective("i") + `
		ORDER BY created_at ASC
	`

	var tags []shared.Tag
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, assetID, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		tags = []shared.Tag{}
		for rows.Next() {
			var tag shared.Tag
			if err := rows.Scan(&tag.ID, &tag.TagType, &tag.Value); err != nil {
				return fmt.Errorf("failed to scan tag: %w", err)
			}
			tags = append(tags, tag)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for asset: %w", err)
	}

	return tags, nil
}

func (s *Storage) GetTagsByLocationID(ctx context.Context, orgID, locationID int) ([]shared.Tag, error) {
	query := `
		SELECT id, type, value
		FROM trakrf.tags i
		WHERE location_id = $1 AND org_id = $2 AND deleted_at IS NULL
		  AND ` + temporallyEffective("i") + `
		ORDER BY created_at ASC
	`

	var tags []shared.Tag
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, locationID, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		tags = []shared.Tag{}
		for rows.Next() {
			var tag shared.Tag
			if err := rows.Scan(&tag.ID, &tag.TagType, &tag.Value); err != nil {
				return fmt.Errorf("failed to scan tag: %w", err)
			}
			tags = append(tags, tag)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for location: %w", err)
	}

	return tags, nil
}

func (s *Storage) AddTagToAsset(ctx context.Context, orgID, assetID int, req shared.TagRequest) (*shared.Tag, error) {
	query := `
		INSERT INTO trakrf.tags (org_id, type, value, asset_id, is_active)
		VALUES ($1, $2, $3, $4, TRUE)
		RETURNING id, type, value
	`

	tagType := req.GetType()
	var tag shared.Tag

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, tagType, req.Value, assetID).Scan(
			&tag.ID, &tag.TagType, &tag.Value,
		)
	})

	if err != nil {
		return nil, s.resolveTagError(ctx, orgID, err, tagType, req.Value)
	}

	return &tag, nil
}

func (s *Storage) AddTagToLocation(ctx context.Context, orgID, locationID int, req shared.TagRequest) (*shared.Tag, error) {
	query := `
		INSERT INTO trakrf.tags (org_id, type, value, location_id, is_active)
		VALUES ($1, $2, $3, $4, TRUE)
		RETURNING id, type, value
	`

	tagType := req.GetType()
	var tag shared.Tag

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, tagType, req.Value, locationID).Scan(
			&tag.ID, &tag.TagType, &tag.Value,
		)
	})

	if err != nil {
		return nil, s.resolveTagError(ctx, orgID, err, tagType, req.Value)
	}

	return &tag, nil
}

// RemoveAssetTag soft-deletes a tag that is attached to the
// given assetID AND is owned by an asset in the caller's org. The assetID
// parameter is load-bearing: it guards against cross-asset path manipulation
// (e.g. DELETE /assets/1/tags/42 where tag 42 actually belongs
// to asset 7).
func (s *Storage) RemoveAssetTag(ctx context.Context, orgID, assetID, tagID int) (bool, error) {
	query := `
		UPDATE trakrf.tags
		SET deleted_at = NOW()
		WHERE id = $1
		  AND asset_id = $2
		  AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM trakrf.assets WHERE id = $2 AND org_id = $3)
	`

	var affected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, query, tagID, assetID, orgID)
		if err != nil {
			return err
		}
		affected = result.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to remove asset tag: %w", err)
	}

	return affected > 0, nil
}

// RemoveLocationTag soft-deletes a tag that is attached to the
// given locationID AND is owned by a location in the caller's org. The
// locationID parameter is load-bearing: it guards against cross-location path
// manipulation.
func (s *Storage) RemoveLocationTag(ctx context.Context, orgID, locationID, tagID int) (bool, error) {
	query := `
		UPDATE trakrf.tags
		SET deleted_at = NOW()
		WHERE id = $1
		  AND location_id = $2
		  AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM trakrf.locations WHERE id = $2 AND org_id = $3)
	`

	var affected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, query, tagID, locationID, orgID)
		if err != nil {
			return err
		}
		affected = result.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to remove location tag: %w", err)
	}

	return affected > 0, nil
}

func (s *Storage) GetTagByID(ctx context.Context, orgID, tagID int) (*shared.Tag, error) {
	query := `
		SELECT id, type, value
		FROM trakrf.tags
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
	`

	var tag shared.Tag
	found := false
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, query, tagID, orgID).Scan(
			&tag.ID, &tag.TagType, &tag.Value,
		)
		if err != nil {
			if err.Error() == "no rows in result set" {
				return nil
			}
			return err
		}
		found = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}
	if !found {
		return nil, nil
	}

	return &tag, nil
}

// isTagDuplicateErr reports whether err is the (org_id, type, value)
// partial-unique-index violation on the tags table.
func isTagDuplicateErr(err error) bool {
	if pgErr, ok := err.(*pgconn.PgError); ok {
		return pgErr.ConstraintName == "tags_org_id_type_value_unique"
	}
	return strings.Contains(err.Error(), "duplicate key")
}

// tagConflict describes the entity a tag value is already attached to.
type tagConflict struct {
	EntityType  string // "asset" or "location"
	Name        string
	ExternalKey string
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// lookupTagConflict finds the live tag row colliding on (orgID, tagType,
// value) and returns the asset or location it is attached to. Returns
// (nil, nil) when no live collision is found — e.g. the conflicting row was
// soft-deleted between the failed INSERT and this lookup.
//
// TRA-816: the asset/location join filters deleted_at so that a tag whose
// parent is soft-deleted does not surface that parent's name in the 409
// message. With the DeleteAsset / DeleteLocation cascade now soft-deleting
// attached tags, this code path should not produce a hit for an orphan; the
// defense in depth covers the window between deploy and the sweep migration
// running, and any future code path that bypasses the cascade.
func (s *Storage) lookupTagConflict(ctx context.Context, orgID int, tagType, value string) (*tagConflict, error) {
	query := `
		SELECT t.asset_id, t.location_id,
		       a.name, a.external_key,
		       l.name, l.external_key
		  FROM trakrf.tags t
		  LEFT JOIN trakrf.assets    a ON a.id = t.asset_id    AND a.deleted_at IS NULL
		  LEFT JOIN trakrf.locations l ON l.id = t.location_id AND l.deleted_at IS NULL
		 WHERE t.org_id = $1 AND t.type = $2 AND t.value = $3
		   AND t.deleted_at IS NULL
		 LIMIT 1
	`
	var assetID, locationID *int
	var assetName, assetKey, locName, locKey *string
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, tagType, value).Scan(
			&assetID, &locationID, &assetName, &assetKey, &locName, &locKey,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	switch {
	case assetID != nil && assetName != nil:
		return &tagConflict{EntityType: "asset", Name: derefStr(assetName), ExternalKey: derefStr(assetKey)}, nil
	case locationID != nil && locName != nil:
		return &tagConflict{EntityType: "location", Name: derefStr(locName), ExternalKey: derefStr(locKey)}, nil
	default:
		// Tag row exists but parent is soft-deleted (or no parent). Fall back
		// to the generic "already exists" message via resolveTagError — we
		// must not leak a soft-deleted entity's name to the caller, and the
		// generic message is a server-side data-integrity miss to fix at the
		// cascade layer, not a user-fixable conflict (TRA-816).
		return nil, nil
	}
}

// resolveTagError converts an INSERT error from AddTagToAsset/AddTagToLocation
// into a user-facing error. For the (org, type, value) unique-violation it
// enriches the message by naming the entity already holding the tag;
// everything else delegates to parseTagError. The enriched message keeps the
// "already exists" substring the HTTP handlers match to produce a 409.
func (s *Storage) resolveTagError(ctx context.Context, orgID int, err error, tagType, value string) error {
	if !isTagDuplicateErr(err) {
		return parseTagError(err, tagType, value)
	}
	conflict, lookupErr := s.lookupTagConflict(ctx, orgID, tagType, value)
	if lookupErr != nil || conflict == nil {
		return parseTagError(err, tagType, value) // generic fallback
	}
	return fmt.Errorf(
		"tag %s:%s already exists — it is attached to %s %q (%s); remove it there before attaching here",
		tagType, value, conflict.EntityType, conflict.Name, conflict.ExternalKey,
	)
}

func parseTagError(err error, tagType, value string) error {
	if pgErr, ok := err.(*pgconn.PgError); ok {
		switch pgErr.ConstraintName {
		case "tags_org_id_type_value_unique":
			return fmt.Errorf("tag %s:%s already exists", tagType, value)
		case "tag_target":
			return fmt.Errorf("tag must be linked to exactly one asset or location")
		}
	}

	if strings.Contains(err.Error(), "duplicate key") {
		return fmt.Errorf("tag %s:%s already exists", tagType, value)
	}

	return fmt.Errorf("failed to create tag: %w", err)
}

func tagsToJSON(tags []shared.TagRequest) ([]byte, error) {
	if len(tags) == 0 {
		return []byte("[]"), nil
	}

	// dbTagEntry uses the DB-native "type" key that create_asset_with_tags
	// and create_location_with_tags stored procedures read via ->>'type'.
	// Intentionally decoupled from TagRequest.TagType (which uses
	// json:"tag_type" for the public API surface) so wire-level renames
	// don't perturb the DB contract.
	type dbTagEntry struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}

	normalized := make([]dbTagEntry, len(tags))
	for i, t := range tags {
		normalized[i] = dbTagEntry{
			Type:  t.GetType(),
			Value: t.Value,
		}
	}

	return json.Marshal(normalized)
}

func (s *Storage) getTagsForAssets(ctx context.Context, orgID int, assetIDs []int) (map[int][]shared.Tag, error) {
	query := `
		SELECT asset_id, id, type, value
		FROM trakrf.tags
		WHERE asset_id = ANY($1) AND org_id = $2 AND deleted_at IS NULL
		ORDER BY asset_id, created_at ASC
	`

	var result map[int][]shared.Tag
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, assetIDs, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make(map[int][]shared.Tag)
		for _, id := range assetIDs {
			result[id] = []shared.Tag{}
		}

		for rows.Next() {
			var assetID int
			var id shared.Tag
			if err := rows.Scan(&assetID, &id.ID, &id.TagType, &id.Value); err != nil {
				return fmt.Errorf("failed to scan tag: %w", err)
			}
			result[assetID] = append(result[assetID], id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to batch fetch tags: %w", err)
	}

	return result, nil
}

func (s *Storage) getTagsForLocations(ctx context.Context, orgID int, locationIDs []int) (map[int][]shared.Tag, error) {
	query := `
		SELECT location_id, id, type, value
		FROM trakrf.tags
		WHERE location_id = ANY($1) AND org_id = $2 AND deleted_at IS NULL
		ORDER BY location_id, created_at ASC
	`

	var result map[int][]shared.Tag
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, locationIDs, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make(map[int][]shared.Tag)
		for _, id := range locationIDs {
			result[id] = []shared.Tag{}
		}

		for rows.Next() {
			var locationID int
			var id shared.Tag
			if err := rows.Scan(&locationID, &id.ID, &id.TagType, &id.Value); err != nil {
				return fmt.Errorf("failed to scan tag: %w", err)
			}
			result[locationID] = append(result[locationID], id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to batch fetch tags: %w", err)
	}

	return result, nil
}

// LookupResult contains the entity found by tag lookup
type LookupResult struct {
	EntityType string             `json:"entity_type"` // "asset" or "location"
	EntityID   int                `json:"entity_id"`
	Asset      *asset.Asset       `json:"asset,omitempty"`
	Location   *location.Location `json:"location,omitempty"`
}

// normalizeEPC strips leading zeros from an EPC value for comparison
func normalizeEPC(epc string) string {
	return strings.TrimLeft(epc, "0")
}

// LookupByTagValues finds assets/locations for multiple tag values (batch)
// Returns a map of value -> LookupResult (nil if not found)
// Note: Comparison is done with leading zeros stripped (normalized) to handle
// cases where scanner returns EPCs with different leading zero counts than stored.
func (s *Storage) LookupByTagValues(ctx context.Context, orgID int, tagType string, values []string) (map[string]*LookupResult, error) {
	if len(values) == 0 {
		return make(map[string]*LookupResult), nil
	}

	// Build map of normalized EPC -> original input values
	// Multiple originals could normalize to the same value (e.g., "00ABC" and "ABC")
	normalizedToOriginals := make(map[string][]string)
	normalizedValues := make([]string, 0, len(values))
	for _, v := range values {
		norm := normalizeEPC(v)
		if _, exists := normalizedToOriginals[norm]; !exists {
			normalizedValues = append(normalizedValues, norm)
		}
		normalizedToOriginals[norm] = append(normalizedToOriginals[norm], v)
	}

	// Query using LTRIM to normalize stored values for comparison
	query := `
		SELECT value, asset_id, location_id
		FROM trakrf.tags
		WHERE org_id = $1 AND type = $2 AND LTRIM(value, '0') = ANY($3) AND deleted_at IS NULL
	`

	// Collect tag data with normalized value for mapping
	type tagRow struct {
		value      string // Original value from DB
		normalized string // Normalized for matching back to input
		assetID    *int
		locationID *int
	}
	var tagRows []tagRow

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, tagType, normalizedValues)
		if err != nil {
			return fmt.Errorf("failed to batch lookup tags: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var row tagRow
			if err := rows.Scan(&row.value, &row.assetID, &row.locationID); err != nil {
				return fmt.Errorf("failed to scan tag row: %w", err)
			}
			row.normalized = normalizeEPC(row.value)
			tagRows = append(tagRows, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to batch lookup tags: %w", err)
	}

	// Collect unique asset and location IDs for batch fetch
	assetIDs := make([]int, 0)
	locationIDs := make([]int, 0)
	for _, row := range tagRows {
		if row.assetID != nil {
			assetIDs = append(assetIDs, *row.assetID)
		}
		if row.locationID != nil {
			locationIDs = append(locationIDs, *row.locationID)
		}
	}

	// Batch fetch assets
	assetMap := make(map[int]*asset.Asset)
	if len(assetIDs) > 0 {
		assets, err := s.GetAssetsByIDs(ctx, orgID, assetIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to batch fetch assets: %w", err)
		}
		for _, a := range assets {
			assetMap[a.ID] = a
		}
	}

	// Batch fetch locations
	locationMap := make(map[int]*location.Location)
	if len(locationIDs) > 0 {
		locations, err := s.GetLocationsByIDs(ctx, orgID, locationIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to batch fetch locations: %w", err)
		}
		for _, loc := range locations {
			locationMap[loc.ID] = loc
		}
	}

	// Build result map keyed by ORIGINAL input values
	result := make(map[string]*LookupResult)
	for _, row := range tagRows {
		var lookupResult *LookupResult

		if row.assetID != nil {
			if a, ok := assetMap[*row.assetID]; ok {
				lookupResult = &LookupResult{
					EntityType: "asset",
					EntityID:   *row.assetID,
					Asset:      a,
				}
			}
		} else if row.locationID != nil {
			if loc, ok := locationMap[*row.locationID]; ok {
				lookupResult = &LookupResult{
					EntityType: "location",
					EntityID:   *row.locationID,
					Location:   loc,
				}
			}
		}

		if lookupResult != nil {
			// Map result to ALL original input values that normalize to this match
			for _, originalValue := range normalizedToOriginals[row.normalized] {
				result[originalValue] = lookupResult
			}
		}
	}

	return result, nil
}

// LookupByTagValue finds an asset or location by its tag value
// Note: Comparison is done with leading zeros stripped (normalized)
func (s *Storage) LookupByTagValue(ctx context.Context, orgID int, tagType, value string) (*LookupResult, error) {
	normalizedValue := normalizeEPC(value)

	query := `
		SELECT asset_id, location_id
		FROM trakrf.tags
		WHERE org_id = $1 AND type = $2 AND LTRIM(value, '0') = $3 AND deleted_at IS NULL
	`

	var assetID, locationID *int
	found := false
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, query, orgID, tagType, normalizedValue).Scan(&assetID, &locationID)
		if err != nil {
			if err.Error() == "no rows in result set" {
				return nil
			}
			return err
		}
		found = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to lookup tag: %w", err)
	}
	if !found {
		return nil, nil
	}

	if assetID != nil {
		a, err := s.GetAssetByID(ctx, orgID, assetID)
		if err != nil {
			return nil, err
		}
		return &LookupResult{EntityType: "asset", EntityID: *assetID, Asset: a}, nil
	}

	if locationID != nil {
		loc, err := s.GetLocationByID(ctx, orgID, *locationID)
		if err != nil {
			return nil, err
		}
		return &LookupResult{EntityType: "location", EntityID: *locationID, Location: loc}, nil
	}

	return nil, nil
}
