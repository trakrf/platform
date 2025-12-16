# Implementation Plan: TRA-214 (Phase 1 + Phase 2)

## Scope

**Phase 1: Database** - PostgreSQL functions for atomic asset/location + identifier creation
**Phase 2: Models + Asset Storage** - TagIdentifier shared model, AssetView, storage layer for assets

> Phases 3-5 (Location Storage, Handlers, Testing) tracked in Appendix A.

---

## Step 1: Create PostgreSQL Function Migration

**File**: `backend/migrations/000024_identifier_functions.up.sql`

```sql
SET search_path=trakrf,public;

-- Function to create asset with identifiers atomically
-- Returns: asset_id and array of identifier_ids
-- Runs within caller's transaction - exceptions cause full rollback
CREATE OR REPLACE FUNCTION create_asset_with_identifiers(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_type VARCHAR(50),
    p_description TEXT,
    p_current_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_identifiers JSONB  -- array of {type, value}
) RETURNS TABLE (
    asset_id INT,
    identifier_ids INT[]
) AS $$
DECLARE
    v_asset_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    -- Insert asset (trigger generates permuted ID)
    INSERT INTO trakrf.assets (
        org_id, identifier, name, type, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_type, p_description,
        p_current_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    -- Insert each identifier (trigger generates permuted ID)
    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (
                org_id, type, value, asset_id, is_active
            ) VALUES (
                p_org_id,
                v_identifier->>'type',
                v_identifier->>'value',
                v_asset_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_identifier_ids := array_append(v_identifier_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_asset_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;

-- Function to create location with identifiers atomically
CREATE OR REPLACE FUNCTION create_location_with_identifiers(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_type VARCHAR(50),
    p_description TEXT,
    p_parent_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_identifiers JSONB  -- array of {type, value}
) RETURNS TABLE (
    location_id INT,
    identifier_ids INT[]
) AS $$
DECLARE
    v_location_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    -- Insert location (trigger generates permuted ID)
    INSERT INTO trakrf.locations (
        org_id, identifier, name, type, description,
        parent_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_type, p_description,
        p_parent_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    -- Insert each identifier (trigger generates permuted ID)
    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (
                org_id, type, value, location_id, is_active
            ) VALUES (
                p_org_id,
                v_identifier->>'type',
                v_identifier->>'value',
                v_location_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_identifier_ids := array_append(v_identifier_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_location_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;

-- Comments for documentation
COMMENT ON FUNCTION create_asset_with_identifiers IS 'Atomically creates an asset with its tag identifiers. Any failure rolls back entire operation.';
COMMENT ON FUNCTION create_location_with_identifiers IS 'Atomically creates a location with its tag identifiers. Any failure rolls back entire operation.';
```

**Verification**: Run `just backend db-up` and test functions manually.

---

## Step 2: Create Down Migration

**File**: `backend/migrations/000024_identifier_functions.down.sql`

```sql
SET search_path=trakrf,public;

DROP FUNCTION IF EXISTS create_asset_with_identifiers(
    INT, VARCHAR, VARCHAR, VARCHAR, TEXT, INT, TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

DROP FUNCTION IF EXISTS create_location_with_identifiers(
    INT, VARCHAR, VARCHAR, VARCHAR, TEXT, INT, TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);
```

---

## Step 3: Create TagIdentifier Shared Model

**File**: `backend/internal/models/shared/identifier.go`

```go
package shared

// TagIdentifier represents a physical tag (RFID, BLE, barcode) linked to an asset or location.
// Used as embedded type in AssetView and LocationView for API responses.
type TagIdentifier struct {
	ID       int    `json:"id,omitempty"`
	Type     string `json:"type" validate:"required,oneof=rfid ble barcode"`
	Value    string `json:"value" validate:"required,min=1,max=255"`
	IsActive bool   `json:"is_active"`
}

// TagIdentifierRequest is used when creating identifiers (no ID field).
type TagIdentifierRequest struct {
	Type  string `json:"type" validate:"required,oneof=rfid ble barcode"`
	Value string `json:"value" validate:"required,min=1,max=255"`
}
```

---

## Step 4: Update Asset Model with AssetView

**File**: `backend/internal/models/asset/asset.go`

Add after existing `Asset` struct:

```go
import (
	// existing imports...
	"github.com/trakrf/platform/backend/internal/models/shared"
)

// AssetView is the API response model that includes embedded tag identifiers.
// GET endpoints return this instead of raw Asset.
type AssetView struct {
	Asset
	Identifiers []shared.TagIdentifier `json:"identifiers"`
}

// CreateAssetWithIdentifiersRequest extends CreateAssetRequest with optional identifiers.
type CreateAssetWithIdentifiersRequest struct {
	CreateAssetRequest
	Identifiers []shared.TagIdentifierRequest `json:"identifiers,omitempty" validate:"omitempty,dive"`
}

// AssetViewListResponse wraps a list of AssetViews with pagination.
type AssetViewListResponse struct {
	Data       []AssetView       `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
