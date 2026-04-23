package storage

import (
	"context"
	stderrors "errors"
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
	err := s.WithOrgTx(ctx, request.OrgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, request.Name, request.Identifier, request.ParentLocationID,
			request.Description, request.ValidFrom, request.ValidTo, request.IsActive, request.OrgID,
		).Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier, &loc.ParentLocationID,
			&loc.Path, &loc.Depth, &loc.Description, &loc.ValidFrom, &loc.ValidTo,
			&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
		)
	})

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("location with identifier %s already exists", request.Identifier)
		}
		if strings.Contains(err.Error(), "parent_location_id_fkey") {
			return nil, fmt.Errorf("invalid parent_location_id: parent location does not exist")
		}
		return nil, fmt.Errorf("failed to create location: %w", err)
	}

	return &loc, nil
}

func (s *Storage) UpdateLocation(ctx context.Context, orgID, id int, request location.UpdateLocationRequest) (*location.LocationWithParent, error) {
	updates := []string{}
	args := []any{id, orgID}
	argPos := 3
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
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		RETURNING id
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
			identifier := "unknown"
			if request.Identifier != nil {
				identifier = *request.Identifier
			}
			return nil, fmt.Errorf("location with identifier %s already exists", identifier)
		}
		if strings.Contains(err.Error(), "parent_location_id_fkey") {
			return nil, fmt.Errorf("invalid parent_location_id: parent location does not exist")
		}
		return nil, fmt.Errorf("failed to update location: %w", err)
	}

	return s.getLocationWithParentByID(ctx, orgID, updatedID)
}

func (s *Storage) GetLocationByID(ctx context.Context, orgID, id int) (*location.Location, error) {
	query := `
	SELECT id, org_id, name, identifier, parent_location_id, path, depth,
	       description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
	FROM trakrf.locations
	WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
	`
	var loc location.Location
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, orgID).Scan(&loc.ID, &loc.OrgID, &loc.Name,
			&loc.Identifier, &loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
			&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
		)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get location by id: %w", err)
	}
	return &loc, nil
}

// GetLocationsByIDs fetches multiple locations by their IDs (batch fetch),
// scoped to the caller's organization. The org_id fence is required because
// identifiers.location_id is a plain FK that does not enforce same-org — see
// TRA-431 for the cross-tenant leak this prevents.
func (s *Storage) GetLocationsByIDs(ctx context.Context, orgID int, ids []int) ([]*location.Location, error) {
	if len(ids) == 0 {
		return []*location.Location{}, nil
	}

	query := `
	SELECT id, org_id, name, identifier, parent_location_id, path, depth,
	       description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
	FROM trakrf.locations
	WHERE org_id = $1 AND id = ANY($2) AND deleted_at IS NULL
	`

	locations := []*location.Location{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, ids)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var loc location.Location
			if err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name,
				&loc.Identifier, &loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
				&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
			); err != nil {
				return fmt.Errorf("failed to scan location: %w", err)
			}
			locations = append(locations, &loc)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to batch fetch locations: %w", err)
	}

	return locations, nil
}

func (s *Storage) GetLocationWithRelations(ctx context.Context, orgID, id int) (*location.Location, error) {
	query := `
	WITH target AS (
		SELECT id, org_id, name, identifier, parent_location_id, path, depth,
		       description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at,
		       'target' as relation_type
		FROM trakrf.locations
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
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
	WHERE l.org_id = $2 AND l.deleted_at IS NULL
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

	var target *location.Location
	ancestors := []location.Location{}
	children := []location.Location{}

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, id, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var loc location.Location
			var relationType string

			if err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
				&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
				&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt,
				&loc.UpdatedAt, &loc.DeletedAt, &relationType,
			); err != nil {
				return fmt.Errorf("failed to scan location with relations: %w", err)
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
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get location with relations: %w", err)
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
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	locations := []location.Location{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var loc location.Location
			if err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
				&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
				&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt,
				&loc.UpdatedAt, &loc.DeletedAt,
			); err != nil {
				return fmt.Errorf("failed to scan location: %w", err)
			}
			locations = append(locations, loc)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list locations: %w", err)
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
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count locations: %w", err)
	}

	return count, nil
}

func (s *Storage) DeleteLocation(ctx context.Context, orgID, id int) (bool, error) {
	query := `UPDATE trakrf.locations SET deleted_at = NOW() WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`
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
		return false, fmt.Errorf("could not delete location: %w", err)
	}
	return rowsAffected > 0, nil
}

// GetAncestors returns all ancestor locations of a given location (from root to parent),
// projected through LocationWithParent so every non-root carries its parent's natural
// key and its tag identifiers — same shape as GET /locations/{identifier}.
// Uses ltree @> operator: ancestor_path @> child_path.
// Both the outer query and the path subselect are scoped to orgID so cross-tenant paths
// that happen to share an identifier (e.g. two orgs both using "whs-01") stay isolated.
func (s *Storage) GetAncestors(ctx context.Context, orgID, id int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.path @> (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND l.id != $2
		  AND l.deleted_at IS NULL
		ORDER BY l.depth
	`
	return s.scanHierarchyRows(ctx, query, "ancestor", orgID, orgID, id)
}

