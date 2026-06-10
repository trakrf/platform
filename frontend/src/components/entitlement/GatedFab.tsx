import { FloatingActionButton, type FloatingActionButtonProps } from '@/components/shared/buttons/FloatingActionButton';
import { usePaidGate } from '@/hooks/entitlement/usePaidGate';
import { UpsellPopover } from './UpsellPopover';

interface GatedFabProps extends FloatingActionButtonProps {
  /** Stable tag for funnel events, e.g. "assets-crud". */
  surface: string;
}

/**
 * TRA-948 gate for fixed-position floating action buttons. A `<PaidGate>` overlay
 * can't size to a `position: fixed` child, so the FAB is gated imperatively via the
 * shared `usePaidGate` logic: entitled → normal FAB; locked → grayed FAB whose click
 * opens the same upsell prompt (anchored above the button) and fires the same events.
 */
export function GatedFab({ surface, onClick, className = '', ...fabProps }: GatedFabProps) {
  const { locked, open, copy, onGatedClick, onCta } = usePaidGate(surface);

  if (!locked) {
    return <FloatingActionButton onClick={onClick} className={className} {...fabProps} />;
  }

  return (
    <>
      <FloatingActionButton
        onClick={onGatedClick}
        className={`opacity-50 cursor-help ${className}`}
        {...fabProps}
      />
      {open && (
        <UpsellPopover copy={copy} onCta={onCta} className="fixed bottom-24 right-6 z-50" />
      )}
    </>
  );
}
