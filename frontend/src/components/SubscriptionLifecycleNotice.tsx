import { useAuthStore, useOrgStore } from "@/stores";

interface SubscriptionLifecycleNoticeProps {
  isEntitled: boolean;
  subscriptionEnabled: boolean;
  subscriptionExpiresAt?: string | null;
}

// Approaching-expiry warnings start this close to the date; earlier than that a
// banner on every screen is noise. Grace and cutoff always show.
const WARN_WITHIN_MS = 14 * 24 * 60 * 60 * 1000;

function formatDate(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleDateString(undefined, {
    year: "numeric", month: "short", day: "numeric",
  });
}

// This is advisory UX only. The backend remains the enforcement point for
// scan persistence, fixed-reader capture, and webhook delivery.
export function SubscriptionLifecycleNotice({
  isEntitled,
  subscriptionEnabled,
  subscriptionExpiresAt,
}: SubscriptionLifecycleNoticeProps) {
  if (isEntitled && !subscriptionExpiresAt) return null;

  if (!isEntitled) {
    return (
      <div role="alert" className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800 dark:border-red-700/50 dark:bg-red-900/20 dark:text-red-200">
        <strong>Subscription cut off.</strong> Paid scan saves, fixed-reader capture, and webhook delivery are stopped. Fixed readers may still appear online, but their reads are not being recorded. Contact your administrator to reactivate service.
      </div>
    );
  }

  if (!subscriptionEnabled) return null;
  const msUntilExpiry = new Date(subscriptionExpiresAt!).getTime() - Date.now();
  if (msUntilExpiry <= 0) {
    return (
      <div role="status" className="rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-700/50 dark:bg-amber-900/20 dark:text-amber-100">
        <strong>Subscription grace period.</strong> Your subscription expired {formatDate(subscriptionExpiresAt!)}. Renew now to avoid capture and webhook delivery stopping.
      </div>
    );
  }
  if (!(msUntilExpiry <= WARN_WITHIN_MS)) return null;

  return (
    <div role="status" className="rounded-lg border border-blue-300 bg-blue-50 px-3 py-2 text-sm text-blue-900 dark:border-blue-700/50 dark:bg-blue-900/20 dark:text-blue-100">
      <strong>Subscription expires {formatDate(subscriptionExpiresAt!)}.</strong> Renew before expiry to avoid interruption to paid scan saves and fixed-reader capture.
    </div>
  );
}

// App-wide placement: the notice for the signed-in user's current org, shown
// above every screen so an expiring or cut-off org is never discovered at Save.
export function CurrentOrgSubscriptionNotice() {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const currentOrg = useOrgStore((state) => state.currentOrg);
  if (!isAuthenticated || !currentOrg) return null;
  return (
    <div className="px-2 pt-2 md:px-8 md:pt-4 bg-gray-50 dark:bg-gray-900 empty:hidden">
      <SubscriptionLifecycleNotice
        isEntitled={currentOrg.is_entitled}
        subscriptionEnabled={currentOrg.subscription_enabled}
        subscriptionExpiresAt={currentOrg.subscription_expires_at}
      />
    </div>
  );
}
