import type { LucideIcon } from 'lucide-react';
import { Package, MapPinned, BarChart3, UserRound } from 'lucide-react';
import type { TabType } from '@/stores';

export interface SignedOutCopy {
  /** Heading — the surface's own name, matching its nav label. */
  title: string;
  /** One sentence on what the surface does. No claims beyond what ships. */
  blurb: string;
  icon: LucideIcon;
}

/**
 * What a signed-out visitor is told about the surface they asked for
 * (TRA-1057). Copy is fixed — do not embellish it and do not add claims.
 * Revisions are Tim's, via normal PR. Same convention as
 * `CAPABILITY_UPSELL_COPY` in `components/capability/registry.ts`.
 *
 * Only the ungated core paid surfaces get a pitch. Everything else falls back:
 * a returning user whose session expired needs a way back in, not a sales
 * page, and a first-time visitor should not be opened with advanced modules.
 */
/** `reports` and `reports-history` are one product surface with two routes. */
const REPORTS: SignedOutCopy = {
  title: 'Reports',
  blurb: 'See where every asset was last seen and how it moved between locations.',
  icon: BarChart3,
};

export const SIGNED_OUT_COPY: Partial<Record<TabType, SignedOutCopy>> = {
  assets: {
    title: 'Assets',
    blurb: 'Track every tagged item, its location, and its history.',
    icon: Package,
  },
  locations: {
    title: 'Locations',
    blurb: 'Organize sites, zones, and scan points so scans mean something.',
    icon: MapPinned,
  },
  reports: REPORTS,
  'reports-history': REPORTS,
};

/** Generic copy for every route without an entry above. Makes no claim. */
export const SIGNED_OUT_FALLBACK: SignedOutCopy = {
  title: 'Sign in to continue',
  blurb: 'This area needs a TrakRF account.',
  icon: UserRound,
};

export function signedOutCopyFor(route: TabType): SignedOutCopy {
  return SIGNED_OUT_COPY[route] ?? SIGNED_OUT_FALLBACK;
}
