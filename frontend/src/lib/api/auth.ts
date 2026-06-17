import { apiClient } from './client';

export interface SignupRequest {
  email: string;
  password: string;
  org_name?: string;
  // TRA-971: contact details required for self-service signup (omitted for the
  // invitation flow, which joins an existing org).
  name?: string;
  phone?: string;
  website?: string;
  // TRA-970: deliberate opt-in to sign up on a non-prod (preview/demo) sandbox.
  acknowledge_non_prod?: boolean;
  invitation_token?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface User {
  id: number;
  email: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface AuthResponse {
  data: {
    access_token: string;
    refresh_token: string;
    expires_in: number;
    user: User;
  };
}

export interface RefreshResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export interface MessageResponse {
  message: string;
}

export interface InvitationInfo {
  org_name: string;
  org_identifier: string;
  role: string;
  email: string;
  user_exists: boolean;
  inviter_name?: string;
}

export interface InvitationInfoResponse {
  data: InvitationInfo;
}

export const authApi = {
  signup: (data: SignupRequest) =>
    apiClient.post<AuthResponse>('/auth/signup', data),

  login: (data: LoginRequest) =>
    apiClient.post<AuthResponse>('/auth/login', data),

  forgotPassword: (email: string) =>
    apiClient.post<MessageResponse>('/auth/forgot-password', {
      email,
      reset_url: `${window.location.origin}/#reset-password`,
    }),

  resetPassword: (token: string, password: string) =>
    apiClient.post<MessageResponse>('/auth/reset-password', { token, password }),

  getInvitationInfo: (token: string) =>
    apiClient.get<InvitationInfoResponse>(
      `/auth/invitation-info?token=${encodeURIComponent(token)}`
    ),

  refresh: (refreshToken: string) =>
    apiClient.post<RefreshResponse>('/auth/refresh', { refresh_token: refreshToken }),

  logout: (refreshToken: string) =>
    apiClient.post<MessageResponse>('/auth/logout', { refresh_token: refreshToken }),
};
