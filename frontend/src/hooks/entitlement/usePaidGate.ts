import { useEffect, useRef, useState } from 'react';
import { useEntitlement } from './useEntitlement';
import { trackEvent } from '@/lib/analytics/track';
import { gateCopy, RENEW_CONTACT_MAILTO, type GateCopy } from '@/components/entitlement/gateCopy';

export interface PaidGateController {
  /** state !== 'entitled' — render the locked treatment. */
  locked: boolean;
  /** Prompt copy + CTA for the current state (meaningful only when locked). */
  copy: GateCopy;
  /** Whether the upsell prompt is currently open. */
  open: boolean;
  /** Click handler for a locked affordance: emits the click event and toggles the prompt. */
  onGatedClick: () => void;
  /** CTA handler: emits cta_click and performs the signup/contact action. */
  onCta: () => void;
  /** Close the prompt. */
  close: () => void;
}

/**
 * Shared interactive logic for the TRA-948 paid gate. Consumed by both the
 * wrapping `<PaidGate>` primitive and the fixed-position `<GatedFab>`, so every
 * gated affordance fires the same funnel events and runs the same CTA action.
 */
export function usePaidGate(
  surface: string,
  opts: { silentImpression?: boolean } = {}
): PaidGateController {
  const { state, isLocked } = useEntitlement();
  const [open, setOpen] = useState(false);
  const shown = useRef(false);

  useEffect(() => {
    if (isLocked && !shown.current && !opts.silentImpression) {
      shown.current = true;
      trackEvent('paid_gate_prompt_shown', { surface, state });
    }
  }, [isLocked, surface, state, opts.silentImpression]);

  const copy = gateCopy(state);

  const onGatedClick = () => {
    trackEvent('paid_gate_click', { surface, state });
    setOpen((v) => !v);
  };

  const onCta = () => {
    trackEvent('paid_gate_cta_click', { surface, state });
    if (copy.action === 'signup') {
      window.location.hash = '#signup';
    } else {
      window.location.href = RENEW_CONTACT_MAILTO;
    }
    setOpen(false);
  };

  return { locked: isLocked, copy, open, onGatedClick, onCta, close: () => setOpen(false) };
}
