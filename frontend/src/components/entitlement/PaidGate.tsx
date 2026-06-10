import { usePaidGate } from '@/hooks/entitlement/usePaidGate';
import { UpsellPopover } from './UpsellPopover';

interface PaidGateProps {
  /** Stable tag for funnel events, e.g. "assets-crud". */
  surface: string;
  /** Center the prompt over a large region (whole-panel gating) instead of a control. */
  panel?: boolean;
  /**
   * Suppress the `paid_gate_prompt_shown` funnel event. Use on secondary / row-level
   * gates so a screen fires one impression (from its primary CTA) instead of one per
   * row. The gray + prompt UX is identical; only the impression event is silenced.
   */
  silentImpression?: boolean;
  className?: string;
  children: React.ReactNode;
}

/**
 * TRA-948 single gating primitive. Entitled → transparent pass-through. Locked
 * (logged-out / lapsed) → children rendered grayed + inert with a click-capturing
 * overlay that opens a value-led upsell prompt. Funnel events fire per surface+state.
 *
 * For fixed-position controls (FABs) use `<GatedFab>`, which shares the same logic.
 */
export function PaidGate({
  surface,
  panel = false,
  silentImpression = false,
  className = '',
  children,
}: PaidGateProps) {
  const { locked, open, copy, onGatedClick, onCta } = usePaidGate(surface, { silentImpression });

  if (!locked) return <>{children}</>;

  return (
    <div className={`relative ${panel ? 'block w-full' : 'inline-block'} ${className}`}>
      <div className="opacity-50 pointer-events-none select-none" aria-disabled="true">
        {children}
      </div>
      <button
        type="button"
        data-testid="paid-gate-overlay"
        onClick={onGatedClick}
        aria-label="Locked feature — show upgrade options"
        className={`absolute inset-0 z-10 cursor-help bg-transparent ${
          panel ? 'flex items-center justify-center' : ''
        }`}
      />
      {open && <UpsellPopover copy={copy} onCta={onCta} />}
    </div>
  );
}
