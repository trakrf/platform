import { create } from 'zustand';
import { createStoreWithTracking } from './createStore';
import { orgsApi } from '@/lib/api/orgs';
import { useAuthStore } from './authStore';
import { useAssetStore } from './assets/assetStore';
import { useLocationStore } from './locations/locationStore';
import type { UserOrgWithRole, UserOrg, OrgRole, Organization } from '@/types/org';

interface OrgState {
  // Derived state (computed from authStore.profile)
  currentOrg: UserOrgWithRole | null;
  currentRole: OrgRole | null;
  orgs: UserOrg[];

  // Action state
  isLoading: boolean;
  error: string | null;

  // Actions
  switchOrg: (orgId: number) => Promise<void>;
  createOrg: (name: string) => Promise<Organization>;
  clearError: () => void;
  syncFromProfile: () => void;
}

export const useOrgStore = create<OrgState>()(
  createStoreWithTracking(
    (set, get) => ({
      // Initial state
      currentOrg: null,
      currentRole: null,
      orgs: [],
      isLoading: false,
      error: null,

      // Sync state from authStore profile
      // Invalidates org-specific caches when org changes (including on login)
      syncFromProfile: async () => {
        const profile = useAuthStore.getState().profile;
        const previousOrgId = get().currentOrg?.id;
        const newOrgId = profile?.current_org?.id;

        // Invalidate caches if org changed (including null -> value on login)
        if (previousOrgId !== newOrgId) {
          useAssetStore.getState().invalidateCache();
          useLocationStore.getState().invalidateCache();
        }

        if (profile) {
          set({
            currentOrg: profile.current_org,
            currentRole: profile.current_org?.role ?? null,
            orgs: profile.orgs,
          });

          // On first login (null -> org), refresh the token to include org_id claim
          // This ensures the lookup API has proper org context
          if (previousOrgId === undefined && newOrgId !== undefined) {
            try {
              console.log('[OrgStore] Refreshing token with org_id after login');
              const response = await orgsApi.setCurrentOrg({ org_id: newOrgId });
              const authState = useAuthStore.getState();
              useAuthStore.setState({ ...authState, token: response.data.token });
              console.log('[OrgStore] Token refreshed, triggering tag enrichment');
              // Import dynamically to avoid circular dependency
              const { useTagStore } = await import('./tagStore');
              useTagStore.getState().refreshAssetEnrichment();
            } catch (error) {
              console.error('[OrgStore] Failed to refresh token after login:', error);
            }
          }
        } else {
          set({
            currentOrg: null,
            currentRole: null,
            orgs: [],
          });
        }
      },

      // Switch to a different organization
      switchOrg: async (orgId: number) => {
        set({ isLoading: true, error: null });
        try {
          const response = await orgsApi.setCurrentOrg({ org_id: orgId });
          // Invalidate org-specific caches before updating token
          useAssetStore.getState().invalidateCache();
          useLocationStore.getState().invalidateCache();
          // Update the token with new org_id claim
          const authState = useAuthStore.getState();
          useAuthStore.setState({ ...authState, token: response.data.token });
          // Refetch profile to get updated current_org
          await useAuthStore.getState().fetchProfile();
          // Sync derived state
          const profile = useAuthStore.getState().profile;
          if (profile) {
            set({
              currentOrg: profile.current_org,
              currentRole: profile.current_org?.role ?? null,
              orgs: profile.orgs,
              isLoading: false,
            });
          }
        } catch (err: any) {
          const errorMessage =
            err.response?.data?.detail ||
            err.response?.data?.error ||
            err.message ||
            'Failed to switch organization';
          set({ error: errorMessage, isLoading: false });
          throw err;
        }
      },

      // Create a new organization
      createOrg: async (name: string) => {
        set({ isLoading: true, error: null });
        try {
          const response = await orgsApi.create({ name });
          const newOrg = response.data.data;
          // Refetch profile to get updated orgs list
          await useAuthStore.getState().fetchProfile();
          // Sync derived state
          const profile = useAuthStore.getState().profile;
          if (profile) {
            set({
              currentOrg: profile.current_org,
              currentRole: profile.current_org?.role ?? null,
              orgs: profile.orgs,
              isLoading: false,
            });
          }
          return newOrg;
        } catch (err: any) {
          const errorMessage =
            err.response?.data?.detail ||
            err.response?.data?.error ||
            err.message ||
            'Failed to create organization';
          set({ error: errorMessage, isLoading: false });
          throw err;
        }
      },

      clearError: () => set({ error: null }),
    }),
    'orgStore'
  )
);

// Subscribe to authStore profile changes and sync
useAuthStore.subscribe((state, prevState) => {
  if (state.profile !== prevState.profile) {
    useOrgStore.getState().syncFromProfile();
  }
});
