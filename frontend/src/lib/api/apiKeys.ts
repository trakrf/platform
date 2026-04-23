// NOTE: Unlike `orgs.ts` (which returns the raw AxiosResponse so callers
// unwrap `.data.data`), this module unwraps Axios `.data` internally so
// callers get the server body directly. Keeps the contract consistent with
// test mocks and avoids the double-`.data.data` access pattern.
import { apiClient } from './client';
import type { APIKey, CreateAPIKeyRequest, APIKeyCreateResponse } from '../../types/apiKey';

export const apiKeysApi = {
  list: async (orgId: number): Promise<{ data: APIKey[] }> => {
    const resp = await apiClient.get<{ data: APIKey[] }>(`/orgs/${orgId}/api-keys`);
    return resp.data;
  },

  create: async (orgId: number, req: CreateAPIKeyRequest): Promise<APIKeyCreateResponse> => {
    const resp = await apiClient.post<{ data: APIKeyCreateResponse }>(`/orgs/${orgId}/api-keys`, req);
    return resp.data.data;
  },

  revoke: async (orgId: number, keyId: number): Promise<void> => {
    await apiClient.delete<void>(`/orgs/${orgId}/api-keys/${keyId}`);
  },
};
