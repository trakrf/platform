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
  // Authenticated but the org/profile hasn't loaded yet (currentOrg === null): fail
  // OPEN so entitled users don't see a flash of grayed controls on every page load.
  // The backend enforces entitlement regardless, so optimistic-unlock is safe; we only
  // ever show the locked treatment once we positively know is_entitled === false.
  if (!currentOrg) {
    return { state: 'entitled', isEntitled: true, isLocked: false, subscriptionExpiresAt: null };
  }
  if (currentOrg.is_entitled) {
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
    subscriptionExpiresAt: currentOrg.subscription_expires_at ?? null,
  };
}
