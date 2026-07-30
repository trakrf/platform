import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, fireEvent, cleanup, within } from '@testing-library/react';

/**
 * Help is the one surface a confused user reaches for, so a nav name it tells
 * them to click has to be a nav name that exists (TRA-1071). These tests read
 * the sidebar rather than a hardcoded list, so a future rename that misses
 * HelpScreen.tsx goes red here instead of shipping.
 *
 * The capability gate is forced open rather than satisfied with a granted org,
 * so this file never sets `authStore.isAuthenticated`. Flipping that wakes
 * tagStore's auth subscription, which flushes its lookup queue against
 * whatever tags an earlier test file left behind and issues an XHR — the
 * TRA-1050 cross-file leak. Labels still come from the real registry; only the
 * grant answer is stubbed, and grants are TabNavigation's tests to own.
 */
vi.mock('@/hooks/capability/useCapability', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/hooks/capability/useCapability')>()),
  useCapabilityNavGate: () => 'visible' as const,
}));

import HelpScreen from '@/components/HelpScreen';
import TabNavigation from '@/components/TabNavigation';
import { useUIStore, useDeviceStore, useOrgStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';

/** Every nav label the sidebar can show — owner role, every gated entry open. */
function renderFullSidebar() {
  useOrgStore.setState({ currentRole: 'owner' } as never);

  const { container } = render(<TabNavigation />);
  return Array.from(container.querySelectorAll('[data-testid^="menu-item-"]'))
    .map((el) => el.textContent?.trim() ?? '')
    .filter(Boolean);
}

/** Full Help text with every accordion expanded. */
function renderHelpText() {
  const { container } = render(<HelpScreen />);
  within(container)
    .getAllByRole('button')
    .forEach((button) => fireEvent.click(button));
  return container.textContent ?? '';
}

describe('HelpScreen nav vocabulary', () => {
  beforeEach(() => {
    useUIStore.setState({ activeTab: 'help' });
    useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });
  });

  afterEach(() => {
    cleanup();
    useOrgStore.setState({ currentRole: null } as never);
  });

  it('only tells users to click nav items that exist in the sidebar', () => {
    const navLabels = renderFullSidebar();
    const helpText = renderHelpText();

    const referenced = [...helpText.matchAll(/Click "([^"]+)" on the left/g)].map((m) => m[1]);

    expect(referenced.length).toBeGreaterThan(0);
    expect(navLabels.length).toBeGreaterThan(0);
    referenced.forEach((name) => {
      expect(navLabels).toContain(name);
    });
  });

  it('does not name controls the app no longer has', () => {
    const helpText = renderHelpText();

    // "My Items" was the pre-rename Scan tab; "Buzzer Volume" was a slider that
    // no longer exists anywhere in the app — sound is a toolbar toggle now.
    expect(helpText).not.toMatch(/My Items/);
    expect(helpText).not.toMatch(/Buzzer Volume/);
  });
});
