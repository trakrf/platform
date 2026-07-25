import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { WebhooksSection } from '@/components/WebhooksSection';
import { webhooksApi } from '@/lib/api/webhooks';
import type { Webhook } from '@/types/webhook';

vi.mock('@/lib/api/webhooks', () => ({
  webhooksApi: {
    get: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    test: vi.fn(),
  },
}));

vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const existing: Webhook = {
  id: 42,
  org_id: 1,
  url: 'https://example.com/hooks',
  secret: 'whsec_…abcd',
  enabled: true,
  created_at: '2026-07-25T00:00:00Z',
};

describe('WebhooksSection', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('offers a create form when the org has no webhook', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(null);
    render(<WebhooksSection />);

    expect(await screen.findByLabelText(/endpoint url/i)).toHaveValue('');
    expect(screen.getByRole('button', { name: /create webhook/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /send test event/i })).not.toBeInTheDocument();
  });

  // The API returns the cleartext secret exactly once and there is no way to
  // read it back, so the warning has to be unmissable.
  it('reveals the secret once after create, with a copy-it-now warning', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(null);
    vi.mocked(webhooksApi.create).mockResolvedValue({
      ...existing,
      secret: 'whsec_0123456789abcdef',
    });

    render(<WebhooksSection />);
    fireEvent.change(await screen.findByLabelText(/endpoint url/i), {
      target: { value: 'https://example.com/hooks' },
    });
    fireEvent.click(screen.getByRole('button', { name: /create webhook/i }));

    await waitFor(() => {
      expect(screen.getByTestId('webhook-secret')).toHaveTextContent('whsec_0123456789abcdef');
    });
    expect(screen.getByText(/will not be shown again/i)).toBeInTheDocument();
    expect(webhooksApi.create).toHaveBeenCalledWith({
      url: 'https://example.com/hooks',
      enabled: true,
    });
  });

  it('renders the existing webhook with its masked secret and controls', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(existing);
    render(<WebhooksSection />);

    expect(await screen.findByLabelText(/endpoint url/i)).toHaveValue('https://example.com/hooks');
    expect(screen.getByText('whsec_…abcd')).toBeInTheDocument();
    expect(screen.getByLabelText(/deliver events/i)).toBeChecked();
    expect(screen.getByRole('button', { name: /send test event/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /delete/i })).toBeInTheDocument();
  });

  it('saves a url change', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(existing);
    vi.mocked(webhooksApi.update).mockResolvedValue({ ...existing, url: 'https://example.com/v2' });

    render(<WebhooksSection />);
    const input = await screen.findByLabelText(/endpoint url/i);
    fireEvent.change(input, { target: { value: 'https://example.com/v2' } });
    fireEvent.click(screen.getByRole('button', { name: /save webhook/i }));

    await waitFor(() => {
      expect(webhooksApi.update).toHaveBeenCalledWith(42, {
        url: 'https://example.com/v2',
        enabled: true,
      });
    });
  });

  it('toggles delivery off', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(existing);
    vi.mocked(webhooksApi.update).mockResolvedValue({ ...existing, enabled: false });

    render(<WebhooksSection />);
    fireEvent.click(await screen.findByLabelText(/deliver events/i));
    fireEvent.click(screen.getByRole('button', { name: /save webhook/i }));

    await waitFor(() => {
      expect(webhooksApi.update).toHaveBeenCalledWith(42, {
        url: 'https://example.com/hooks',
        enabled: false,
      });
    });
  });

  it('shows the status code a test fire came back with', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(existing);
    vi.mocked(webhooksApi.test).mockResolvedValue({ status_code: 200 });

    render(<WebhooksSection />);
    fireEvent.click(await screen.findByRole('button', { name: /send test event/i }));

    expect(await screen.findByTestId('webhook-test-result')).toHaveTextContent('200');
  });

  // A failed delivery is the diagnostic the operator asked for, not an error
  // state to hide.
  it('shows why a test fire failed', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(existing);
    vi.mocked(webhooksApi.test).mockResolvedValue({
      status_code: 502,
      error: 'webhook endpoint returned 502',
    });

    render(<WebhooksSection />);
    fireEvent.click(await screen.findByRole('button', { name: /send test event/i }));

    const result = await screen.findByTestId('webhook-test-result');
    expect(result).toHaveTextContent('502');
    expect(result).toHaveTextContent('webhook endpoint returned 502');
  });

  it('deletes the webhook and returns to the empty state', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(existing);
    vi.mocked(webhooksApi.remove).mockResolvedValue(undefined);

    render(<WebhooksSection />);
    fireEvent.click(await screen.findByRole('button', { name: /delete/i }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /create webhook/i })).toBeInTheDocument();
    });
    expect(webhooksApi.remove).toHaveBeenCalledWith(42);
  });

  it('surfaces a save failure without clearing the form', async () => {
    vi.mocked(webhooksApi.get).mockResolvedValue(null);
    vi.mocked(webhooksApi.create).mockRejectedValue({
      response: { data: { error: { detail: 'url must use https' } } },
    });

    render(<WebhooksSection />);
    fireEvent.change(await screen.findByLabelText(/endpoint url/i), {
      target: { value: 'http://example.com/hooks' },
    });
    fireEvent.click(screen.getByRole('button', { name: /create webhook/i }));

    expect(await screen.findByText('url must use https')).toBeInTheDocument();
    expect(screen.getByLabelText(/endpoint url/i)).toHaveValue('http://example.com/hooks');
  });
});
