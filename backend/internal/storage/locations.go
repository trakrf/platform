package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

func (s *Storage) CreateLocation(ctx context.Context, request location.Location) (*location.Location, error) {
	query := `
	INSERT INTO trakrf.locations
	(name, identifier, parent_location_id, description, valid_from, valid_to, is_active, org_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id, org_id, name, identifier, parent_location_id, path, depth,
	          description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
	`
	var loc location.Location
	err := s.pool.QueryRow(ctx, query, request.Name, request.Identifier, request.ParentLocationID,
		request.Description, request.ValidFrom, request.ValidTo, request.IsActive, request.OrgID,
	).Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier, &loc.ParentLocationID,
		&loc.Path, &loc.Depth, &loc.Description, &loc.ValidFrom, &loc.ValidTo,
		&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
	)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("location with identifier %s already exists", request.Identifier)
		}
		if strings.Contains(err.Error(), "foreign key constraint") {
			return nil, fmt.Errorf("invalid parent location ID or organization ID")
		}
		return nil, fmt.Errorf("failed to create location: %w", err)
	}

	return &loc, nil
}

func (s *Storage) UpdateLocation(ctx context.Context, id int, request location.UpdateLocationRequest) (*location.Location, error) {
	updates := []string{}
	args := []any{id}
	argPos := 2
	fields, err := mapLocationReqToFields(request)

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
		UPDATE trakrf.locations
		SET %s, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, org_id, name, identifier, parent_location_id, path, depth,
		          description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
	`, strings.Join(updates, ", "))

	var loc location.Location
	err = s.pool.QueryRow(ctx, query, args...).Scan(&loc.ID, &loc.OrgID, &loc.Name,
		&loc.Identifier, &loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
		&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "foreign key constraint") {
			return nil, fmt.Errorf("invalid parent location ID")
		}
		return nil, fmt.Errorf("failed to update location: %w", err)
	}

	return &loc, nil
}

func (s *Storage) GetLocationByID(ctx context.Context, id int) (*location.Location, error) {
	query := `
	SELECT id, org_id, name, identifier, parent_location_id, path, depth,
	       description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
	FROM trakrf.locations
	WHERE id = $1 AND deleted_at IS NULL
	`
	var loc location.Location
	err := s.pool.QueryRow(ctx, query, id).Scan(&loc.ID, &loc.OrgID, &loc.Name,
		&loc.Identifier, &loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
		&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get location by id: %w", err)
	}
	return &loc, nil
}

func (s *Storage) GetLocationWithRelations(ctx context.Context, id int) (*location.Location, error) {
	query := `
	WITH target AS (
		SELECT id, org_id, name, identifier, parent_location_id, path, depth,
		       description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at,
		       'target' as relation_type
		FROM trakrf.locations
		WHERE id = $1 AND deleted_at IS NULL
	)
	SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
	       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
	       CASE
	           WHEN l.id = $1 THEN 'target'
	           WHEN l.path @> (SELECT path FROM target) AND l.id != $1 THEN 'ancestor'
	           WHEN l.parent_location_id = $1 THEN 'child'
	           ELSE 'other'
	       END as relation_type
	FROM trakrf.locations l, target t
	WHERE l.deleted_at IS NULL
	  AND (
	      l.id = $1
	      OR l.path @> t.path
	      OR l.parent_location_id = $1
	  )
	ORDER BY
	    CASE
	        WHEN l.id = $1 THEN 0
	        WHEN l.path @> t.path THEN 1
	        ELSE 2
	    END,
	    l.depth
	`

	rows, err := s.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get location with relations: %w", err)
	}
	defer rows.Close()

	var target *location.Location
	var ancestors []location.Location
	var children []location.Location

	for rows.Next() {
		var loc location.Location
		var relationType string

		err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
			&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
			&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt,
			&loc.UpdatedAt, &loc.DeletedAt, &relationType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan location with relations: %w", err)
		}

		switch relationType {
		case "target":
			target = &loc
		case "ancestor":
			ancestors = append(ancestors, loc)
		case "child":
			children = append(children, loc)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating location relations: %w", err)
	}

	if target == nil {
		return nil, nil
	}

	// Populate relationships
	target.Ancestors = ancestors
	target.Children = children

	return target, nil
}

func (s *Storage) ListAllLocations(ctx context.Context, orgID int, limit int, offset int) ([]location.Location, error) {
	query := `
		SELECT id, org_id, name, identifier, parent_location_id, path, depth,
		       description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
		FROM trakrf.locations
		WHERE org_id = $1 AND deleted_at IS NULL
		ORDER BY path
		LIMIT $2 OFFSET $3
	`
	rows, err := s.pool.Query(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list locations: %w", err)
	}
	defer rows.Close()

	var locations []location.Location
	for rows.Next() {
		var loc location.Location
		err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
			&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
			&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt,
			&loc.UpdatedAt, &loc.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan location: %w", err)
		}
		locations = append(locations, loc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating locations: %w", err)
	}

	return locations, nil
}

// CountAllLocations returns the total count of non-deleted locations for a specific org
func (s *Storage) CountAllLocations(ctx context.Context, orgID int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM trakrf.locations
		WHERE org_id = $1 AND deleted_at IS NULL
	`

	var count int
	err := s.pool.QueryRow(ctx, query, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count locations: %w", err)
	}

	return count, nil
}

