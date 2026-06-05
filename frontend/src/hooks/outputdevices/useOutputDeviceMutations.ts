import { useMutation, useQueryClient } from '@tanstack/react-query';
import { outputDevicesApi } from '@/lib/api/outputdevices';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type {
  CreateOutputDeviceRequest,
  UpdateOutputDeviceRequest,
} from '@/types/outputdevices';

export function useOutputDeviceMutations() {
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: async (data: CreateOutputDeviceRequest) => {
      await ensureOrgContext();
      const response = await outputDevicesApi.create(data);
      return response.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['outputDevices'] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      updates,
    }: {
      id: number;
      updates: UpdateOutputDeviceRequest;
    }) => {
      await ensureOrgContext();
      const response = await outputDevicesApi.update(id, updates);
      return response.data.data;
    },
    onSuccess: (device) => {
      queryClient.invalidateQueries({ queryKey: ['outputDevices'] });
      queryClient.invalidateQueries({ queryKey: ['outputDevice', device.id] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await outputDevicesApi.delete(id);
      return id;
    },
    onSuccess: (id) => {
      queryClient.invalidateQueries({ queryKey: ['outputDevices'] });
      queryClient.invalidateQueries({ queryKey: ['outputDevice', id] });
    },
  });

  // Test-fire and reset don't mutate stored state, so they don't invalidate
  // the list — they just drive the physical device.
  const testMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await outputDevicesApi.test(id);
      return id;
    },
  });

  const resetMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await outputDevicesApi.reset(id);
      return id;
    },
  });

  return {
    create: createMutation.mutateAsync,
    update: updateMutation.mutateAsync,
    delete: deleteMutation.mutateAsync,
    test: testMutation.mutateAsync,
    reset: resetMutation.mutateAsync,
    isCreating: createMutation.isPending,
    isUpdating: updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
    isTesting: testMutation.isPending,
    isResetting: resetMutation.isPending,
    createError: createMutation.error,
    updateError: updateMutation.error,
    deleteError: deleteMutation.error,
  };
}
