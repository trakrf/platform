import { useMutation } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import { jwtDecode } from 'jwt-decode';
import {
  inventoryApi,
  type SaveInventoryRequest,
  type SaveInventoryResponse,
} from '@/lib/api/inventory';
import { useAuthStore } from '@/stores/authStore';
import { orgsApi } from '@/lib/api/orgs';

interface JwtClaims {
  current_org_id?: number;
  user_id?: number;
}

/**
 * Extract org_id from the JWT in localStorage.
 * Returns null if missing, zero, or token is invalid.
 * Exported for testing.
 */
export function getTokenOrgId(): number | null {
  try {
    const authStorage = localStorage.getItem('auth-storage');
    if (!authStorage) return null;

    const { state } = JSON.parse(authStorage);
    if (!state?.token) return null;

    const decoded = jwtDecode<JwtClaims>(state.token);
    return decoded.current_org_id || null; // treats 0 and undefined as null
  } catch {
    return null;
  }
}

/**
 * Attempt to refresh the JWT with the current org context.
 * Returns true if the token was successfully refreshed.
 */
async function refreshOrgToken(): Promise<boolean> {
  try {
    const profile = useAuthStore.getState().profile;
    if (!profile) {
      await useAuthStore.getState().fetchProfile();
    }
    const currentOrgId = useAuthStore.getState().profile?.current_org?.id;
    if (!currentOrgId) return false;

    const response = await orgsApi.setCurrentOrg({ org_id: currentOrgId });
    // Zustand persist middleware wraps setState, so this persists to localStorage.
    // Same pattern used in orgStore.switchOrg (orgStore.ts:62).
    useAuthStore.setState({ token: response.data.token });
    return true;
  } catch {
    return false;
  }
}

/**
 * Hook for saving scanned inventory to the database.
 *
 * Includes a JWT org-context guard that:
 * 1. Checks the token has a valid current_org_id before sending
 * 2. On 403, attempts one token refresh + retry
 */
export function useInventorySave() {
  const saveMutation = useMutation({
    mutationFn: async (data: SaveInventoryRequest): Promise<SaveInventoryResponse> => {
      // Guard: verify JWT has org context before sending
      const orgId = getTokenOrgId();
      if (!orgId) {
        console.warn('[InventorySave] JWT missing org_id claim, refreshing token');
        const refreshed = await refreshOrgToken();
        if (!refreshed) {
          throw new Error('No organization context. Please select an organization and try again.');
        }
      }

      try {
        const response = await inventoryApi.save(data);
        return response.data.data;
      } catch (error: unknown) {
        // On 403, attempt one token refresh + retry
        const axiosError = error as { response?: { status?: number } };
        if (axiosError.response?.status === 403) {
          console.warn('[InventorySave] Got 403, attempting token refresh and retry');
          const refreshed = await refreshOrgToken();
          if (refreshed) {
            const retryResponse = await inventoryApi.save(data);
            return retryResponse.data.data;
          }
        }
        throw error;
      }
    },
    onSuccess: (result) => {
      toast.success(`${result.count} assets saved to ${result.location_name}`);
    },
    onError: (error: Error) => {
      if (error.message.includes('No organization context')) {
        toast.error(error.message);
      } else {
        toast.error('Failed to save inventory');
      }
    },
  });

  return {
    save: saveMutation.mutateAsync,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  };
}