func (s *Storage) DeleteLocation(ctx context.Context, id int) (bool, error) {
	query := `UPDATE trakrf.locations SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("could not delete location: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

// GetAncestors returns all ancestor locations of a given location (from root to parent)
// Uses ltree @> operator: ancestor_path @> child_path
func (s *Storage) GetAncestors(ctx context.Context, id int) ([]location.Location, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at
		FROM trakrf.locations l
		WHERE l.path @> (SELECT path FROM trakrf.locations WHERE id = $1 AND deleted_at IS NULL)
		  AND l.id != $1
		  AND l.deleted_at IS NULL
		ORDER BY l.depth
	`
	rows, err := s.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get ancestors: %w", err)
	}
	defer rows.Close()

	var locations []location.Location
	for rows.Next() {
		var loc location.Location
		err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
			&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
			&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt,
			&loc.UpdatedAt, &loc.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ancestor: %w", err)
		}
		locations = append(locations, loc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ancestors: %w", err)
	}

	return locations, nil
}

// GetDescendants returns all descendant locations of a given location (children at all levels)
// Uses ltree <@ operator: child_path <@ parent_path
func (s *Storage) GetDescendants(ctx context.Context, id int) ([]location.Location, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at
		FROM trakrf.locations l
		WHERE l.path <@ (SELECT path FROM trakrf.locations WHERE id = $1 AND deleted_at IS NULL)
		  AND l.id != $1
		  AND l.deleted_at IS NULL
		ORDER BY l.path
	`
	rows, err := s.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get descendants: %w", err)
	}
	defer rows.Close()

	var locations []location.Location
	for rows.Next() {
		var loc location.Location
		err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
			&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
			&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt,
			&loc.UpdatedAt, &loc.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan descendant: %w", err)
		}
		locations = append(locations, loc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating descendants: %w", err)
	}

	return locations, nil
}

// GetChildren returns immediate children of a given location (depth = parent_depth + 1)
func (s *Storage) GetChildren(ctx context.Context, id int) ([]location.Location, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at
		FROM trakrf.locations l
		WHERE l.parent_location_id = $1 AND l.deleted_at IS NULL
		ORDER BY l.name
	`
	rows, err := s.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get children: %w", err)
	}
	defer rows.Close()

	var locations []location.Location
	for rows.Next() {
		var loc location.Location
		err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
			&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
			&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt,
			&loc.UpdatedAt, &loc.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan child: %w", err)
		}
		locations = append(locations, loc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating children: %w", err)
	}

	return locations, nil
}

