import { apiClient } from './client';

export interface SignupRequest {
  email: string;
  password: string;
  org_name?: string;
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
    token: string;
    user: User;
  };
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
};