```

---

## Step 5: Add Identifier Error Messages

**File**: `backend/internal/apierrors/messages.go`

Add to existing constants:

```go
// Identifier errors
const (
	IdentifierDuplicateValue    = "identifier with this type and value already exists"
	IdentifierInvalidType       = "identifier type must be rfid, ble, or barcode"
	IdentifierCreateFailed      = "failed to create identifier"
	IdentifierNotFound          = "identifier not found"
	IdentifierDeleteFailed      = "failed to delete identifier"
)
```

---

## Step 6: Create Identifiers Storage Functions

**File**: `backend/internal/storage/identifiers.go` (NEW)

```go
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

// GetIdentifiersByAssetID retrieves all active identifiers for an asset.
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

	// Return empty slice instead of nil for consistent JSON
	if identifiers == nil {
		identifiers = []shared.TagIdentifier{}
	}

	return identifiers, nil
}

// GetIdentifiersByLocationID retrieves all active identifiers for a location.
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

// AddIdentifierToAsset adds a single identifier to an existing asset.
func (s *Storage) AddIdentifierToAsset(ctx context.Context, orgID, assetID int, req shared.TagIdentifierRequest) (*shared.TagIdentifier, error) {
	query := `
		INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active)
		VALUES ($1, $2, $3, $4, TRUE)
		RETURNING id, type, value, is_active
	`

	var identifier shared.TagIdentifier
	err := s.pool.QueryRow(ctx, query, orgID, req.Type, req.Value, assetID).Scan(
		&identifier.ID, &identifier.Type, &identifier.Value, &identifier.IsActive,
	)

	if err != nil {
		return nil, parseIdentifierError(err, req.Type, req.Value)
	}

	return &identifier, nil
}

// AddIdentifierToLocation adds a single identifier to an existing location.
func (s *Storage) AddIdentifierToLocation(ctx context.Context, orgID, locationID int, req shared.TagIdentifierRequest) (*shared.TagIdentifier, error) {
	query := `
		INSERT INTO trakrf.identifiers (org_id, type, value, location_id, is_active)
		VALUES ($1, $2, $3, $4, TRUE)
		RETURNING id, type, value, is_active
	`

	var identifier shared.TagIdentifier
	err := s.pool.QueryRow(ctx, query, orgID, req.Type, req.Value, locationID).Scan(
		&identifier.ID, &identifier.Type, &identifier.Value, &identifier.IsActive,
	)

	if err != nil {
		return nil, parseIdentifierError(err, req.Type, req.Value)
	}

	return &identifier, nil
}

