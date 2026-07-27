import type { EntitlementState } from '@/hooks/entitlement/useEntitlement';

export type GateAction = 'signup' | 'contact';

export interface GateCopy {
  message: string;
  ctaLabel: string;
  action: GateAction;
}

/** The one support address the app hands out. Every contact CTA points here. */
export const SUPPORT_EMAIL = 'support@trakrf.id';

/** Where the lapsed "contact us" CTA points (no self-serve Stripe yet). */
export const RENEW_CONTACT_MAILTO = `mailto:${SUPPORT_EMAIL}?subject=Renew%20TrakRF%20subscription`;

/** Value-led (not feature-led) prompt copy per state. `entitled` never renders a prompt. */
export function gateCopy(state: EntitlementState): GateCopy {
  if (state === 'logged-out') {
    return {
      message: 'Start a free trial to save scans and manage your assets.',
      ctaLabel: 'Start free trial',
      action: 'signup',
    };
  }
  return {
    message: 'Your subscription has lapsed. Renew to edit and manage your assets.',
    ctaLabel: 'Contact us to renew',
    action: 'contact',
  };
}