// GetDescendants returns all descendant locations of a given location (children at all levels),
// projected through LocationWithParent so every entry carries its parent's natural key
// and its tag identifiers — same shape as GET /locations/{identifier}.
// Uses ltree <@ operator: child_path <@ parent_path.
// Both the outer query and the path subselect are scoped to orgID: ltree paths are derived
// from identifier segments alone (see migration 000018), so without this fence two tenants
// with identical identifier hierarchies would see each other's subtrees.
func (s *Storage) GetDescendants(ctx context.Context, orgID, id int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.path <@ (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND l.id != $2
		  AND l.deleted_at IS NULL
		ORDER BY l.path
	`
	return s.scanHierarchyRows(ctx, query, "descendant", orgID, orgID, id)
}

// GetChildren returns immediate children of a given location (depth = parent_depth + 1),
// projected through LocationWithParent for parent-identifier and tag-identifier parity
// with GET /locations/{identifier}.
// parent_location_id references a globally unique PK so the query alone is not cross-tenant
// reachable, but the orgID filter keeps the invariant explicit (defense in depth) and in
// line with GetAncestors/GetDescendants.
func (s *Storage) GetChildren(ctx context.Context, orgID, id int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.parent_location_id = $2
		  AND l.deleted_at IS NULL
		ORDER BY l.name
	`
	return s.scanHierarchyRows(ctx, query, "child", orgID, orgID, id)
}

// scanHierarchyRows runs a hierarchy query whose projection ends in p.identifier
// (LEFT JOIN parent) and then bulk-fetches tag identifiers for the returned locations.
// kind ("ancestor"/"descendant"/"child") is interpolated into error messages.
func (s *Storage) scanHierarchyRows(
	ctx context.Context, query, kind string, orgID int, args ...any,
) ([]location.LocationWithParent, error) {
	out := []location.LocationWithParent{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var (
				loc    location.Location
				parIdt *string
			)
			if err := rows.Scan(
				&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
				&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
				&loc.ValidFrom, &loc.ValidTo, &loc.IsActive,
				&loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
				&parIdt,
			); err != nil {
				return fmt.Errorf("failed to scan %s: %w", kind, err)
			}
			out = append(out, location.LocationWithParent{
				LocationView:     location.LocationView{Location: loc},
				ParentIdentifier: parIdt,
			})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get %ss: %w", kind, err)
	}

	if len(out) > 0 {
		ids := make([]int, len(out))
		for i, l := range out {
			ids[i] = l.ID
		}
		idMap, err := s.getIdentifiersForLocations(ctx, orgID, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Identifiers = idMap[out[i].ID]
			if out[i].Identifiers == nil {
				out[i].Identifiers = []shared.TagIdentifier{}
			}
		}
	}

	return out, nil
}

// CreateLocationWithIdentifiers creates a location with tag identifiers in a single transaction
func (s *Storage) CreateLocationWithIdentifiers(ctx context.Context, orgID int, request location.CreateLocationWithIdentifiersRequest) (*location.LocationWithParent, error) {
	identifiersJSON, err := identifiersToJSON(request.Identifiers)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize identifiers: %w", err)
	}

	// Handler normally applies defaults; storage re-applies as a safety net.
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

	query := `SELECT * FROM trakrf.create_location_with_identifiers($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	var locationID int
	var identifierIDs []int

	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			orgID,
			request.Identifier,
			request.Name,
			request.Description,
			request.ParentLocationID,
			validFrom,
			validTo,
			isActive,
			nil, // metadata - not used in CreateLocationRequest
			identifiersJSON,
		).Scan(&locationID, &identifierIDs)
	})

	if err != nil {
		return nil, parseLocationWithIdentifiersError(err, request.Identifier)
	}

	return s.getLocationWithParentByID(ctx, orgID, locationID)
}

// GetLocationViewByID fetches a location with its tag identifiers
func (s *Storage) GetLocationViewByID(ctx context.Context, orgID, id int) (*location.LocationView, error) {
	baseLoc, err := s.GetLocationByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if baseLoc == nil {
		return nil, nil
	}

	identifiers, err := s.GetIdentifiersByLocationID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	return &location.LocationView{
		Location:    *baseLoc,
		Identifiers: identifiers,
	}, nil
}

// getLocationWithParentByID returns a LocationWithParent by surrogate id,
// performing the self-join on parent location and fetching identifiers.
// Used by CreateLocationWithIdentifiers and UpdateLocation to emit the
// public write-response shape. Returns (nil, nil) if the location doesn't
// exist or is soft-deleted.
func (s *Storage) getLocationWithParentByID(ctx context.Context, orgID, id int) (*location.LocationWithParent, error) {
	query := `
		SELECT
			l.id, l.org_id, l.name, l.identifier, l.parent_location_id,
			l.path, l.depth, l.description, l.valid_from, l.valid_to,
			l.is_active, l.created_at, l.updated_at, l.deleted_at,
			p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.id = $1 AND l.org_id = $2 AND l.deleted_at IS NULL
		LIMIT 1
	`
	var (
		loc    location.Location
		parIdt *string
	)
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, orgID).Scan(
			&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier, &loc.ParentLocationID,
			&loc.Path, &loc.Depth, &loc.Description, &loc.ValidFrom, &loc.ValidTo,
			&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
			&parIdt,
		)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get location with parent by id: %w", err)
	}

	identifiers, err := s.GetIdentifiersByLocationID(ctx, orgID, loc.ID)
	if err != nil {
		return nil, err
	}

	return &location.LocationWithParent{
		LocationView: location.LocationView{
			Location:    loc,
			Identifiers: identifiers,
		},
		ParentIdentifier: parIdt,
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

	identifierMap, err := s.getIdentifiersForLocations(ctx, orgID, locationIDs)
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

	if strings.Contains(errStr, "parent_location_id_fkey") {
		return fmt.Errorf("invalid parent_location_id: parent location does not exist")
	}

	return fmt.Errorf("failed to create location with identifiers: %w", err)
}

// GetLocationByIdentifier returns the live location with the given natural key
// for the given org, plus the parent location's natural key. Returns (nil, nil)
// if no match.
func (s *Storage) GetLocationByIdentifier(
	ctx context.Context, orgID int, identifier string,
) (*location.LocationWithParent, error) {
	query := `
		SELECT
			l.id, l.org_id, l.name, l.identifier, l.parent_location_id,
			l.path, l.depth, l.description, l.valid_from, l.valid_to,
			l.is_active, l.created_at, l.updated_at, l.deleted_at,
			p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1 AND l.identifier = $2 AND l.deleted_at IS NULL
		LIMIT 1
	`
	var (
		loc    location.Location
		parIdt *string
	)
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, identifier).Scan(
			&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier, &loc.ParentLocationID,
			&loc.Path, &loc.Depth, &loc.Description, &loc.ValidFrom, &loc.ValidTo,
			&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
			&parIdt,
		)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get location by identifier: %w", err)
	}

	identifiers, err := s.GetIdentifiersByLocationID(ctx, orgID, loc.ID)
	if err != nil {
		return nil, err
	}

	return &location.LocationWithParent{
		LocationView: location.LocationView{
			Location:    loc,
			Identifiers: identifiers,
		},
		ParentIdentifier: parIdt,
	}, nil
}

// ListLocationsFiltered returns locations matching the filter with parent's
// natural key resolved via self-join.
func (s *Storage) ListLocationsFiltered(
	ctx context.Context, orgID int, f location.ListFilter,
) ([]location.LocationWithParent, error) {
	where, args := buildLocationsWhere(orgID, f)
	orderBy := buildLocationsOrderBy(f.Sorts)

	query := fmt.Sprintf(`
		SELECT
			l.id, l.org_id, l.name, l.identifier,
			l.parent_location_id, l.path, l.depth, l.description,
			l.valid_from, l.valid_to, l.is_active,
			l.created_at, l.updated_at, l.deleted_at,
			p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, where, orderBy, len(args)+1, len(args)+2)

	args = append(args, clampLocListLimit(f.Limit), f.Offset)

	out := []location.LocationWithParent{}
	if err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var (
				loc    location.Location
				parIdt *string
			)
			if err := rows.Scan(
				&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier,
				&loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
				&loc.ValidFrom, &loc.ValidTo, &loc.IsActive,
				&loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
				&parIdt,
			); err != nil {
				return fmt.Errorf("scan location: %w", err)
			}
			out = append(out, location.LocationWithParent{
				LocationView:     location.LocationView{Location: loc},
				ParentIdentifier: parIdt,
			})
		}
		return rows.Err()
	}); err != nil {
		return nil, fmt.Errorf("list locations filtered: %w", err)
	}

	// Bulk-fetch identifiers for the returned locations, matching the
	// assets-list pattern so the public list endpoint returns `[]` rather
	// than `null` for locations without tag identifiers.
	if len(out) > 0 {
		ids := make([]int, len(out))
		for i, l := range out {
			ids[i] = l.ID
		}
		idMap, err := s.getIdentifiersForLocations(ctx, orgID, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Identifiers = idMap[out[i].ID]
			if out[i].Identifiers == nil {
				out[i].Identifiers = []shared.TagIdentifier{}
			}
		}
	}

	return out, nil
}

