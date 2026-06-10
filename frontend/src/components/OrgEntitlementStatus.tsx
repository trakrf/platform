/**
 * OrgEntitlementStatus — read-only subscription/trial status for org admins (TRA-975).
 *
 * Distinct from the superadmin entitlement EDITOR (TRA-949, OrgEntitlementSection):
 * this is a non-editable badge so an org's own admins can see their trial/expiry
 * before paid surfaces start locking. Data comes from /users/me's current org
 * (is_entitled + subscription_expires_at, surfaced in TRA-947).
 *
 * "Plan" is intentionally absent — plan tiers are out of scope for lite.
 */

type Tone = "active" | "trial" | "expired";

interface StatusView {
  label: string;
  tone: Tone;
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

/**
 * Derives the display status. is_entitled is computed server-side as
 * "enabled AND (no expiry OR now < expiry)", so an entitled org's expiry is
 * always null or in the future. A not-entitled org is either expired (a past
 * date) or manually disabled (no/irrelevant date).
 */
export function entitlementStatus(
  isEntitled: boolean,
  subscriptionExpiresAt: string | null | undefined,
  now: number = Date.now(),
): StatusView {
  if (isEntitled) {
    if (subscriptionExpiresAt) {
      return {
        label: `Trial — expires ${formatDate(subscriptionExpiresAt)}`,
        tone: "trial",
      };
    }
    return { label: "Active", tone: "active" };
  }
  if (
    subscriptionExpiresAt &&
    new Date(subscriptionExpiresAt).getTime() < now
  ) {
    return {
      label: `Expired ${formatDate(subscriptionExpiresAt)}`,
      tone: "expired",
    };
  }
  return { label: "Inactive", tone: "expired" };
}

const toneClass: Record<Tone, string> = {
  active: "bg-green-900/30 text-green-300 border border-green-700/50",
  trial: "bg-blue-900/30 text-blue-300 border border-blue-700/50",
  expired: "bg-red-900/30 text-red-300 border border-red-700/50",
};

interface OrgEntitlementStatusProps {
  isEntitled: boolean;
  subscriptionExpiresAt?: string | null;
}

export function OrgEntitlementStatus({
  isEntitled,
  subscriptionExpiresAt,
}: OrgEntitlementStatusProps) {
  const { label, tone } = entitlementStatus(isEntitled, subscriptionExpiresAt);
  return (
    <div>
      <span className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
        Subscription
      </span>
      <span
        aria-label="Subscription status"
        className={`inline-flex items-center rounded-full px-3 py-1 text-sm font-medium ${toneClass[tone]}`}
      >
        {label}
      </span>
    </div>
  );
}
