import { useEffect, useState, useRef } from 'react';
import { useEntitlement } from '@/hooks/entitlement/useEntitlement';
import { trackEvent } from '@/lib/analytics/track';
import { gateCopy, RENEW_CONTACT_MAILTO } from './gateCopy';

interface PaidGateProps {
  /** Stable tag for funnel events, e.g. "assets-crud". */
  surface: string;
  /** Center the prompt over a large region (whole-panel gating) instead of a control. */
  panel?: boolean;
  className?: string;
  children: React.ReactNode;
}

/**
 * TRA-948 single gating primitive. Entitled → transparent pass-through. Locked
 * (logged-out / lapsed) → children rendered grayed + inert with a click-capturing
 * overlay that opens a value-led upsell prompt. Funnel events fire per surface+state.
 */
export function PaidGate({ surface, panel = false, className = '', children }: PaidGateProps) {
  const { state, isLocked } = useEntitlement();
  const [open, setOpen] = useState(false);
  const shown = useRef(false);

  useEffect(() => {
    if (isLocked && !shown.current) {
      shown.current = true;
      trackEvent('paid_gate_prompt_shown', { surface, state });
    }
  }, [isLocked, surface, state]);

  if (!isLocked) return <>{children}</>;

  const copy = gateCopy(state);

  const handleOverlay = () => {
    trackEvent('paid_gate_click', { surface, state });
    setOpen((v) => !v);
  };

  const handleCta = () => {
    trackEvent('paid_gate_cta_click', { surface, state });
    if (copy.action === 'signup') {
      window.location.hash = '#signup';
    } else {
      window.location.href = RENEW_CONTACT_MAILTO;
    }
    setOpen(false);
  };

  return (
    <div className={`relative ${panel ? 'block w-full' : 'inline-block'} ${className}`}>
      <div className="opacity-50 pointer-events-none select-none" aria-disabled="true">
        {children}
      </div>
      <button
        type="button"
        data-testid="paid-gate-overlay"
        onClick={handleOverlay}
        aria-label="Locked feature — show upgrade options"
        className={`absolute inset-0 z-10 cursor-help bg-transparent ${
          panel ? 'flex items-center justify-center' : ''
        }`}
      />
      {open && (
        <div
          role="dialog"
          className="absolute right-0 top-full z-20 mt-1 w-64 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-3 shadow-lg"
        >
          <p className="text-sm text-gray-700 dark:text-gray-200">{copy.message}</p>
          <button
            type="button"
            onClick={handleCta}
            className="mt-2 w-full rounded-md bg-blue-600 hover:bg-blue-700 px-3 py-1.5 text-sm font-medium text-white"
          >
            {copy.ctaLabel}
          </button>
        </div>
      )}
    </div>
  );
}