// CreateLocationWithIdentifiers creates a location with tag identifiers in a single transaction
func (s *Storage) CreateLocationWithIdentifiers(ctx context.Context, orgID int, request location.CreateLocationWithIdentifiersRequest) (*location.LocationView, error) {
	identifiersJSON, err := identifiersToJSON(request.Identifiers)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize identifiers: %w", err)
	}

	// Convert FlexibleDate to time.Time
	validFrom := request.ValidFrom.ToTime()
	var validTo *time.Time
	if request.ValidTo != nil {
		t := request.ValidTo.ToTime()
		validTo = &t
	}

	query := `SELECT * FROM trakrf.create_location_with_identifiers($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	var locationID int
	var identifierIDs []int

	err = s.pool.QueryRow(ctx, query,
		orgID,
		request.Identifier,
		request.Name,
		request.Description,
		request.ParentLocationID,
		validFrom,
		validTo,
		request.IsActive,
		nil, // metadata - not used in CreateLocationRequest
		identifiersJSON,
	).Scan(&locationID, &identifierIDs)

	if err != nil {
		return nil, parseLocationWithIdentifiersError(err, request.Identifier)
	}

	return s.GetLocationViewByID(ctx, locationID)
}

// GetLocationViewByID fetches a location with its tag identifiers
func (s *Storage) GetLocationViewByID(ctx context.Context, id int) (*location.LocationView, error) {
	baseLoc, err := s.GetLocationByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if baseLoc == nil {
		return nil, nil
	}

	identifiers, err := s.GetIdentifiersByLocationID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &location.LocationView{
		Location:    *baseLoc,
		Identifiers: identifiers,
	}, nil
}

// ListLocationViews fetches locations with their tag identifiers for an org
func (s *Storage) ListLocationViews(ctx context.Context, orgID, limit, offset int) ([]location.LocationView, error) {
	locations, err := s.ListAllLocations(ctx, orgID, limit, offset)
	if err != nil {
		return nil, err
	}

	if len(locations) == 0 {
		return []location.LocationView{}, nil
	}

	locationIDs := make([]int, len(locations))
	for i, loc := range locations {
		locationIDs[i] = loc.ID
	}

	identifierMap, err := s.getIdentifiersForLocations(ctx, locationIDs)
	if err != nil {
		return nil, err
	}

	views := make([]location.LocationView, len(locations))
	for i, loc := range locations {
		ids := identifierMap[loc.ID]
		if ids == nil {
			ids = []shared.TagIdentifier{}
		}
		views[i] = location.LocationView{
			Location:    loc,
			Identifiers: ids,
		}
	}

	return views, nil
}

func parseLocationWithIdentifiersError(err error, identifier string) error {
	errStr := err.Error()

	if strings.Contains(errStr, "locations_org_id_identifier") ||
		(strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "locations")) {
		return fmt.Errorf("location with identifier %s already exists", identifier)
	}

	if strings.Contains(errStr, "identifiers_org_id_type_value") ||
		(strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "identifiers")) {
		return fmt.Errorf("one or more tag identifiers already exist")
	}

	return fmt.Errorf("failed to create location with identifiers: %w", err)
}

func mapLocationReqToFields(req location.UpdateLocationRequest) (map[string]any, error) {
	fields := make(map[string]any)

	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Identifier != nil {
		fields["identifier"] = *req.Identifier
	}
	if req.ParentLocationID != nil {
		fields["parent_location_id"] = *req.ParentLocationID
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.ValidFrom != nil {
		t := req.ValidFrom.ToTime()
		fields["valid_from"] = t
	}
	if req.ValidTo != nil {
		t := req.ValidTo.ToTime()
		fields["valid_to"] = t
	}
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}

	return fields, nil
}
