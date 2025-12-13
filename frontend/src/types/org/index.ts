/**
 * Organization RBAC Types
 * Types for organization management, roles, and membership
 */

// Role hierarchy: owner > admin > manager > operator > viewer
export type OrgRole = 'owner' | 'admin' | 'manager' | 'operator' | 'viewer';

/**
 * Minimal org reference (for lists)
 */
export interface UserOrg {
  id: number;
  name: string;
}

/**
 * Org with user's role (for current org context)
 */
export interface UserOrgWithRole {
  id: number;
  name: string;
  role: OrgRole;
}

/**
 * User profile with org memberships
 * Returned by GET /api/v1/users/me
 */
export interface UserProfile {
  id: number;
  name: string;
  email: string;
  is_superadmin: boolean;
  current_org: UserOrgWithRole | null;
  orgs: UserOrg[];
}

/**
 * Full organization entity
 */
export interface Organization {
  id: number;
  name: string;
  identifier: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

/**
 * Request to create a new organization
 */
export interface CreateOrgRequest {
  name: string;
}

/**
 * Response from creating an organization
 */
export interface CreateOrgResponse {
  data: Organization;
}

/**
 * Request to set the current organization
 */
export interface SetCurrentOrgRequest {
  org_id: number;
}

/**
 * Organization member (for Phase 3b: Members screen)
 */
export interface OrgMember {
  user_id: number;
  name: string;
  email: string;
  role: OrgRole;
  joined_at: string;
}

/**
 * Invitation (for Phase 3c: Invitations)
 */
export interface Invitation {
  id: number;
  email: string;
  role: OrgRole;
  invited_by: { id: number; name: string } | null;
  expires_at: string;
  created_at: string;
}

/**
 * Response from accepting an invitation
 */
export interface AcceptInvitationResponse {
  message: string;
  org_id: number;
  org_name: string;
  role: string;
}
