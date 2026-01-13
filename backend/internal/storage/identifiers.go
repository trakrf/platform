package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

func (s *Storage) GetIdentifiersByAssetID(ctx context.Context, assetID int) ([]shared.TagIdentifier, error) {
	query := `
		SELECT id, type, value, is_active
		FROM trakrf.identifiers
		WHERE asset_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
	`

	rows, err := s.pool.Query(ctx, query, assetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get identifiers for asset: %w", err)
	}
	defer rows.Close()

	var identifiers []shared.TagIdentifier
	for rows.Next() {
		var id shared.TagIdentifier
		if err := rows.Scan(&id.ID, &id.Type, &id.Value, &id.IsActive); err != nil {
			return nil, fmt.Errorf("failed to scan identifier: %w", err)
		}
		identifiers = append(identifiers, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating identifiers: %w", err)
	}

	if identifiers == nil {
		identifiers = []shared.TagIdentifier{}
	}

	return identifiers, nil
}

func (s *Storage) GetIdentifiersByLocationID(ctx context.Context, locationID int) ([]shared.TagIdentifier, error) {
	query := `
		SELECT id, type, value, is_active
		FROM trakrf.identifiers
		WHERE location_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
	`

	rows, err := s.pool.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get identifiers for location: %w", err)
	}
	defer rows.Close()

	var identifiers []shared.TagIdentifier
	for rows.Next() {
		var id shared.TagIdentifier
		if err := rows.Scan(&id.ID, &id.Type, &id.Value, &id.IsActive); err != nil {
			return nil, fmt.Errorf("failed to scan identifier: %w", err)
		}
		identifiers = append(identifiers, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating identifiers: %w", err)
	}

	if identifiers == nil {
		identifiers = []shared.TagIdentifier{}
	}

	return identifiers, nil
}

func (s *Storage) AddIdentifierToAsset(ctx context.Context, orgID, assetID int, req shared.TagIdentifierRequest) (*shared.TagIdentifier, error) {
	query := `
		INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active)
		VALUES ($1, $2, $3, $4, TRUE)
		RETURNING id, type, value, is_active
	`

	identifierType := req.GetType()
	var identifier shared.TagIdentifier

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, identifierType, req.Value, assetID).Scan(
			&identifier.ID, &identifier.Type, &identifier.Value, &identifier.IsActive,
		)
	})

	if err != nil {
		return nil, parseIdentifierError(err, identifierType, req.Value)
	}

	return &identifier, nil
}

func (s *Storage) AddIdentifierToLocation(ctx context.Context, orgID, locationID int, req shared.TagIdentifierRequest) (*shared.TagIdentifier, error) {
	query := `
		INSERT INTO trakrf.identifiers (org_id, type, value, location_id, is_active)
		VALUES ($1, $2, $3, $4, TRUE)
		RETURNING id, type, value, is_active
	`

	identifierType := req.GetType()
	var identifier shared.TagIdentifier

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, identifierType, req.Value, locationID).Scan(
			&identifier.ID, &identifier.Type, &identifier.Value, &identifier.IsActive,
		)
	})

	if err != nil {
		return nil, parseIdentifierError(err, identifierType, req.Value)
	}

	return &identifier, nil
}

