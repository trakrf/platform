import { describe, it, expect, vi, beforeEach } from 'vitest';
import { webhooksApi } from './webhooks';
import { apiClient } from './client';

vi.mock('./client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('webhooksApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('unwraps the collection to the org single webhook', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      data: { data: [{ id: 1, url: 'https://x/y' }] },
    } as never);

    const wh = await webhooksApi.get();
    expect(wh?.id).toBe(1);
  });

  it('returns null when the org has no webhook', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ data: { data: [] } } as never);
    expect(await webhooksApi.get()).toBeNull();
  });

  // Regression: a bodyless axios POST omits Content-Type, and every write route
  // in the session group sits behind middleware.ContentType, which answers 415.
  // The test fire looked fine in a handler test (that router mounts no
  // ContentType middleware) and failed against the real stack.
  it('sends a body on test fire so Content-Type survives', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({
      data: { data: { status_code: 200 } },
    } as never);

    await webhooksApi.test(42);

    expect(apiClient.post).toHaveBeenCalledWith('/webhooks/42/test', {});
    const [, body] = vi.mocked(apiClient.post).mock.calls[0];
    expect(body).toBeDefined();
  });

  it('posts create and patch payloads through', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({ data: { data: { id: 7 } } } as never);
    vi.mocked(apiClient.patch).mockResolvedValue({ data: { data: { id: 7 } } } as never);

    await webhooksApi.create({ url: 'https://x/y', enabled: true });
    expect(apiClient.post).toHaveBeenCalledWith('/webhooks', {
      url: 'https://x/y',
      enabled: true,
    });

    await webhooksApi.update(7, { enabled: false });
    expect(apiClient.patch).toHaveBeenCalledWith('/webhooks/7', { enabled: false });
  });
});
