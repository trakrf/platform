import '@testing-library/jest-dom';
import { StrictMode } from 'react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';

const tracked = vi.hoisted(() => ({ calls: [] as Array<[string, Record<string, unknown>]> }));
vi.mock('@/lib/analytics/track', () => ({
  trackEvent: (name: string, props: Record<string, unknown> = {}) => {
    tracked.calls.push([name, props]);
  },
}));

import SignedOutUpsell from './SignedOutUpsell';

describe('SignedOutUpsell', () => {
  beforeEach(() => {
    tracked.calls = [];
    sessionStorage.clear();
    window.location.hash = '';
  });

  afterEach(cleanup);

  it('renders the registry pitch for a core surface', () => {
    render(<SignedOutUpsell route="reports" />);

    expect(screen.getByText('Reports')).toBeInTheDocument();
    expect(
      screen.getByText('See where every asset was last seen and how it moved between locations.')
    ).toBeInTheDocument();
  });

  it('renders generic copy for a surface with no entry', () => {
    render(<SignedOutUpsell route="mustering" />);

    expect(screen.getByText('Sign in to continue')).toBeInTheDocument();
    expect(screen.getByText('This area needs a TrakRF account.')).toBeInTheDocument();
  });

  it('sends the trial CTA to signup', () => {
    render(<SignedOutUpsell route="assets" />);

    fireEvent.click(screen.getByTestId('signed-out-trial'));

    expect(window.location.hash).toBe('#signup');
    expect(tracked.calls).toContainEqual([
      'signed_out_cta_click',
      { surface: 'assets', cta: 'signup' },
    ]);
  });

  it('stashes the current route before sending the login CTA to login', () => {
    window.location.hash = '#locations';
    render(<SignedOutUpsell route="locations" />);

    fireEvent.click(screen.getByTestId('signed-out-login'));

    expect(sessionStorage.getItem('redirectAfterLogin')).toBe('locations');
    expect(window.location.hash).toBe('#login');
    expect(tracked.calls).toContainEqual([
      'signed_out_cta_click',
      { surface: 'locations', cta: 'login' },
    ]);
  });

  it('fires one impression event per mount', () => {
    render(<SignedOutUpsell route="reports" />);

    expect(tracked.calls.filter(([name]) => name === 'signed_out_gate_shown')).toEqual([
      ['signed_out_gate_shown', { surface: 'reports' }],
    ]);
  });

  it('fires a new impression event per distinct route on the SAME instance', () => {
    // App.tsx renders <SignedOutUpsell route={activeTab} /> at a stable tree
    // position with no `key`, so React reuses this instance across route
    // changes rather than remounting. A signed-out visitor clicking
    // Assets -> Locations -> Reports must produce three impression events,
    // one per surface, not one for whichever route happened to mount first.
    const { rerender } = render(<SignedOutUpsell route="assets" />);
    rerender(<SignedOutUpsell route="locations" />);
    rerender(<SignedOutUpsell route="reports" />);
    // Re-rendering with the same route again (e.g. a parent re-render that
    // doesn't change the tab) must not re-fire for a route already counted.
    rerender(<SignedOutUpsell route="reports" />);

    expect(tracked.calls.filter(([name]) => name === 'signed_out_gate_shown')).toEqual([
      ['signed_out_gate_shown', { surface: 'assets' }],
      ['signed_out_gate_shown', { surface: 'locations' }],
      ['signed_out_gate_shown', { surface: 'reports' }],
    ]);
  });

  it('does not double-fire the same route under StrictMode double-invoke', () => {
    render(
      <StrictMode>
        <SignedOutUpsell route="assets" />
      </StrictMode>
    );

    expect(tracked.calls.filter(([name]) => name === 'signed_out_gate_shown')).toEqual([
      ['signed_out_gate_shown', { surface: 'assets' }],
    ]);
  });

  it('offers signup and login, never the capability "contact us" treatment', () => {
    // Guards against someone pasting CapabilityUpsell's mailto copy in here:
    // "contact us to enable it for your organization" is the wrong answer for a
    // visitor who has no organization.
    const { container } = render(<SignedOutUpsell route="reports" />);

    expect(container.querySelector('a[href^="mailto:"]')).toBeNull();
    expect(screen.queryByText(/your organization/i)).not.toBeInTheDocument();
  });
});
