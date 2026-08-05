import { apiClient } from './client';
import type { UserProfile } from '@/types/org';

/** Partial self-edit: omit a field to leave it unchanged (TRA-958). */
export interface UpdateProfileRequest {
  name?: string;
  email?: string;
}

export const usersApi = {
  // PATCH /users/me, not PUT /users/{id}: the id-keyed route is internal and
  // takes its target from the path. This one takes it from the session, and
  // returns the same envelope GET /users/me does.
  updateProfile: (data: UpdateProfileRequest) =>
    apiClient.patch<{ data: UserProfile }>('/users/me', data, {
      headers: { 'Content-Type': 'application/json' },
    }),
};
