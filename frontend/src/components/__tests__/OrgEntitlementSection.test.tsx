import '@testing-library/jest-dom';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { OrgEntitlementSection } from '@/components/OrgEntitlementSection';
import { orgsApi } from '@/lib/api/orgs';

vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    updateEntitlement: vi.fn(),
  },
}));

vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

describe('OrgEntitlementSection', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('reflects the initial subscription_enabled state', () => {
    render(
      <OrgEntitlementSection orgId={7} initialEnabled={false} initialExpiresAt={null} />
    );
    const toggle = screen.getByLabelText(/subscription enabled/i) as HTMLInputElement;
    expect(toggle.checked).toBe(false);
  });

  it('saves the toggled enabled flag', async () => {
    vi.mocked(orgsApi.updateEntitlement).mockResolvedValueOnce({
      data: { data: {} },
    } as Awaited<ReturnType<typeof orgsApi.updateEntitlement>>);

    render(
      <OrgEntitlementSection orgId={7} initialEnabled={true} initialExpiresAt={null} />
    );
    const toggle = screen.getByLabelText(/subscription enabled/i) as HTMLInputElement;
    fireEvent.click(toggle); // now false
    fireEvent.click(screen.getByRole('button', { name: /save entitlement/i }));

    await waitFor(() => {
      expect(orgsApi.updateEntitlement).toHaveBeenCalledWith(
        7,
        expect.objectContaining({ subscription_enabled: false })
      );
    });
  });

  it('clears the expiry to null when the date field is emptied', async () => {
    vi.mocked(orgsApi.updateEntitlement).mockResolvedValueOnce({
      data: { data: {} },
    } as Awaited<ReturnType<typeof orgsApi.updateEntitlement>>);

    render(
      <OrgEntitlementSection
        orgId={7}
        initialEnabled={true}
        initialExpiresAt={'2999-01-01T00:00:00Z'}
      />
    );
    const expiry = screen.getByLabelText(/expires/i) as HTMLInputElement;
    fireEvent.change(expiry, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: /save entitlement/i }));

    await waitFor(() => {
      expect(orgsApi.updateEntitlement).toHaveBeenCalledWith(
        7,
        expect.objectContaining({ subscription_expires_at: null })
      );
    });
  });
});
