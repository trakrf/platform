import { apiClient } from './client';
import type { APIKey, CreateAPIKeyRequest, APIKeyCreateResponse } from '../../types/apiKey';

export const apiKeysApi = {
  list: (orgId: number) =>
    apiClient.get<{ data: APIKey[] }>(`/orgs/${orgId}/api-keys`),

  create: (orgId: number, req: CreateAPIKeyRequest) =>
    apiClient.post<APIKeyCreateResponse>(`/orgs/${orgId}/api-keys`, req),

  revoke: (orgId: number, keyId: number) =>
    apiClient.delete<void>(`/orgs/${orgId}/api-keys/${keyId}`),
};
