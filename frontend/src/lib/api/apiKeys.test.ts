import { describe, it, expect, vi, beforeEach } from 'vitest';
import { apiKeysApi } from './apiKeys';
import { apiClient } from './client';

vi.mock('./client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('apiKeysApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('list GETs /orgs/{orgId}/api-keys', async () => {
    (apiClient.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] });
    await apiKeysApi.list(42);
    expect(apiClient.get).toHaveBeenCalledWith('/orgs/42/api-keys');
  });

  it('create POSTs request body to /orgs/{orgId}/api-keys and returns unwrapped data', async () => {
    const payload = {
      key: 'eyJ...',
      id: 1,
      name: 'x',
      scopes: ['assets:read' as const],
      created_at: '2026-04-19T00:00:00Z',
      expires_at: null,
    };
    (apiClient.post as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { data: payload },
    });
    const req = { name: 'x', scopes: ['assets:read' as const] };
    const result = await apiKeysApi.create(42, req);
    expect(apiClient.post).toHaveBeenCalledWith('/orgs/42/api-keys', req);
    expect(result).toEqual(payload);
  });

  it('revoke DELETEs /orgs/{orgId}/api-keys/{id}', async () => {
    (apiClient.delete as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
    await apiKeysApi.revoke(42, 99);
    expect(apiClient.delete).toHaveBeenCalledWith('/orgs/42/api-keys/99');
  });
});
