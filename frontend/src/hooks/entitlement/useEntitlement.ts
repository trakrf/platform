import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';

export type EntitlementState = 'logged-out' | 'entitled' | 'lapsed';

export interface Entitlement {
  state: EntitlementState;
  /** state === 'entitled' */
  isEntitled: boolean;
  /** state !== 'entitled' — the gate should render the locked treatment. */
  isLocked: boolean;
  subscriptionExpiresAt: string | null;
}

/**
 * Single source of truth for the three-state paid gating (TRA-948).
 * Derives from auth state + the TRA-947 `current_org.is_entitled` signal.
 */
export function useEntitlement(): Entitlement {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const currentOrg = useOrgStore((s) => s.currentOrg);

  if (!isAuthenticated) {
    return { state: 'logged-out', isEntitled: false, isLocked: true, subscriptionExpiresAt: null };
  }
  if (currentOrg?.is_entitled) {
    return {
      state: 'entitled',
      isEntitled: true,
      isLocked: false,
      subscriptionExpiresAt: currentOrg.subscription_expires_at ?? null,
    };
  }
  return {
    state: 'lapsed',
    isEntitled: false,
    isLocked: true,
    subscriptionExpiresAt: currentOrg?.subscription_expires_at ?? null,
  };
}
