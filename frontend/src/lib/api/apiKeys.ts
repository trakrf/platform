import { apiClient } from './client';
import type { APIKey, CreateAPIKeyRequest, APIKeyCreateResponse } from '../../types/apiKey';

export const apiKeysApi = {
  list: async (orgId: number): Promise<{ data: APIKey[] }> => {
    const resp = await apiClient.get<{ data: APIKey[] }>(`/orgs/${orgId}/api-keys`);
    return resp.data;
  },

  create: async (orgId: number, req: CreateAPIKeyRequest): Promise<APIKeyCreateResponse> => {
    const resp = await apiClient.post<APIKeyCreateResponse>(`/orgs/${orgId}/api-keys`, req);
    return resp.data;
  },

  revoke: async (orgId: number, keyId: number): Promise<void> => {
    await apiClient.delete<void>(`/orgs/${orgId}/api-keys/${keyId}`);
  },
};