func (s *Storage) RemoveIdentifier(ctx context.Context, identifierID int) (bool, error) {
	query := `UPDATE trakrf.identifiers SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

	result, err := s.pool.Exec(ctx, query, identifierID)
	if err != nil {
		return false, fmt.Errorf("failed to remove identifier: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

func (s *Storage) GetIdentifierByID(ctx context.Context, identifierID int) (*shared.TagIdentifier, error) {
	query := `
		SELECT id, type, value, is_active
		FROM trakrf.identifiers
		WHERE id = $1 AND deleted_at IS NULL
	`

	var identifier shared.TagIdentifier
	err := s.pool.QueryRow(ctx, query, identifierID).Scan(
		&identifier.ID, &identifier.Type, &identifier.Value, &identifier.IsActive,
	)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get identifier: %w", err)
	}

	return &identifier, nil
}

func parseIdentifierError(err error, identifierType, value string) error {
	if pgErr, ok := err.(*pgconn.PgError); ok {
		switch pgErr.ConstraintName {
		case "identifiers_org_id_type_value_valid_from_key":
			return fmt.Errorf("identifier %s:%s already exists", identifierType, value)
		case "identifier_target":
			return fmt.Errorf("identifier must be linked to exactly one asset or location")
		}
	}

	if strings.Contains(err.Error(), "duplicate key") {
		return fmt.Errorf("identifier %s:%s already exists", identifierType, value)
	}

	return fmt.Errorf("failed to create identifier: %w", err)
}

func identifiersToJSON(identifiers []shared.TagIdentifierRequest) ([]byte, error) {
	if len(identifiers) == 0 {
		return []byte("[]"), nil
	}

	normalized := make([]shared.TagIdentifierRequest, len(identifiers))
	for i, id := range identifiers {
		normalized[i] = shared.TagIdentifierRequest{
			Type:  id.GetType(),
			Value: id.Value,
		}
	}

	return json.Marshal(normalized)
}

func (s *Storage) getIdentifiersForAssets(ctx context.Context, assetIDs []int) (map[int][]shared.TagIdentifier, error) {
	query := `
		SELECT asset_id, id, type, value, is_active
		FROM trakrf.identifiers
		WHERE asset_id = ANY($1) AND deleted_at IS NULL
		ORDER BY asset_id, created_at ASC
	`

	rows, err := s.pool.Query(ctx, query, assetIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch fetch identifiers: %w", err)
	}
	defer rows.Close()

	result := make(map[int][]shared.TagIdentifier)
	for _, id := range assetIDs {
		result[id] = []shared.TagIdentifier{}
	}

	for rows.Next() {
		var assetID int
		var id shared.TagIdentifier
		if err := rows.Scan(&assetID, &id.ID, &id.Type, &id.Value, &id.IsActive); err != nil {
			return nil, fmt.Errorf("failed to scan identifier: %w", err)
		}
		result[assetID] = append(result[assetID], id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating identifiers: %w", err)
	}

	return result, nil
}

func (s *Storage) getIdentifiersForLocations(ctx context.Context, locationIDs []int) (map[int][]shared.TagIdentifier, error) {
	query := `
		SELECT location_id, id, type, value, is_active
		FROM trakrf.identifiers
		WHERE location_id = ANY($1) AND deleted_at IS NULL
		ORDER BY location_id, created_at ASC
	`

	rows, err := s.pool.Query(ctx, query, locationIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch fetch identifiers: %w", err)
	}
	defer rows.Close()

	result := make(map[int][]shared.TagIdentifier)
	for _, id := range locationIDs {
		result[id] = []shared.TagIdentifier{}
	}

	for rows.Next() {
		var locationID int
		var id shared.TagIdentifier
		if err := rows.Scan(&locationID, &id.ID, &id.Type, &id.Value, &id.IsActive); err != nil {
			return nil, fmt.Errorf("failed to scan identifier: %w", err)
		}
		result[locationID] = append(result[locationID], id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating identifiers: %w", err)
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
		FROM trakrf.identifiers
		WHERE org_id = $1 AND type = $2 AND LTRIM(value, '0') = ANY($3) AND deleted_at IS NULL
	`

	rows, err := s.pool.Query(ctx, query, orgID, tagType, normalizedValues)
	if err != nil {
		return nil, fmt.Errorf("failed to batch lookup tags: %w", err)
	}
	defer rows.Close()

	// Collect identifier data with normalized value for mapping
	type identifierRow struct {
		value      string // Original value from DB
		normalized string // Normalized for matching back to input
		assetID    *int
		locationID *int
	}
	var identifierRows []identifierRow

	for rows.Next() {
		var row identifierRow
		if err := rows.Scan(&row.value, &row.assetID, &row.locationID); err != nil {
			return nil, fmt.Errorf("failed to scan identifier row: %w", err)
		}
		row.normalized = normalizeEPC(row.value)
		identifierRows = append(identifierRows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating identifier rows: %w", err)
	}

	// Collect unique asset and location IDs for batch fetch
	assetIDs := make([]int, 0)
	locationIDs := make([]int, 0)
	for _, row := range identifierRows {
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
		assets, err := s.GetAssetsByIDs(ctx, assetIDs)
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
		locations, err := s.GetLocationsByIDs(ctx, locationIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to batch fetch locations: %w", err)
		}
		for _, loc := range locations {
			locationMap[loc.ID] = loc
		}
	}

	// Build result map keyed by ORIGINAL input values
	result := make(map[string]*LookupResult)
	for _, row := range identifierRows {
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

// LookupByTagValue finds an asset or location by its tag identifier value
// Note: Comparison is done with leading zeros stripped (normalized)
func (s *Storage) LookupByTagValue(ctx context.Context, orgID int, tagType, value string) (*LookupResult, error) {
	normalizedValue := normalizeEPC(value)

	query := `
		SELECT asset_id, location_id
		FROM trakrf.identifiers
		WHERE org_id = $1 AND type = $2 AND LTRIM(value, '0') = $3 AND deleted_at IS NULL
	`

	var assetID, locationID *int
	err := s.pool.QueryRow(ctx, query, orgID, tagType, normalizedValue).Scan(&assetID, &locationID)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to lookup tag: %w", err)
	}

	if assetID != nil {
		a, err := s.GetAssetByID(ctx, assetID)
		if err != nil {
			return nil, err
		}
		return &LookupResult{EntityType: "asset", EntityID: *assetID, Asset: a}, nil
	}

	if locationID != nil {
		loc, err := s.GetLocationByID(ctx, *locationID)
		if err != nil {
			return nil, err
		}
		return &LookupResult{EntityType: "location", EntityID: *locationID, Location: loc}, nil
	}

	return nil, nil
}