// RemoveIdentifier soft-deletes an identifier by ID.
func (s *Storage) RemoveIdentifier(ctx context.Context, identifierID int) (bool, error) {
	query := `UPDATE trakrf.identifiers SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

	result, err := s.pool.Exec(ctx, query, identifierID)
	if err != nil {
		return false, fmt.Errorf("failed to remove identifier: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

// parseIdentifierError converts PostgreSQL errors to user-friendly messages.
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

// identifiersToJSON converts TagIdentifierRequest slice to JSONB for stored function.
func identifiersToJSON(identifiers []shared.TagIdentifierRequest) ([]byte, error) {
	if len(identifiers) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(identifiers)
}
```

---

## Step 7: Add Asset Storage Functions for Transactional Creation

**File**: `backend/internal/storage/assets.go`

Add these functions:

```go
import (
	// Add to existing imports:
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

// CreateAssetWithIdentifiers atomically creates an asset with its tag identifiers
// using a PostgreSQL function. All-or-nothing operation.
func (s *Storage) CreateAssetWithIdentifiers(ctx context.Context, request asset.CreateAssetWithIdentifiersRequest) (*asset.AssetView, error) {
	// Convert identifiers to JSON for the function
	identifiersJSON, err := identifiersToJSON(request.Identifiers)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize identifiers: %w", err)
	}

	// Call PostgreSQL function
	query := `SELECT * FROM trakrf.create_asset_with_identifiers($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	var assetID int
	var identifierIDs []int

	err = s.pool.QueryRow(ctx, query,
		request.OrgID,
		request.Identifier,
		request.Name,
		request.Type,
		request.Description,
		request.CurrentLocationID,
		request.ValidFrom,
		request.ValidTo,
		request.IsActive,
		request.Metadata,
		identifiersJSON,
	).Scan(&assetID, &identifierIDs)

	if err != nil {
		return nil, parseAssetWithIdentifiersError(err, request.Identifier)
	}

	// Fetch the complete asset view
	return s.GetAssetViewByID(ctx, assetID)
}

// GetAssetViewByID returns an asset with its embedded identifiers.
func (s *Storage) GetAssetViewByID(ctx context.Context, id int) (*asset.AssetView, error) {
	// Get base asset
	baseAsset, err := s.GetAssetByID(ctx, &id)
	if err != nil {
		return nil, err
	}
	if baseAsset == nil {
		return nil, nil
	}

	// Get identifiers
	identifiers, err := s.GetIdentifiersByAssetID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &asset.AssetView{
		Asset:       *baseAsset,
		Identifiers: identifiers,
	}, nil
}

// ListAssetViews returns paginated assets with their identifiers.
func (s *Storage) ListAssetViews(ctx context.Context, orgID, limit, offset int) ([]asset.AssetView, error) {
	// Get base assets
	assets, err := s.ListAllAssets(ctx, orgID, limit, offset)
	if err != nil {
		return nil, err
	}

	if len(assets) == 0 {
		return []asset.AssetView{}, nil
	}

	// Collect asset IDs for batch identifier fetch
	assetIDs := make([]int, len(assets))
	for i, a := range assets {
		assetIDs[i] = a.ID
	}

	// Batch fetch all identifiers
	identifierMap, err := s.getIdentifiersForAssets(ctx, assetIDs)
	if err != nil {
		return nil, err
	}

	// Build AssetViews
	views := make([]asset.AssetView, len(assets))
	for i, a := range assets {
		views[i] = asset.AssetView{
			Asset:       a,
			Identifiers: identifierMap[a.ID],
		}
	}

	return views, nil
}

// getIdentifiersForAssets batch fetches identifiers for multiple assets.
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
	// Initialize with empty slices for all asset IDs
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

// parseAssetWithIdentifiersError converts PostgreSQL errors to user-friendly messages.
func parseAssetWithIdentifiersError(err error, identifier string) error {
	errStr := err.Error()

	// Check for duplicate asset identifier
	if strings.Contains(errStr, "assets_org_id_identifier") ||
	   (strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "assets")) {
		return fmt.Errorf("asset with identifier %s already exists", identifier)
	}

	// Check for duplicate tag identifier
	if strings.Contains(errStr, "identifiers_org_id_type_value") ||
	   (strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "identifiers")) {
		return fmt.Errorf("one or more tag identifiers already exist")
	}

	return fmt.Errorf("failed to create asset with identifiers: %w", err)
}
```

---

## Step 8: Run Migrations and Verify

```bash
# From project root
just backend db-up

# Verify functions exist
just backend db-shell
\df trakrf.create_asset_with_identifiers
\df trakrf.create_location_with_identifiers
```

---

## Step 9: Write Unit Tests for Storage Layer

**File**: `backend/internal/storage/identifiers_test.go` (NEW)

```go
package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

func TestCreateAssetWithIdentifiers(t *testing.T) {
	s := setupTestStorage(t)
	ctx := context.Background()
	orgID := createTestOrg(t, s)

	t.Run("creates asset with identifiers atomically", func(t *testing.T) {
		req := asset.CreateAssetWithIdentifiersRequest{
			CreateAssetRequest: asset.CreateAssetRequest{
				OrgID:      orgID,
				Identifier: "TEST-001",
				Name:       "Test Asset",
				Type:       "asset",
				IsActive:   true,
			},
			Identifiers: []shared.TagIdentifierRequest{
				{Type: "rfid", Value: "E20000001234"},
				{Type: "ble", Value: "AA:BB:CC:DD:EE:FF"},
			},
		}

		result, err := s.CreateAssetWithIdentifiers(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "TEST-001", result.Identifier)
		assert.Len(t, result.Identifiers, 2)
		assert.Equal(t, "rfid", result.Identifiers[0].Type)
		assert.Equal(t, "ble", result.Identifiers[1].Type)
	})

	t.Run("rolls back on duplicate identifier", func(t *testing.T) {
		// First create succeeds
		req1 := asset.CreateAssetWithIdentifiersRequest{
			CreateAssetRequest: asset.CreateAssetRequest{
				OrgID:      orgID,
				Identifier: "TEST-002",
				Name:       "Asset 2",
				Type:       "asset",
				IsActive:   true,
			},
			Identifiers: []shared.TagIdentifierRequest{
				{Type: "rfid", Value: "E20000005678"},
			},
		}
		_, err := s.CreateAssetWithIdentifiers(ctx, req1)
		require.NoError(t, err)

		// Second with same tag value fails
		req2 := asset.CreateAssetWithIdentifiersRequest{
			CreateAssetRequest: asset.CreateAssetRequest{
				OrgID:      orgID,
				Identifier: "TEST-003",
				Name:       "Asset 3",
				Type:       "asset",
				IsActive:   true,
			},
			Identifiers: []shared.TagIdentifierRequest{
				{Type: "rfid", Value: "E20000005678"}, // duplicate
			},
		}
		result, err := s.CreateAssetWithIdentifiers(ctx, req2)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "already exist")

		// Verify asset was NOT created (rollback worked)
		assets, _ := s.ListAllAssets(ctx, orgID, 100, 0)
		for _, a := range assets {
			assert.NotEqual(t, "TEST-003", a.Identifier)
		}
	})

	t.Run("creates asset without identifiers", func(t *testing.T) {
		req := asset.CreateAssetWithIdentifiersRequest{
			CreateAssetRequest: asset.CreateAssetRequest{
				OrgID:      orgID,
				Identifier: "TEST-004",
				Name:       "No Tags Asset",
				Type:       "asset",
				IsActive:   true,
			},
			Identifiers: []shared.TagIdentifierRequest{}, // empty
		}

		result, err := s.CreateAssetWithIdentifiers(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Identifiers, 0)
	})
}

