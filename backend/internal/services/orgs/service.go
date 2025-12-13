package orgs

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/services/email"
	"github.com/trakrf/platform/backend/internal/storage"
)

type Service struct {
	db          *pgxpool.Pool
	storage     *storage.Storage
	emailClient *email.Client
}

func NewService(db *pgxpool.Pool, storage *storage.Storage, emailClient *email.Client) *Service {
	return &Service{db: db, storage: storage, emailClient: emailClient}
}

// CreateOrgWithAdmin creates a new team org and makes the creator an admin.
func (s *Service) CreateOrgWithAdmin(ctx context.Context, name string, creatorUserID int) (*organization.Organization, error) {
	identifier := slugifyOrgName(name)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Create org
	var org organization.Organization
	orgQuery := `
		INSERT INTO trakrf.organizations (name, identifier)
		VALUES ($1, $2)
		RETURNING id, name, identifier, metadata,
		          valid_from, valid_to, is_active, created_at, updated_at
	`
	err = tx.QueryRow(ctx, orgQuery, name, identifier).Scan(
		&org.ID, &org.Name, &org.Identifier, &org.Metadata,
		&org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, fmt.Errorf("organization identifier already taken")
		}
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// Add creator as admin
	orgUserQuery := `INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'admin')`
	_, err = tx.Exec(ctx, orgUserQuery, org.ID, creatorUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to add creator to org: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &org, nil
}

// DeleteOrgWithConfirmation deletes an org if the confirmation name matches (case-insensitive).
func (s *Service) DeleteOrgWithConfirmation(ctx context.Context, orgID int, confirmName string) error {
	org, err := s.storage.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}
	if org == nil {
		return fmt.Errorf("organization not found")
	}

	// Case-insensitive comparison (GitHub-style)
	if !strings.EqualFold(org.Name, confirmName) {
		return fmt.Errorf("organization name does not match")
	}

	return s.storage.SoftDeleteOrganization(ctx, orgID)
}

// GetUserProfile builds the enhanced /users/me response.
func (s *Service) GetUserProfile(ctx context.Context, userID int) (*organization.UserProfile, error) {
	user, err := s.storage.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	orgs, err := s.storage.ListUserOrgs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user orgs: %w", err)
	}

	profile := &organization.UserProfile{
		ID:           user.ID,
		Name:         user.Name,
		Email:        user.Email,
		IsSuperadmin: user.IsSuperadmin,
		Orgs:         orgs,
	}

	// Determine current org: use last_org_id if set and valid, otherwise first org
	var currentOrgID int
	if user.LastOrgID != nil {
		// Verify user is still a member of this org
		for _, org := range orgs {
			if org.ID == *user.LastOrgID {
				currentOrgID = *user.LastOrgID
				break
			}
		}
	}
	if currentOrgID == 0 && len(orgs) > 0 {
		currentOrgID = orgs[0].ID
	}

	if currentOrgID > 0 {
		// Get role for current org
		role, err := s.storage.GetUserOrgRole(ctx, userID, currentOrgID)
		if err == nil {
			for _, org := range orgs {
				if org.ID == currentOrgID {
					profile.CurrentOrg = &organization.UserOrgWithRole{
						ID:   org.ID,
						Name: org.Name,
						Role: string(role),
					}
					break
				}
			}
		}
	}

	return profile, nil
}

// SetCurrentOrg updates the user's last_org_id after verifying membership.
func (s *Service) SetCurrentOrg(ctx context.Context, userID, orgID int) error {
	// Verify user is a member
	_, err := s.storage.GetUserOrgRole(ctx, userID, orgID)
	if err != nil {
		return fmt.Errorf("you are not a member of this organization")
	}
	return s.storage.UpdateUserLastOrg(ctx, userID, orgID)
}

func slugifyOrgName(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, "@", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	slug = reg.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

// ListMembers returns all members of an organization
func (s *Service) ListMembers(ctx context.Context, orgID int) ([]organization.OrgMember, error) {
	members, err := s.storage.ListOrgMembers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	return members, nil
}

// UpdateMemberRole updates a member's role with last-admin protection
func (s *Service) UpdateMemberRole(ctx context.Context, orgID, targetUserID int, newRole models.OrgRole) error {
	// Get current role
	currentRole, err := s.storage.GetUserOrgRole(ctx, targetUserID, orgID)
	if err != nil {
		return fmt.Errorf("member not found")
	}

	// If demoting from admin, check if they're the last admin
	if currentRole == models.RoleAdmin && newRole != models.RoleAdmin {
		adminCount, err := s.storage.CountOrgAdmins(ctx, orgID)
		if err != nil {
			return fmt.Errorf("failed to check admin count: %w", err)
		}
		if adminCount <= 1 {
			return fmt.Errorf("cannot demote the last admin")
		}
	}

	return s.storage.UpdateMemberRole(ctx, orgID, targetUserID, newRole)
}

// RemoveMember removes a member with last-admin and self-removal protection
func (s *Service) RemoveMember(ctx context.Context, orgID, targetUserID, actorUserID int) error {
	// Prevent self-removal
	if targetUserID == actorUserID {
		return fmt.Errorf("cannot remove yourself")
	}

	// Check if target is a member
	targetRole, err := s.storage.GetUserOrgRole(ctx, targetUserID, orgID)
	if err != nil {
		return fmt.Errorf("member not found")
	}

	// If removing an admin, check if they're the last admin
	if targetRole == models.RoleAdmin {
		adminCount, err := s.storage.CountOrgAdmins(ctx, orgID)
		if err != nil {
			return fmt.Errorf("failed to check admin count: %w", err)
		}
		if adminCount <= 1 {
			return fmt.Errorf("cannot remove the last admin")
		}
	}

	return s.storage.RemoveMember(ctx, orgID, targetUserID)
}
