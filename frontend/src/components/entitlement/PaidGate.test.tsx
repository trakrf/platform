import { render, fireEvent, within } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { PaidGate } from './PaidGate';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';

const trackEvent = vi.fn();
vi.mock('@/lib/analytics/track', () => ({ trackEvent: (...a: unknown[]) => trackEvent(...a) }));

function setState(kind: 'logged-out' | 'entitled' | 'lapsed') {
  useAuthStore.setState({ isAuthenticated: kind !== 'logged-out' } as never);
  useOrgStore.setState({
    currentOrg:
      kind === 'logged-out'
        ? null
        : ({
            id: 1,
            name: 'Acme',
            identifier: 'acme',
            role: 'owner',
            is_entitled: kind === 'entitled',
            subscription_enabled: true,
            subscription_expires_at: null,
          } as never),
  } as never);
}

describe('PaidGate', () => {
  beforeEach(() => {
    trackEvent.mockClear();
    window.location.hash = '';
  });

  it('entitled: renders children as an interactive pass-through, no prompt event', () => {
    setState('entitled');
    const onClick = vi.fn();
    const { container } = render(
      <PaidGate surface="assets-crud">
        <button onClick={onClick}>Add</button>
      </PaidGate>
    );
    fireEvent.click(within(container).getByText('Add'));
    expect(onClick).toHaveBeenCalledTimes(1);
    expect(trackEvent).not.toHaveBeenCalled();
    expect(within(container).queryByTestId('paid-gate-overlay')).toBeNull();
  });

  it('locked: emits prompt_shown on mount and gates the child click', () => {
    setState('lapsed');
    const onClick = vi.fn();
    const { container } = render(
      <PaidGate surface="assets-crud">
        <button onClick={onClick}>Add</button>
      </PaidGate>
    );
    expect(trackEvent).toHaveBeenCalledWith('paid_gate_prompt_shown', {
      surface: 'assets-crud',
      state: 'lapsed',
    });
    fireEvent.click(within(container).getByTestId('paid-gate-overlay'));
    expect(onClick).not.toHaveBeenCalled();
    expect(trackEvent).toHaveBeenCalledWith('paid_gate_click', {
      surface: 'assets-crud',
      state: 'lapsed',
    });
    expect(within(container).getByRole('dialog').textContent).toMatch(/renew to edit/i);
  });

  it('silentImpression suppresses prompt_shown but still gates the click', () => {
    setState('lapsed');
    const onClick = vi.fn();
    const { container } = render(
      <PaidGate surface="assets-crud" silentImpression>
        <button onClick={onClick}>Edit</button>
      </PaidGate>
    );
    expect(trackEvent).not.toHaveBeenCalledWith('paid_gate_prompt_shown', expect.anything());
    fireEvent.click(within(container).getByTestId('paid-gate-overlay'));
    expect(onClick).not.toHaveBeenCalled();
    expect(trackEvent).toHaveBeenCalledWith('paid_gate_click', {
      surface: 'assets-crud',
      state: 'lapsed',
    });
  });

  it('logged-out CTA fires cta_click and redirects to signup', () => {
    setState('logged-out');
    const { container } = render(
      <PaidGate surface="inventory-save">
        <button>Save</button>
      </PaidGate>
    );
    fireEvent.click(within(container).getByTestId('paid-gate-overlay'));
    fireEvent.click(within(container).getByRole('button', { name: 'Start free trial' }));
    expect(trackEvent).toHaveBeenCalledWith('paid_gate_cta_click', {
      surface: 'inventory-save',
      state: 'logged-out',
    });
    expect(window.location.hash).toBe('#signup');
  });
});
