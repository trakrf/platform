interface SubscriptionLifecycleNoticeProps {
  isEntitled: boolean;
  subscriptionEnabled: boolean;
  subscriptionExpiresAt?: string | null;
}

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
      <div role="alert" className="rounded-lg border border-red-700/50 bg-red-900/20 px-3 py-2 text-sm text-red-200">
        <strong>Subscription cut off.</strong> Paid scan saves, fixed-reader capture, and webhook delivery are stopped. Contact your administrator to reactivate service.
      </div>
    );
  }

  if (!subscriptionEnabled) return null;
  const expired = subscriptionExpiresAt && new Date(subscriptionExpiresAt).getTime() <= Date.now();
  if (expired) {
    return (
      <div role="status" className="rounded-lg border border-amber-700/50 bg-amber-900/20 px-3 py-2 text-sm text-amber-100">
        <strong>Subscription grace period.</strong> Your subscription expired {formatDate(subscriptionExpiresAt!)}. Renew now to avoid capture and webhook delivery stopping.
      </div>
    );
  }

  return (
    <div role="status" className="rounded-lg border border-blue-700/50 bg-blue-900/20 px-3 py-2 text-sm text-blue-100">
      <strong>Subscription expires {formatDate(subscriptionExpiresAt!)}.</strong> Renew before expiry to avoid interruption to paid scan saves and fixed-reader capture.
    </div>
  );
}
