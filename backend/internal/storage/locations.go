package storage

import (
	"context"
	"database/sql"
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
	(name, external_key, parent_location_id, description, valid_from, valid_to, is_active, org_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id, org_id, name, external_key, parent_location_id, path, depth,
	          description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
	`
	var loc location.Location
	err := s.WithOrgTx(ctx, request.OrgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, request.Name, request.ExternalKey, request.ParentID,
			request.Description, request.ValidFrom, request.ValidTo, request.IsActive, request.OrgID,
		).Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.ExternalKey, &loc.ParentID,
			&loc.TreePath, &loc.Depth, &loc.Description, &loc.ValidFrom, &loc.ValidTo,
			&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
		)
	})

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("location with external_key %s already exists", request.ExternalKey)
		}
		if strings.Contains(err.Error(), "parent_location_id_fkey") {
			return nil, fmt.Errorf("invalid parent_location_id: parent location does not exist")
		}
		return nil, fmt.Errorf("failed to create location: %w", err)
	}

	return &loc, nil
}

// GetNextLocationSequence derives the next sequence number for auto-generated
// location external_keys. Queries the max sequence from existing LOC-NNNN
// external_keys for the org. Returns 1 if no LOC-NNNN external_keys exist.
// Parallels GetNextAssetSequence. TRA-665 / BB26 D3.
func (s *Storage) GetNextLocationSequence(ctx context.Context, orgID int) (int, error) {
	var maxSeq sql.NullInt64
	query := `
		SELECT MAX(CAST(SUBSTRING(external_key FROM 'LOC-([0-9]+)') AS INT))
		FROM trakrf.locations
		WHERE org_id = $1
		  AND external_key ~ '^LOC-[0-9]+$'
		  AND deleted_at IS NULL
	`
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID).Scan(&maxSeq)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get max sequence: %w", err)
	}
	if !maxSeq.Valid {
		return 1, nil
	}
	return int(maxSeq.Int64) + 1, nil
}

// GenerateLocationExternalKey creates an external_key in format LOC-NNNN.
// Zero-pads to 4 digits minimum, grows naturally beyond 9999. Parallels
// GenerateAssetExternalKey.
func GenerateLocationExternalKey(seq int) string {
	return fmt.Sprintf("LOC-%04d", seq)
}

func (s *Storage) UpdateLocation(ctx context.Context, orgID, id int, request location.UpdateLocationRequest) (*location.LocationWithParent, error) {
	updates := []string{}
	args := []any{id, orgID}
	argPos := 3
	fields, err := mapLocationReqToFields(request)

	if err != nil {
		return nil, err
	}

	// Nil entries (only from ClearValidTo) pass through as SQL NULL.
	for key, value := range fields {
		updates = append(updates, fmt.Sprintf("%s = $%d", key, argPos))
		args = append(args, value)
		argPos++
	}

	// Empty effective body (e.g. PATCH body that decoded to no writable fields
	// after the read-only drop in TRA-608, or a `{}` body) is a no-op success:
	// return the unchanged record so a verbatim GET → PATCH round-trip with only
	// read-only fields succeeds. TRA-619.
	if len(updates) == 0 {
		return s.getLocationWithParentByID(ctx, orgID, id)
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
		// external_key is immutable via UpdateLocation (TRA-664); the only
		// uniqueness collision reachable here would be a future-added
		// unique column. Keep the generic conflict error.
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("location update conflicts with an existing unique constraint")
		}
		if strings.Contains(err.Error(), "parent_location_id_fkey") {
			return nil, fmt.Errorf("invalid parent_location_id: parent location does not exist")
		}
		return nil, fmt.Errorf("failed to update location: %w", err)
	}

	return s.getLocationWithParentByID(ctx, orgID, updatedID)
}

// RenameLocation mutates the location's external_key. The DB trigger
// cascade_location_path_change (migration 000038) rewrites tree_path on
// this row and every descendant inside the same statement, so the whole
// cascade is atomic with the rename. Returns the updated row plus the
// count of descendant rows whose tree_path changed (does not include this
// row itself; same-value rename returns 0). TRA-664 / BB26 D7.
func (s *Storage) RenameLocation(ctx context.Context, orgID, id int, newExternalKey string) (*location.LocationWithParent, int, error) {
	var updatedID int
	var descendantCount int

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var currentKey string
		err := tx.QueryRow(ctx, `
			SELECT external_key FROM trakrf.locations
			WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		`, id, orgID).Scan(&currentKey)
		if err != nil {
			return err
		}

		// Same-value rename: nothing changes, no cascade fires.
		// descendant_count_affected stays 0 because no tree_path changed.
		if currentKey == newExternalKey {
			updatedID = id
			return nil
		}

		// Count live descendants BEFORE the update. Soft-deleted rows are
		// excluded because their tree_path is no longer observable through
		// the public API; integrators don't need to re-fetch a subtree
		// they can't see.
		err = tx.QueryRow(ctx, `
			SELECT count(*)
			FROM trakrf.locations
			WHERE org_id = $1
			  AND deleted_at IS NULL
			  AND id != $2
			  AND path <@ (
			      SELECT path FROM trakrf.locations
			      WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL
			  )
		`, orgID, id).Scan(&descendantCount)
		if err != nil {
			return err
		}

		return tx.QueryRow(ctx, `
			UPDATE trakrf.locations
			SET external_key = $3, updated_at = NOW()
			WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
			RETURNING id
		`, id, orgID, newExternalKey).Scan(&updatedID)
	})

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, 0, nil
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, 0, fmt.Errorf("location with external_key %s already exists", newExternalKey)
		}
		return nil, 0, fmt.Errorf("failed to rename location: %w", err)
	}

	loc, err := s.getLocationWithParentByID(ctx, orgID, updatedID)
	if err != nil {
		return nil, 0, err
	}
	return loc, descendantCount, nil
}

func (s *Storage) GetLocationByID(ctx context.Context, orgID, id int) (*location.Location, error) {
	query := `
	SELECT id, org_id, name, external_key, parent_location_id, path, depth,
	       description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
	FROM trakrf.locations
	WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
	`
	var loc location.Location
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, orgID).Scan(&loc.ID, &loc.OrgID, &loc.Name,
			&loc.ExternalKey, &loc.ParentID, &loc.TreePath, &loc.Depth, &loc.Description,
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
// tags.location_id is a plain FK that does not enforce same-org — see
// TRA-431 for the cross-tenant leak this prevents.
func (s *Storage) GetLocationsByIDs(ctx context.Context, orgID int, ids []int) ([]*location.Location, error) {
	if len(ids) == 0 {
		return []*location.Location{}, nil
	}

	query := `
	SELECT id, org_id, name, external_key, parent_location_id, path, depth,
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
				&loc.ExternalKey, &loc.ParentID, &loc.TreePath, &loc.Depth, &loc.Description,
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
		SELECT id, org_id, name, external_key, parent_location_id, path, depth,
		       description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at,
		       'target' as relation_type
		FROM trakrf.locations
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
	)
	SELECT l.id, l.org_id, l.name, l.external_key, l.parent_location_id, l.path, l.depth,
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

			if err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.ExternalKey,
				&loc.ParentID, &loc.TreePath, &loc.Depth, &loc.Description,
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
		SELECT id, org_id, name, external_key, parent_location_id, path, depth,
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
			if err := rows.Scan(&loc.ID, &loc.OrgID, &loc.Name, &loc.ExternalKey,
				&loc.ParentID, &loc.TreePath, &loc.Depth, &loc.Description,
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

// CountActiveChildLocations returns the number of non-deleted locations whose
// parent_location_id points at id. Used by DELETE /locations/{id} to refuse a
// delete that would orphan descendants and leave their parent_id / path
// pointing at a soft-deleted parent (TRA-644 / BB22 F2).
func (s *Storage) CountActiveChildLocations(ctx context.Context, orgID, id int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM trakrf.locations
		WHERE org_id = $1 AND parent_location_id = $2 AND deleted_at IS NULL
	`
	var count int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, id).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count child locations: %w", err)
	}
	return count, nil
}

// CountActiveAssetsAtLocation returns the number of non-deleted assets placed
// directly at the location. Used by DELETE /locations/{id} to refuse a delete
// that would leave assets pointing at a soft-deleted location (TRA-644 / BB22
// F2).
func (s *Storage) CountActiveAssetsAtLocation(ctx context.Context, orgID, locationID int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM trakrf.assets
		WHERE org_id = $1 AND current_location_id = $2 AND deleted_at IS NULL
	`
	var count int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, locationID).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count assets at location: %w", err)
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
// key and its tags — same shape as GET /locations/{identifier}.
// Uses ltree @> operator: ancestor_path @> child_path.
// Both the outer query and the path subselect are scoped to orgID so cross-tenant paths
// that happen to share an identifier (e.g. two orgs both using "whs-01") stay isolated.
func (s *Storage) GetAncestors(ctx context.Context, orgID, id int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.external_key, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.external_key
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

// ListAncestorsPaginated returns the ancestors of a location ordered by depth
// (root first), with LIMIT/OFFSET applied. The id ASC tiebreaker ensures
// fully-deterministic paging across requests with the same offset.
func (s *Storage) ListAncestorsPaginated(ctx context.Context, orgID, id, limit, offset int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.external_key, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.external_key
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.path @> (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND l.id != $2
		  AND l.deleted_at IS NULL
		ORDER BY l.depth ASC, l.id ASC
		LIMIT $3 OFFSET $4
	`
	return s.scanHierarchyRows(ctx, query, "ancestor", orgID, orgID, id, limit, offset)
}

// CountAncestors returns the total number of ancestors of the given location,
// matching the WHERE clause used by ListAncestorsPaginated.
func (s *Storage) CountAncestors(ctx context.Context, orgID, id int) (int, error) {
	query := `
		SELECT COUNT(*) FROM trakrf.locations
		WHERE org_id = $1
		  AND path @> (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND id != $2
		  AND deleted_at IS NULL
	`
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, id).Scan(&n)
	})
	return n, err
}

// GetDescendants returns all descendant locations of a given location (children at all levels),
// projected through LocationWithParent so every entry carries its parent's natural key
// and its tags — same shape as GET /locations/{identifier}.
// Uses ltree <@ operator: child_path <@ parent_path.
// Both the outer query and the path subselect are scoped to orgID: ltree paths are derived
// from identifier segments alone (see migration 000018), so without this fence two tenants
// with identical identifier hierarchies would see each other's subtrees.
func (s *Storage) GetDescendants(ctx context.Context, orgID, id int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.external_key, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.external_key
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

// ListDescendantsPaginated returns all descendants of a location ordered
// depth-first by ltree path, with LIMIT/OFFSET applied. The id ASC tiebreaker
// keeps paging deterministic across calls.
func (s *Storage) ListDescendantsPaginated(ctx context.Context, orgID, id, limit, offset int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.external_key, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.external_key
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.path <@ (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND l.id != $2
		  AND l.deleted_at IS NULL
		ORDER BY l.path ASC, l.id ASC
		LIMIT $3 OFFSET $4
	`
	return s.scanHierarchyRows(ctx, query, "descendant", orgID, orgID, id, limit, offset)
}

// CountDescendants returns the total number of descendants of the given
// location, matching the WHERE clause used by ListDescendantsPaginated.
func (s *Storage) CountDescendants(ctx context.Context, orgID, id int) (int, error) {
	query := `
		SELECT COUNT(*) FROM trakrf.locations
		WHERE org_id = $1
		  AND path <@ (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND id != $2
		  AND deleted_at IS NULL
	`
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, id).Scan(&n)
	})
	return n, err
}

// GetChildren returns immediate children of a given location (depth = parent_depth + 1),
// projected through LocationWithParent for parent-identifier and tag parity
// with GET /locations/{identifier}.
// parent_location_id references a globally unique PK so the query alone is not cross-tenant
// reachable, but the orgID filter keeps the invariant explicit (defense in depth) and in
// line with GetAncestors/GetDescendants.
func (s *Storage) GetChildren(ctx context.Context, orgID, id int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.external_key, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.external_key
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

// ListChildrenPaginated returns immediate children (depth = parent_depth + 1)
// of a location ordered alphabetically by name, with LIMIT/OFFSET applied.
// The id ASC tiebreaker keeps paging deterministic when sibling names collide.
func (s *Storage) ListChildrenPaginated(ctx context.Context, orgID, id, limit, offset int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.external_key, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.external_key
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.parent_location_id = $2
		  AND l.deleted_at IS NULL
		ORDER BY l.name ASC, l.id ASC
		LIMIT $3 OFFSET $4
	`
	return s.scanHierarchyRows(ctx, query, "child", orgID, orgID, id, limit, offset)
}

// CountChildren returns the total number of immediate children of the given
// location, matching the WHERE clause used by ListChildrenPaginated.
func (s *Storage) CountChildren(ctx context.Context, orgID, id int) (int, error) {
	query := `
		SELECT COUNT(*) FROM trakrf.locations
		WHERE org_id = $1
		  AND parent_location_id = $2
		  AND deleted_at IS NULL
	`
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, id).Scan(&n)
	})
	return n, err
}

// scanHierarchyRows runs a hierarchy query whose projection ends in p.external_key
// (LEFT JOIN parent) and then bulk-fetches tags for the returned locations.
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
				loc       location.Location
				parExtKey *string
			)
			if err := rows.Scan(
				&loc.ID, &loc.OrgID, &loc.Name, &loc.ExternalKey,
				&loc.ParentID, &loc.TreePath, &loc.Depth, &loc.Description,
				&loc.ValidFrom, &loc.ValidTo, &loc.IsActive,
				&loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
				&parExtKey,
			); err != nil {
				return fmt.Errorf("failed to scan %s: %w", kind, err)
			}
			out = append(out, location.LocationWithParent{
				LocationView:      location.LocationView{Location: loc},
				ParentExternalKey: parExtKey,
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
		idMap, err := s.getTagsForLocations(ctx, orgID, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Tags = idMap[out[i].ID]
			if out[i].Tags == nil {
				out[i].Tags = []shared.Tag{}
			}
		}
	}

	return out, nil
}

// CreateLocationWithTags creates a location with tags in a single transaction
func (s *Storage) CreateLocationWithTags(ctx context.Context, orgID int, request location.CreateLocationWithTagsRequest) (*location.LocationWithParent, error) {
	// Auto-generate external_key if empty (TRA-665 / BB26 D3). Mirrors
	// CreateAssetWithTags's ASSET-NNNN behavior.
	if strings.TrimSpace(request.ExternalKey) == "" {
		seq, err := s.GetNextLocationSequence(ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate external_key: %w", err)
		}
		request.ExternalKey = GenerateLocationExternalKey(seq)
	}

	tagsJSON, err := tagsToJSON(request.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize tags: %w", err)
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

	query := `SELECT * FROM trakrf.create_location_with_tags($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	var locationID int
	var tagIDs []int

	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			orgID,
			request.ExternalKey,
			request.Name,
			request.Description,
			request.ParentID,
			validFrom,
			validTo,
			isActive,
			nil, // metadata - not used in CreateLocationRequest
			tagsJSON,
		).Scan(&locationID, &tagIDs)
	})

	if err != nil {
		return nil, parseLocationWithTagsError(err, request.ExternalKey)
	}

	return s.getLocationWithParentByID(ctx, orgID, locationID)
}

// GetLocationViewByID fetches a location with its tags
func (s *Storage) GetLocationViewByID(ctx context.Context, orgID, id int) (*location.LocationView, error) {
	baseLoc, err := s.GetLocationByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if baseLoc == nil {
		return nil, nil
	}

	tags, err := s.GetTagsByLocationID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	return &location.LocationView{
		Location: *baseLoc,
		Tags:     tags,
	}, nil
}

// getLocationWithParentByID returns a LocationWithParent by surrogate id,
// performing the self-join on parent location and fetching identifiers.
// Used by CreateLocationWithTags and UpdateLocation to emit the
// public write-response shape. Returns (nil, nil) if the location doesn't
// exist or is soft-deleted.
func (s *Storage) getLocationWithParentByID(ctx context.Context, orgID, id int) (*location.LocationWithParent, error) {
	query := `
		SELECT
			l.id, l.org_id, l.name, l.external_key, l.parent_location_id,
			l.path, l.depth, l.description, l.valid_from, l.valid_to,
			l.is_active, l.created_at, l.updated_at, l.deleted_at,
			p.external_key
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.id = $1 AND l.org_id = $2 AND l.deleted_at IS NULL
		LIMIT 1
	`
	var (
		loc       location.Location
		parExtKey *string
	)
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, orgID).Scan(
			&loc.ID, &loc.OrgID, &loc.Name, &loc.ExternalKey, &loc.ParentID,
			&loc.TreePath, &loc.Depth, &loc.Description, &loc.ValidFrom, &loc.ValidTo,
			&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
			&parExtKey,
		)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get location with parent by id: %w", err)
	}

	tags, err := s.GetTagsByLocationID(ctx, orgID, loc.ID)
	if err != nil {
		return nil, err
	}

	return &location.LocationWithParent{
		LocationView: location.LocationView{
			Location: loc,
			Tags:     tags,
		},
		ParentExternalKey: parExtKey,
	}, nil
}