func TestGetAssetViewByID(t *testing.T) {
	s := setupTestStorage(t)
	ctx := context.Background()
	orgID := createTestOrg(t, s)

	// Create asset with identifiers
	req := asset.CreateAssetWithIdentifiersRequest{
		CreateAssetRequest: asset.CreateAssetRequest{
			OrgID:      orgID,
			Identifier: "VIEW-001",
			Name:       "View Test",
			Type:       "asset",
			IsActive:   true,
		},
		Identifiers: []shared.TagIdentifierRequest{
			{Type: "rfid", Value: "VIEW-RFID-001"},
		},
	}
	created, err := s.CreateAssetWithIdentifiers(ctx, req)
	require.NoError(t, err)

	t.Run("returns asset with identifiers", func(t *testing.T) {
		view, err := s.GetAssetViewByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, "VIEW-001", view.Identifier)
		assert.Len(t, view.Identifiers, 1)
		assert.Equal(t, "VIEW-RFID-001", view.Identifiers[0].Value)
	})

	t.Run("returns nil for non-existent asset", func(t *testing.T) {
		view, err := s.GetAssetViewByID(ctx, 999999)
		require.NoError(t, err)
		assert.Nil(t, view)
	})
}

func TestAddIdentifierToAsset(t *testing.T) {
	s := setupTestStorage(t)
	ctx := context.Background()
	orgID := createTestOrg(t, s)

	// Create asset without identifiers
	a, err := s.CreateAsset(ctx, asset.Asset{
		OrgID:      orgID,
		Identifier: "ADD-ID-001",
		Name:       "Add ID Test",
		Type:       "asset",
		IsActive:   true,
	})
	require.NoError(t, err)

	t.Run("adds identifier to existing asset", func(t *testing.T) {
		req := shared.TagIdentifierRequest{Type: "barcode", Value: "BC-12345"}
		id, err := s.AddIdentifierToAsset(ctx, orgID, a.ID, req)
		require.NoError(t, err)
		assert.Equal(t, "barcode", id.Type)
		assert.Equal(t, "BC-12345", id.Value)
		assert.True(t, id.IsActive)
	})

	t.Run("fails on duplicate identifier", func(t *testing.T) {
		req := shared.TagIdentifierRequest{Type: "barcode", Value: "BC-12345"}
		_, err := s.AddIdentifierToAsset(ctx, orgID, a.ID, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}
```

---

## Verification Checklist

After implementation, verify:

- [ ] Migration runs successfully (`just backend db-up`)
- [ ] Functions visible in database (`\df trakrf.create_*`)
- [ ] `go build ./...` compiles without errors
- [ ] `go test ./internal/storage/...` passes
- [ ] Manual test: Create asset with identifiers via function
- [ ] Manual test: Duplicate tag value causes full rollback

---

## Appendix A: Remaining Phases (Roadmap)

### Phase 3: Location Storage Layer
- Add `CreateLocationWithIdentifiers` (calls PostgreSQL function)
- Add `GetLocationViewByID`, `ListLocationViews`
- Add `getIdentifiersForLocations` batch fetch
- Unit tests for location storage

### Phase 4: API Handlers
- Update `handlers/assets/assets.go`:
  - `Create` calls `CreateAssetWithIdentifiers` when identifiers present
  - `GetAsset` returns `AssetView`
  - `ListAssets` returns `[]AssetView`
- Add `POST /api/v1/assets/{id}/identifiers`
- Add `DELETE /api/v1/assets/{id}/identifiers/{identifierId}`
- Repeat for locations
- Add `handlers/lookup/lookup.go` with `GET /api/v1/lookup/tag`

### Phase 5: Integration Tests
- API tests for transactional creation
- API tests for add/remove identifier
- Rollback verification tests
- Lookup endpoint tests
- Performance tests (< 100ms for asset + 3 identifiers)
