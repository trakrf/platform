/**
 * useOrgSwitch - Orchestrates org switching and creation with cache invalidation
 *
 * Handles:
 * - Calling orgStore.switchOrg() to update backend
 * - Creating new orgs with proper JWT token setup
 * - Cache invalidation via central orgScopedCache
 *
 * This follows React best practices by keeping the queryClient access
 * within React context via useQueryClient() hook.
 */
import { useQueryClient } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { useAuthStore } from '@/stores/authStore';
import { orgsApi } from '@/lib/api/orgs';
import { invalidateAllOrgScopedData } from '@/lib/cache/orgScopedCache';

export function useOrgSwitch() {
  const queryClient = useQueryClient();
  const { switchOrg: storeSwitchOrg, createOrg: storeCreateOrg, isLoading } = useOrgStore();

  const switchOrg = async (orgId: number) => {
    // storeSwitchOrg now calls invalidateAllOrgScopedData internally
    await storeSwitchOrg(orgId);
  };

  const createOrg = async (name: string) => {
    const newOrg = await storeCreateOrg(name);

    // Switch to new org to get valid JWT token with org_id claim
    const response = await orgsApi.setCurrentOrg({ org_id: newOrg.id });
    useAuthStore.setState({ token: response.data.token });

    await useAuthStore.getState().fetchProfile();

    // Clear all org-scoped caches
    await invalidateAllOrgScopedData(queryClient);

    return newOrg;
  };

  return {
    switchOrg,
    createOrg,
    isLoading,
  };
}
