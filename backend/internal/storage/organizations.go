package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/organization"
)

// ListUserOrgs returns all organizations the user belongs to
func (s *Storage) ListUserOrgs(ctx context.Context, userID int) ([]organization.UserOrg, error) {
	query := `
		SELECT o.id, o.name
		FROM trakrf.organizations o
		JOIN trakrf.org_users ou ON o.id = ou.org_id
		WHERE ou.user_id = $1
		  AND ou.deleted_at IS NULL
		  AND o.deleted_at IS NULL
		ORDER BY o.name ASC
	`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user orgs: %w", err)
	}
	defer rows.Close()

	orgs := []organization.UserOrg{}
	for rows.Next() {
		var org organization.UserOrg
		if err := rows.Scan(&org.ID, &org.Name); err != nil {
			return nil, fmt.Errorf("failed to scan org: %w", err)
		}
		orgs = append(orgs, org)
	}
	return orgs, nil
}

// GetOrganizationByID retrieves a single organization by its ID.
func (s *Storage) GetOrganizationByID(ctx context.Context, id int) (*organization.Organization, error) {
	query := `
		SELECT id, name, identifier, metadata,
		       valid_from, valid_to, is_active, created_at, updated_at
		FROM trakrf.organizations
		WHERE id = $1 AND deleted_at IS NULL
	`
	var org organization.Organization
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&org.ID, &org.Name, &org.Identifier, &org.Metadata,
		&org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	return &org, nil
}

// GetOrganizationByIdentifier retrieves a single organization by its identifier
// (the URL-safe natural key, e.g. "bb-test-org"). Returns (nil, nil) when no
// active row matches, matching the no-rows convention of GetOrganizationByID.
func (s *Storage) GetOrganizationByIdentifier(ctx context.Context, identifier string) (*organization.Organization, error) {
	query := `
		SELECT id, name, identifier, metadata,
		       valid_from, valid_to, is_active, created_at, updated_at
		FROM trakrf.organizations
		WHERE identifier = $1 AND deleted_at IS NULL
	`
	var org organization.Organization
	err := s.pool.QueryRow(ctx, query, identifier).Scan(
		&org.ID, &org.Name, &org.Identifier, &org.Metadata,
		&org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get organization by identifier: %w", err)
	}
	return &org, nil
}

// CreateOrganization creates a new organization.
// Returns the created org. Caller must separately add user to org_users.
func (s *Storage) CreateOrganization(ctx context.Context, name, identifier string) (*organization.Organization, error) {
	query := `
		INSERT INTO trakrf.organizations (name, identifier)
		VALUES ($1, $2)
		RETURNING id, name, identifier, metadata,
		          valid_from, valid_to, is_active, created_at, updated_at
	`
	var org organization.Organization
	err := s.pool.QueryRow(ctx, query, name, identifier).Scan(
		&org.ID, &org.Name, &org.Identifier, &org.Metadata,
		&org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("organization identifier already taken")
		}
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}
	return &org, nil
}

// UpdateOrganization updates an organization's name.
func (s *Storage) UpdateOrganization(ctx context.Context, id int, request organization.UpdateOrganizationRequest) (*organization.Organization, error) {
	if request.Name == nil {
		return s.GetOrganizationByID(ctx, id)
	}

	query := `
		UPDATE trakrf.organizations
		SET name = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, name, identifier, metadata,
		          valid_from, valid_to, is_active, created_at, updated_at
	`
	var org organization.Organization
	err := s.pool.QueryRow(ctx, query, id, *request.Name).Scan(
		&org.ID, &org.Name, &org.Identifier, &org.Metadata,
		&org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update organization: %w", err)
	}
	return &org, nil
}

// GetOrgGeofenceDefaults returns the org-tier geofence tuning overrides (TRA-955)
// parsed from organizations.metadata.geofence_defaults. Unset keys are nil. A
// missing org yields empty defaults (the geofence engine treats this tier as
// best-effort and falls back to the system/code default).
func (s *Storage) GetOrgGeofenceDefaults(ctx context.Context, orgID int) (organization.GeofenceDefaults, error) {
	org, err := s.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return organization.GeofenceDefaults{}, err
	}
	if org == nil {
		return organization.GeofenceDefaults{}, nil
	}
	return organization.ParseGeofenceDefaults(org.Metadata), nil
}

// UpdateOrgGeofenceDefaults replaces metadata.geofence_defaults with d (TRA-955).
// Full-replace: nil fields are omitted from the written object so they fall back
// to the system tier. Other metadata keys are preserved via jsonb_set.
func (s *Storage) UpdateOrgGeofenceDefaults(ctx context.Context, orgID int, d organization.GeofenceDefaults) error {
	sub := map[string]any{}
	if d.RSSIThreshold != nil {
		sub["rssi_threshold"] = *d.RSSIThreshold
	}
	if d.AgeOutSeconds != nil {
		sub["age_out_seconds"] = *d.AgeOutSeconds
	}
	if d.AutoOffSeconds != nil {
		sub["auto_off_seconds"] = *d.AutoOffSeconds
	}
	if d.Mode != nil {
		sub["mode"] = *d.Mode
	}
	blob, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("failed to marshal geofence defaults: %w", err)
	}
	query := `
		UPDATE trakrf.organizations
		SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{geofence_defaults}', $2::jsonb, true),
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`
	result, err := s.pool.Exec(ctx, query, orgID, blob)
	if err != nil {
		return fmt.Errorf("failed to update geofence defaults: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("organization not found")
	}
	return nil
}

// SoftDeleteOrganization marks an organization as deleted.
func (s *Storage) SoftDeleteOrganization(ctx context.Context, id int) error {
	query := `UPDATE trakrf.organizations SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("organization not found")
	}
	return nil
}

// SoftDeleteOrganizationWithMangle marks an organization as deleted and mangles name/identifier
// to free them for reuse. The mangled format preserves the original values for audit purposes.
func (s *Storage) SoftDeleteOrganizationWithMangle(ctx context.Context, id int, mangledName, mangledIdentifier string, deletedAt time.Time) error {
	query := `
		UPDATE trakrf.organizations
		SET name = $2, identifier = $3, deleted_at = $4
		WHERE id = $1 AND deleted_at IS NULL
	`
	result, err := s.pool.Exec(ctx, query, id, mangledName, mangledIdentifier, deletedAt)
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("organization not found")
	}
	return nil
}
