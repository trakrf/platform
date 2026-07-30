import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, cleanup } from '@testing-library/react';

/**
 * A tab's page title must read the same as the sidebar entry you clicked to get
 * there (TRA-1071). The Scan-tab rename left `settings` titled "Device Setup"
 * under a nav item reading "Settings", and Locate printing its own "Find Item"
 * heading under a header reading "Locate".
 *
 * Nav labels are read out of a rendered TabNavigation rather than a hardcoded
 * list, so renaming one side and forgetting the other goes red here.
 *
 * Gate stubbed rather than satisfied with a granted org so this file never sets
 * `authStore.isAuthenticated` — see the note in HelpScreen.test.tsx and TRA-1079.
 */
vi.mock('@/hooks/capability/useCapability', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/hooks/capability/useCapability')>()),
  useCapabilityNavGate: () => 'visible' as const,
}));

import TabNavigation from '@/components/TabNavigation';
import { PAGE_TITLES } from '@/components/Header';
import { useUIStore, useDeviceStore, useOrgStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';

/**
 * Tabs whose screen owns its own heading instead of appearing in PAGE_TITLES.
 * Listed explicitly so adding a nav entry without a page title is a decision
 * someone made here, not a blank header nobody noticed.
 */
const SCREEN_OWNS_HEADING = new Set([
  'kits',
  'mustering',
  'scan-devices',
  'live-reads',
  'output-devices',
  'org-geofence-defaults',
]);

/** tab id -> sidebar label, for every entry the sidebar can show. */
function navLabels(): Map<string, string> {
  useOrgStore.setState({ currentRole: 'owner' } as never);

  const { container } = render(<TabNavigation />);
  const entries = Array.from(container.querySelectorAll('[data-testid^="menu-item-"]'))
    .map((el): [string, string] => [
      el.getAttribute('data-testid')!.replace(/^menu-item-/, ''),
      el.textContent?.trim() ?? '',
    ])
    // The lock affordance renders its own testid inside the button.
    .filter(([id, label]) => !id.endsWith('-locked') && label);

  return new Map(entries);
}

describe('page title / nav label parity', () => {
  beforeEach(() => {
    useUIStore.setState({ activeTab: 'scan' });
    useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });
  });

  afterEach(() => {
    cleanup();
    useOrgStore.setState({ currentRole: null } as never);
  });

  it('titles each tab the same as the sidebar entry that opens it', () => {
    const labels = navLabels();
    const checked: string[] = [];

    labels.forEach((label, id) => {
      const page = PAGE_TITLES[id as keyof typeof PAGE_TITLES];
      if (!page) return;
      checked.push(id);
      expect(page.title, `page title for "${id}" should match its nav label`).toBe(label);
    });

    // Guard against the assertion loop silently covering nothing.
    expect(checked.length).toBeGreaterThan(5);
  });

  it('accounts for every sidebar entry, by title or by an owned heading', () => {
    const labels = navLabels();

    labels.forEach((_label, id) => {
      const accounted =
        id in PAGE_TITLES || SCREEN_OWNS_HEADING.has(id);
      expect(accounted, `nav entry "${id}" has no page title and is not listed as owning one`).toBe(
        true
      );
    });
  });
});
