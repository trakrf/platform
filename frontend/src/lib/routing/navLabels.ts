import type { TabType } from '@/stores';

/**
 * Sidebar labels for surfaces whose screen owns its own page heading.
 *
 * Most tabs are titled by `PAGE_TITLES` in Header and print nothing themselves.
 * These three have no PAGE_TITLES entry, so their in-screen heading *is* the page
 * identity — and it has to read as the sidebar entry the user clicked to get
 * there. Both sides import from here so they cannot be renamed apart (TRA-1071);
 * `pageTitleNavParity.test.tsx` pins these values to the rendered sidebar.
 *
 * Names chosen 2026-07-30: "Readers" over "Scan Devices" (technically correct,
 * but nobody says it), "Live Reads" over "Live feed" (matches how the surface is
 * named everywhere else), "Outputs" over "Output Devices".
 *
 * This is not a general nav-label registry. Capability-gated entries get their
 * labels from `components/capability/registry.ts`, which stays the source of
 * truth for those — `output-devices` appears here because its *screen* needs the
 * string, and the value is kept equal to the registry entry by the parity test.
 */
export const NAV_LABELS = {
  'scan-devices': 'Readers',
  'live-reads': 'Live Reads',
  'output-devices': 'Outputs',
} as const satisfies Partial<Record<TabType, string>>;
