import { create } from 'zustand';
import { createStoreWithTracking } from './createStore';
import { orgsApi } from '@/lib/api/orgs';
import { useAuthStore } from './authStore';
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
    (set) => ({
      // Initial state
      currentOrg: null,
      currentRole: null,
      orgs: [],
      isLoading: false,
      error: null,

      // Sync state from authStore profile
      syncFromProfile: () => {
        const profile = useAuthStore.getState().profile;
        if (profile) {
          set({
            currentOrg: profile.current_org,
            currentRole: profile.current_org?.role ?? null,
            orgs: profile.orgs,
          });
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
