import type { TabType } from '@/stores';

/** The screen shown after login and when the hash is empty or unknown. */
export const DEFAULT_TAB: TabType = 'scan';

/**
 * Legacy tab ids that no longer have their own screen. Old bookmarks
 * (#home, #inventory, #barcode) resolve to the Scan tab (TRA-1029).
 */
export const LEGACY_TAB_REDIRECTS: Record<string, TabType> = {
  home: 'scan',
  inventory: 'scan',
  barcode: 'scan',
};

/** Resolve a raw hash tab id: legacy ids map to their successor, others pass through. */
export function resolveLegacyTab(tab: string): string {
  return LEGACY_TAB_REDIRECTS[tab] ?? tab;
}

/** Whether a raw hash tab id is a retired/legacy id that should be rewritten. */
export function isLegacyTab(tab: string): boolean {
  return Object.prototype.hasOwnProperty.call(LEGACY_TAB_REDIRECTS, tab);
}
