import type { GateCopy } from './gateCopy';

interface UpsellPopoverProps {
  copy: GateCopy;
  onCta: () => void;
  /** Positioning classes for the popover container (defaults to anchored below-right). */
  className?: string;
}

/**
 * Presentational upsell prompt used by both `<PaidGate>` and `<GatedFab>` (TRA-948).
 * Shows a short value-led message and a single CTA.
 */
export function UpsellPopover({ copy, onCta, className = 'absolute right-0 top-full mt-1' }: UpsellPopoverProps) {
  return (
    <div
      role="dialog"
      className={`z-20 w-64 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-3 shadow-lg ${className}`}
    >
      <p className="text-sm text-gray-700 dark:text-gray-200">{copy.message}</p>
      <button
        type="button"
        onClick={onCta}
        className="mt-2 w-full rounded-md bg-blue-600 hover:bg-blue-700 px-3 py-1.5 text-sm font-medium text-white"
      >
        {copy.ctaLabel}
      </button>
    </div>
  );
}
