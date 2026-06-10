import { apiClient } from './client';
import type {
  UserProfile,
  Organization,
  CreateOrgRequest,
  CreateOrgResponse,
  SetCurrentOrgRequest,
  OrgMember,
  Invitation,
  AcceptInvitationResponse,
  GeofenceDefaults,
  GeofenceDefaultsView,
  AdminOrgListItem,
  UpdateEntitlementRequest,
} from '../../types/org';

export interface MessageResponse {
  message: string;
}

export interface SetCurrentOrgResponse {
  message: string;
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export const orgsApi = {
  // Profile
  getProfile: () => apiClient.get<{ data: UserProfile }>('/users/me'),

  // Org CRUD
  get: (orgId: number) => apiClient.get<{ data: Organization }>(`/orgs/${orgId}`),

  create: (data: CreateOrgRequest) =>
    apiClient.post<CreateOrgResponse>('/orgs', data),

  update: (orgId: number, data: { name: string }) =>
    apiClient.put<{ data: Organization }>(`/orgs/${orgId}`, data),

  delete: (orgId: number, confirmName: string) =>
    apiClient.delete<MessageResponse>(`/orgs/${orgId}`, {
      data: { confirm_name: confirmName },
    }),

  // Org switching
  setCurrentOrg: (data: SetCurrentOrgRequest) =>
    apiClient.post<SetCurrentOrgResponse>('/users/me/current-org', data),

  // Members (Phase 3b)
  listMembers: (orgId: number) =>
    apiClient.get<{ data: OrgMember[] }>(`/orgs/${orgId}/members`),

  updateMemberRole: (orgId: number, userId: number, role: string) =>
    apiClient.put<MessageResponse>(`/orgs/${orgId}/members/${userId}`, { role }),

  removeMember: (orgId: number, userId: number) =>
    apiClient.delete<MessageResponse>(`/orgs/${orgId}/members/${userId}`),

  // Invitations (Phase 3c)
  listInvitations: (orgId: number) =>
    apiClient.get<{ data: Invitation[] }>(`/orgs/${orgId}/invitations`),

  createInvitation: (orgId: number, email: string, role: string) =>
    apiClient.post<{ data: Invitation }>(`/orgs/${orgId}/invitations`, { email, role }),

  cancelInvitation: (orgId: number, inviteId: number) =>
    apiClient.delete<MessageResponse>(`/orgs/${orgId}/invitations/${inviteId}`),

  resendInvitation: (orgId: number, inviteId: number) =>
    apiClient.post<MessageResponse>(`/orgs/${orgId}/invitations/${inviteId}/resend`, {}),

  // Accept invitation (auth endpoint)
  acceptInvitation: (token: string) =>
    apiClient.post<{ data: AcceptInvitationResponse }>('/auth/accept-invite', { token }),

  // Geofence tuning defaults (TRA-955), internal-only. Read by any member;
  // write is admin-only.
  getGeofenceDefaults: (orgId: number) =>
    apiClient.get<{ data: GeofenceDefaultsView }>(`/orgs/${orgId}/geofence-defaults`),

  updateGeofenceDefaults: (orgId: number, body: GeofenceDefaults) =>
    apiClient.patch<{ data: GeofenceDefaultsView }>(`/orgs/${orgId}/geofence-defaults`, body, {
      headers: { 'Content-Type': 'application/json' },
    }),

  // Superadmin-only cross-org controls (TRA-949). Backend gates these strictly
  // on is_superadmin; a non-superadmin gets 403.
  listAllOrgs: () =>
    apiClient.get<{ data: AdminOrgListItem[] }>('/admin/orgs'),

  updateEntitlement: (orgId: number, body: UpdateEntitlementRequest) =>
    apiClient.patch<{ data: Organization }>(`/orgs/${orgId}/entitlement`, body, {
      headers: { 'Content-Type': 'application/json' },
    }),
};