// CountLocationsFiltered returns total count matching the filter.
func (s *Storage) CountLocationsFiltered(
	ctx context.Context, orgID int, f location.ListFilter,
) (int, error) {
	where, args := buildLocationsWhere(orgID, f)
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE %s
	`, where)

	var n int
	if err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&n)
	}); err != nil {
		return 0, fmt.Errorf("count locations filtered: %w", err)
	}
	return n, nil
}

func buildLocationsWhere(orgID int, f location.ListFilter) (string, []any) {
	clauses := []string{"l.org_id = $1", "l.deleted_at IS NULL"}
	args := []any{orgID}

	if len(f.ParentIdentifiers) > 0 {
		args = append(args, f.ParentIdentifiers)
		clauses = append(clauses, fmt.Sprintf("p.identifier = ANY($%d::text[])", len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		clauses = append(clauses, fmt.Sprintf("l.is_active = $%d", len(args)))
	}
	if f.Q != nil {
		args = append(args, "%"+*f.Q+"%")
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(l.name ILIKE $%d OR l.identifier ILIKE $%d OR l.description ILIKE $%d "+
				"OR EXISTS (SELECT 1 FROM trakrf.identifiers i "+
				"WHERE i.location_id = l.id AND i.is_active = true "+
				"AND i.deleted_at IS NULL AND i.value ILIKE $%d))",
			idx, idx, idx, idx))
	}
	return strings.Join(clauses, " AND "), args
}

func buildLocationsOrderBy(sorts []location.ListSort) string {
	if len(sorts) == 0 {
		return "l.path ASC"
	}
	out := make([]string, 0, len(sorts))
	for _, s := range sorts {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		out = append(out, "l."+s.Field+" "+dir)
	}
	return strings.Join(out, ", ")
}

func clampLocListLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
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

// GetLocationWithParentByIDForTest exposes getLocationWithParentByID to
// integration tests in the same package. Production code must use
// GetLocationByIdentifier or the CreateLocationWithIdentifiers /
// UpdateLocation return values.
func (s *Storage) GetLocationWithParentByIDForTest(ctx context.Context, orgID, id int) (*location.LocationWithParent, error) {
	return s.getLocationWithParentByID(ctx, orgID, id)
}
