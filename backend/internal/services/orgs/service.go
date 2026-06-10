package orgs

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

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
// creatorEmail is used only for the best-effort superadmin notification (TRA-977).
func (s *Service) CreateOrgWithAdmin(ctx context.Context, name string, creatorUserID int, creatorEmail string) (*organization.Organization, error) {
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

	// Notify superadmins of the new org (TRA-977). Fire-and-forget on a detached
	// context so it never delays or fails the create. Internal creates leave
	// subscription_expires_at NULL (perpetual).
	go s.notifyOrgCreated(context.Background(), org, creatorEmail)

	return &org, nil
}

// DeleteOrgWithConfirmation deletes an org if the confirmation name matches (case-insensitive).
// It mangles the name and identifier to free them for reuse while preserving audit trail.
// actorEmail is used only for the best-effort superadmin churn notification (TRA-977).
func (s *Service) DeleteOrgWithConfirmation(ctx context.Context, orgID int, confirmName, actorEmail string) error {
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

	// Capture the real name/identifier before mangling, for the notification.
	origName, origIdentifier := org.Name, org.Identifier

	// Mangle name and identifier to free them for reuse
	deletedAt := time.Now().UTC()
	prefix := fmt.Sprintf("*** DELETED %s *** ", deletedAt.Format(time.RFC3339))
	mangledName := prefix + org.Name
	mangledIdentifier := prefix + org.Identifier

	if err := s.storage.SoftDeleteOrganizationWithMangle(ctx, orgID, mangledName, mangledIdentifier, deletedAt); err != nil {
		return err
	}

	// Notify superadmins of the churn (TRA-977). Fire-and-forget on a detached
	// context so it never delays or fails the delete response.
	go s.notifyOrgDeleted(context.Background(), origName, origIdentifier, actorEmail, deletedAt)

	return nil
}

// notifySuperadmins lists every active superadmin and invokes send for each.
// Best-effort: a lookup failure is logged not returned, and a send failure to
// one superadmin does not stop the others. Returns the number successfully
// notified (used by tests). Shared by the org create/delete notifications.
func (s *Service) notifySuperadmins(ctx context.Context, send func(adminEmail string) error) int {
	if s.emailClient == nil {
		return 0
	}

	admins, err := s.storage.ListSuperadmins(ctx)
	if err != nil {
		fmt.Printf("warning: failed to list superadmins for org notification: %v\n", err)
		return 0
	}

	sent := 0
	for _, admin := range admins {
		if err := send(admin.Email); err != nil {
			fmt.Printf("warning: failed to send org notification to %s: %v\n", admin.Email, err)
			continue
		}
		sent++
	}
	return sent
}

// notifyOrgCreated emails every superadmin that a new org was created (TRA-977).
func (s *Service) notifyOrgCreated(ctx context.Context, org organization.Organization, creatorEmail string) int {
	return s.notifySuperadmins(ctx, func(adminEmail string) error {
		return s.emailClient.SendOrgCreatedNotification(
			adminEmail, org.Name, org.Identifier, creatorEmail, org.SubscriptionExpiresAt)
	})
}

// notifyOrgDeleted emails every superadmin that an org was deleted (TRA-977).
func (s *Service) notifyOrgDeleted(ctx context.Context, orgName, orgIdentifier, actorEmail string, deletedAt time.Time) int {
	return s.notifySuperadmins(ctx, func(adminEmail string) error {
		return s.emailClient.SendOrgDeletedNotification(
			adminEmail, orgName, orgIdentifier, actorEmail, deletedAt)
	})
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
					cur := &organization.UserOrgWithRole{
						ID:   org.ID,
						Name: org.Name,
						Role: string(role),
					}
					// TRA-922: include the org slug so the UI can pre-fill the
					// required {org_slug}/ publish_topic prefix. Best-effort — a
					// lookup miss leaves Identifier empty rather than failing /me.
					// TRA-947: also populate raw subscription fields for display.
					if full, ferr := s.storage.GetOrganizationByID(ctx, currentOrgID); ferr == nil && full != nil {
						cur.Identifier = full.Identifier
						cur.SubscriptionEnabled = full.SubscriptionEnabled
						cur.SubscriptionExpiresAt = full.SubscriptionExpiresAt
					}
					if entitled, eerr := s.storage.OrgIsEntitled(ctx, currentOrgID); eerr == nil {
						cur.IsEntitled = entitled
					}
					profile.CurrentOrg = cur
					break
				}
			}
		}
	}

	return profile, nil
}

// SetCurrentOrg updates the user's last_org_id after verifying membership.
// Returns an error wrapping storage.ErrOrgUserNotFound when the user is not a
// member of the requested org so callers can distinguish 403 from 500.
func (s *Service) SetCurrentOrg(ctx context.Context, userID, orgID int) error {
	// Verify user is a member
	_, err := s.storage.GetUserOrgRole(ctx, userID, orgID)
	if err != nil {
		if errors.Is(err, storage.ErrOrgUserNotFound) {
			return fmt.Errorf("%w", storage.ErrOrgUserNotFound)
		}
		return fmt.Errorf("verify org membership: %w", err)
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
