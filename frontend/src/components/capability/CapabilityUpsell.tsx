import { Lock } from 'lucide-react';
import { SUPPORT_EMAIL } from '@/components/entitlement/gateCopy';
import { CAPABILITY_UPSELL_COPY } from './registry';

interface CapabilityUpsellProps {
  /** The ungated capability whose surface was requested. */
  capability: string;
  /** Registry label, used as the heading when the capability has no copy entry. */
  label: string;
}

/**
 * The one upsell view, parameterized by capability (TRA-1026 / ADR 0002).
 * Rendered in place of a `locked` capability's surface for an org without the
 * grant, so the real surface's chunk is never fetched.
 *
 * Copy is fixed by the ticket. Do not embellish it and do not add capability
 * claims here — a capability with no copy entry gets its heading and the
 * contact line, nothing invented.
 */
export default function CapabilityUpsell({ capability, label }: CapabilityUpsellProps) {
  const copy = CAPABILITY_UPSELL_COPY[capability];
  const title = copy?.title ?? label;
  const mailto = `mailto:${SUPPORT_EMAIL}?subject=${encodeURIComponent(`Enable ${title} for my TrakRF organization`)}`;

  return (
    <div className="max-w-xl mx-auto mt-8" data-testid={`capability-upsell-${capability}`}>
      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6">
        <div className="flex items-center mb-3">
          <Lock className="w-5 h-5 mr-2 text-gray-500 dark:text-gray-400" aria-hidden="true" />
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{title}</h2>
        </div>

        {copy && (
          <p className="text-sm text-gray-600 dark:text-gray-400 mb-3">{copy.blurb}</p>
        )}

        <p className="text-sm text-gray-600 dark:text-gray-400">
          This feature isn&apos;t enabled for your organization. Contact us to enable it:{' '}
          <a
            href={mailto}
            className="text-blue-600 dark:text-blue-400 hover:underline"
            data-testid="capability-upsell-contact"
          >
            {SUPPORT_EMAIL}
          </a>
        </p>
      </div>
    </div>
  );
}