// ListLocationViews fetches locations with their tags for an org
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

	tagMap, err := s.getTagsForLocations(ctx, orgID, locationIDs)
	if err != nil {
		return nil, err
	}

	views := make([]location.LocationView, len(locations))
	for i, loc := range locations {
		ids := tagMap[loc.ID]
		if ids == nil {
			ids = []shared.Tag{}
		}
		views[i] = location.LocationView{
			Location: loc,
			Tags:     ids,
		}
	}

	return views, nil
}

func parseLocationWithTagsError(err error, externalKey string) error {
	errStr := err.Error()

	if strings.Contains(errStr, "locations_org_id_external_key") ||
		(strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "locations")) {
		return fmt.Errorf("location with external_key %s already exists", externalKey)
	}

	if strings.Contains(errStr, "tags_org_id_type_value") ||
		(strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "tags")) {
		return fmt.Errorf("one or more tags already exist")
	}

	if strings.Contains(errStr, "parent_location_id_fkey") {
		return fmt.Errorf("invalid parent_location_id: parent location does not exist")
	}

	return fmt.Errorf("failed to create location with tags: %w", err)
}

// GetLocationByExternalKey returns the live location with the given natural key
// for the given org, plus the parent location's natural key. Returns (nil, nil)
// if no match.
func (s *Storage) GetLocationByExternalKey(
	ctx context.Context, orgID int, identifier string,
) (*location.LocationWithParent, error) {
	query := `
		SELECT
			l.id, l.org_id, l.name, l.external_key, l.parent_location_id,
			l.path, l.depth, l.description, l.valid_from, l.valid_to,
			l.is_active, l.created_at, l.updated_at, l.deleted_at,
			p.external_key
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1 AND l.external_key = $2 AND l.deleted_at IS NULL
		LIMIT 1
	`
	var (
		loc       location.Location
		parExtKey *string
	)
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, identifier).Scan(
			&loc.ID, &loc.OrgID, &loc.Name, &loc.ExternalKey, &loc.ParentID,
			&loc.TreePath, &loc.Depth, &loc.Description, &loc.ValidFrom, &loc.ValidTo,
			&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
			&parExtKey,
		)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get location by external_key: %w", err)
	}

	tags, err := s.GetTagsByLocationID(ctx, orgID, loc.ID)
	if err != nil {
		return nil, err
	}

	return &location.LocationWithParent{
		LocationView: location.LocationView{
			Location: loc,
			Tags:     tags,
		},
		ParentExternalKey: parExtKey,
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
			l.id, l.org_id, l.name, l.external_key,
			l.parent_location_id, l.path, l.depth, l.description,
			l.valid_from, l.valid_to, l.is_active,
			l.created_at, l.updated_at, l.deleted_at,
			p.external_key
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
				loc       location.Location
				parExtKey *string
			)
			if err := rows.Scan(
				&loc.ID, &loc.OrgID, &loc.Name, &loc.ExternalKey,
				&loc.ParentID, &loc.TreePath, &loc.Depth, &loc.Description,
				&loc.ValidFrom, &loc.ValidTo, &loc.IsActive,
				&loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
				&parExtKey,
			); err != nil {
				return fmt.Errorf("scan location: %w", err)
			}
			out = append(out, location.LocationWithParent{
				LocationView:      location.LocationView{Location: loc},
				ParentExternalKey: parExtKey,
			})
		}
		return rows.Err()
	}); err != nil {
		return nil, fmt.Errorf("list locations filtered: %w", err)
	}

	// Bulk-fetch tags for the returned locations, matching the
	// assets-list pattern so the public list endpoint returns `[]` rather
	// than `null` for locations without tags.
	if len(out) > 0 {
		ids := make([]int, len(out))
		for i, l := range out {
			ids[i] = l.ID
		}
		idMap, err := s.getTagsForLocations(ctx, orgID, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Tags = idMap[out[i].ID]
			if out[i].Tags == nil {
				out[i].Tags = []shared.Tag{}
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
	// TRA-659 / BB25 A3: include_deleted relaxes the soft-delete filter so
	// callers reconciling against an external system of record can enumerate
	// deleted rows alongside live ones. Temporal validity still applies.
	// Orthogonal to is_active.
	clauses := []string{
		"l.org_id = $1",
		temporallyEffective("l"),
	}
	if !f.IncludeDeleted {
		clauses = append(clauses, "l.deleted_at IS NULL")
	}
	args := []any{orgID}

	if len(f.ParentIDs) > 0 {
		args = append(args, f.ParentIDs)
		clauses = append(clauses, fmt.Sprintf("p.id = ANY($%d::int[])", len(args)))
	}
	if len(f.ParentExternalKeys) > 0 {
		args = append(args, f.ParentExternalKeys)
		clauses = append(clauses, fmt.Sprintf("p.external_key = ANY($%d::text[])", len(args)))
	}
	if len(f.ExternalKeys) > 0 {
		args = append(args, f.ExternalKeys)
		clauses = append(clauses, fmt.Sprintf("l.external_key = ANY($%d::text[])", len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		clauses = append(clauses, fmt.Sprintf("l.is_active = $%d", len(args)))
	}
	if f.Q != nil {
		args = append(args, "%"+*f.Q+"%")
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(l.name ILIKE $%d OR l.external_key ILIKE $%d OR l.description ILIKE $%d "+
				"OR EXISTS (SELECT 1 FROM trakrf.tags i "+
				"WHERE i.location_id = l.id AND i.is_active = true "+
				"AND i.deleted_at IS NULL AND "+temporallyEffective("i")+
				" AND i.value ILIKE $%d))",
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
		col := s.Field
		if col == "tree_path" {
			// Wire field is tree_path (TRA-580 C-1); SQL column stays `path`.
			col = "path"
		}
		out = append(out, "l."+col+" "+dir)
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
	// external_key is intentionally not writable via UpdateLocationRequest
	// (TRA-664 / BB26 D7); see RenameLocation for that path.
	// parent_location_id is nullable in the DB; SQL NULL on clear.
	if req.ClearParentID {
		fields["parent_location_id"] = nil
	} else if req.ParentID != nil {
		fields["parent_location_id"] = *req.ParentID
	}
	// description: explicit null on PATCH clears to empty string. Same rationale
	// as assets — preserves the null-on-read contract without changing every
	// existing scan to handle SQL NULL into a Go string. (TRA-614 / BB19 §S1.)
	if req.ClearDescription {
		fields["description"] = ""
	} else if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.ValidFrom != nil && !req.ValidFrom.IsZero() {
		fields["valid_from"] = req.ValidFrom.ToTime()
	}
	if req.ClearValidTo {
		fields["valid_to"] = nil
	} else if req.ValidTo != nil && !req.ValidTo.IsZero() {
		fields["valid_to"] = req.ValidTo.ToTime()
	}
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}

	return fields, nil
}

// GetLocationWithParentByIDForTest exposes getLocationWithParentByID to
// integration tests in the same package. Production code must use
// GetLocationByExternalKey or the CreateLocationWithTags /
// UpdateLocation return values.
func (s *Storage) GetLocationWithParentByIDForTest(ctx context.Context, orgID, id int) (*location.LocationWithParent, error) {
	return s.getLocationWithParentByID(ctx, orgID, id)
}
