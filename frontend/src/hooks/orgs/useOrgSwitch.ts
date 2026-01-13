/**
 * useOrgSwitch - Orchestrates org switching and creation with cache invalidation
 *
 * Handles:
 * - Calling orgStore.switchOrg() to update backend
 * - Creating new orgs with proper JWT token setup
 * - Invalidating all org-scoped TanStack Query caches
 * - Clearing all org-scoped Zustand stores
 *
 * This follows React best practices by keeping the queryClient access
 * within React context via useQueryClient() hook.
 */
import { useQueryClient } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { useAuthStore } from '@/stores/authStore';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useTagStore } from '@/stores/tagStore';
import { useBarcodeStore } from '@/stores/barcodeStore';
import { orgsApi } from '@/lib/api/orgs';

export function useOrgSwitch() {
  const queryClient = useQueryClient();
  const { switchOrg: storeSwitchOrg, createOrg: storeCreateOrg, isLoading } = useOrgStore();

  // Helper to invalidate all org-scoped caches
  const invalidateOrgCaches = () => {
    // Clear all org-scoped Zustand stores
    useAssetStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();
    useTagStore.getState().clearTags();
    useBarcodeStore.getState().clearBarcodes();

    // Invalidate all org-scoped TanStack Query caches
    // Using predicate to exclude auth-related queries
    queryClient.invalidateQueries({
      predicate: (query) => {
        const key = query.queryKey[0];
        return key !== 'user' && key !== 'profile';
      },
    });
  };

  const switchOrg = async (orgId: number) => {
    // 1. Call backend to switch org (updates JWT token)
    await storeSwitchOrg(orgId);

    // 2. Clear all org-scoped caches
    invalidateOrgCaches();
  };

  const createOrg = async (name: string) => {
    // 1. Create org via store
    const newOrg = await storeCreateOrg(name);

    // 2. Switch to new org to get valid JWT token with org_id claim
    const response = await orgsApi.setCurrentOrg({ org_id: newOrg.id });
    useAuthStore.setState({ token: response.data.token });

    // 3. Refetch profile with new token
    await useAuthStore.getState().fetchProfile();

    // 4. Clear all org-scoped caches
    invalidateOrgCaches();

    return newOrg;
  };

  return {
    switchOrg,
    createOrg,
    isLoading,
  };
}
