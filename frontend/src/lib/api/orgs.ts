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
} from '../../types/org';

export interface MessageResponse {
  message: string;
}

export interface SetCurrentOrgResponse {
  message: string;
  token: string;
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
    apiClient.patch<MessageResponse>(`/orgs/${orgId}/members/${userId}`, { role }),

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
};
