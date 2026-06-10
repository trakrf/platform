import { render, screen, fireEvent } from '@testing-library/react';
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
    render(
      <PaidGate surface="assets-crud">
        <button onClick={onClick}>Add</button>
      </PaidGate>
    );
    fireEvent.click(screen.getByText('Add'));
    expect(onClick).toHaveBeenCalledTimes(1);
    expect(trackEvent).not.toHaveBeenCalled();
  });

  it('locked: emits prompt_shown on mount and gates the child click', () => {
    setState('lapsed');
    const onClick = vi.fn();
    render(
      <PaidGate surface="assets-crud">
        <button onClick={onClick}>Add</button>
      </PaidGate>
    );
    expect(trackEvent).toHaveBeenCalledWith('paid_gate_prompt_shown', {
      surface: 'assets-crud',
      state: 'lapsed',
    });
    fireEvent.click(screen.getByTestId('paid-gate-overlay'));
    expect(onClick).not.toHaveBeenCalled();
    expect(trackEvent).toHaveBeenCalledWith('paid_gate_click', {
      surface: 'assets-crud',
      state: 'lapsed',
    });
    expect(screen.getByRole('dialog').textContent).toMatch(/renew to edit/i);
  });

  it('logged-out CTA fires cta_click and redirects to signup', () => {
    setState('logged-out');
    render(
      <PaidGate surface="inventory-save">
        <button>Save</button>
      </PaidGate>
    );
    fireEvent.click(screen.getByTestId('paid-gate-overlay'));
    fireEvent.click(screen.getByRole('button', { name: 'Start free trial' }));
    expect(trackEvent).toHaveBeenCalledWith('paid_gate_cta_click', {
      surface: 'inventory-save',
      state: 'logged-out',
    });
    expect(window.location.hash).toBe('#signup');
  });
});
