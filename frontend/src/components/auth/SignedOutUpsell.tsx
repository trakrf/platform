import { useEffect, useRef } from 'react';
import type { TabType } from '@/stores';
import { trackEvent } from '@/lib/analytics/track';
import { signedOutCopyFor } from './signedOutCopy';

interface SignedOutUpsellProps {
  /** The route the visitor asked for. Selects the copy and tags the events. */
  route: TabType;
}

/**
 * What a signed-out visitor sees on an org-scoped route (TRA-1057).
 *
 * Replaces the old wrapper's silent `#login` redirect: a card naming the
 * surface, one sentence on what it does, and both paths out — start a trial, or
 * log in if the session simply expired. The redirect told the visitor nothing
 * about their options; this does.
 *
 * No lock icon here, deliberately. A lock is the capability/entitlement
 * treatment and means "your organization did not buy this" — false for someone
 * who has no organization at all.
 */
export default function SignedOutUpsell({ route }: SignedOutUpsellProps) {
  const copy = signedOutCopyFor(route);
  const Icon = copy.icon;
  const shown = useRef(false);

  useEffect(() => {
    if (shown.current) return;
    shown.current = true;
    // Distinct from the paid_gate_* events on purpose: a signed-out visitor and
    // a lapsed subscriber are different funnel populations.
    trackEvent('signed_out_gate_shown', { surface: route });
  }, [route]);

  const onTrial = () => {
    trackEvent('signed_out_cta_click', { surface: route, cta: 'signup' });
    window.location.hash = '#signup';
  };

  const onLogin = () => {
    trackEvent('signed_out_cta_click', { surface: route, cta: 'login' });
    // Same key `handleAuthRedirect()` reads after a successful login, so the
    // visitor lands back where they were headed.
    sessionStorage.setItem('redirectAfterLogin', route);
    window.location.hash = '#login';
  };

  return (
    <div className="max-w-xl mx-auto mt-8" data-testid={`signed-out-upsell-${route}`}>
      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6">
        <div className="flex items-center mb-3">
          <Icon className="w-5 h-5 mr-2 text-gray-500 dark:text-gray-400" aria-hidden="true" />
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{copy.title}</h2>
        </div>

        <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">{copy.blurb}</p>

        <div className="flex flex-wrap items-center gap-3">
          <button
            type="button"
            onClick={onTrial}
            data-testid="signed-out-trial"
            className="rounded-md bg-blue-600 hover:bg-blue-700 px-4 py-2 text-sm font-medium text-white"
          >
            Start free trial
          </button>
          <button
            type="button"
            onClick={onLogin}
            data-testid="signed-out-login"
            className="text-sm font-medium text-blue-600 dark:text-blue-400 hover:underline"
          >
            Already have an account? Log in
          </button>
        </div>
      </div>
    </div>
  );
}
