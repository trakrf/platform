/**
 * useOrgSwitch - Orchestrates org switching with cache invalidation
 *
 * Handles:
 * - Calling orgStore.switchOrg() to update backend
 * - Invalidating all org-scoped TanStack Query caches
 * - Clearing all org-scoped Zustand stores
 *
 * This follows React best practices by keeping the queryClient access
 * within React context via useQueryClient() hook.
 */
import { useQueryClient } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useTagStore } from '@/stores/tagStore';
import { useBarcodeStore } from '@/stores/barcodeStore';

export function useOrgSwitch() {
  const queryClient = useQueryClient();
  const { switchOrg: storeSwitchOrg, isLoading } = useOrgStore();

  const switchOrg = async (orgId: number) => {
    // 1. Call backend to switch org
    await storeSwitchOrg(orgId);

    // 2. Clear all org-scoped Zustand stores
    useAssetStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();
    useTagStore.getState().clearTags();
    useBarcodeStore.getState().clearBarcodes();

    // 3. Invalidate all org-scoped TanStack Query caches
    // Using predicate to exclude auth-related queries
    queryClient.invalidateQueries({
      predicate: (query) => {
        const key = query.queryKey[0];
        return key !== 'user' && key !== 'profile';
      },
    });
  };

  return {
    switchOrg,
    isLoading,
  };
}
