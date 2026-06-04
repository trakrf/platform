import { useMutation, useQueryClient } from '@tanstack/react-query';
import { alarmDevicesApi } from '@/lib/api/alarmdevices';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type {
  CreateAlarmDeviceRequest,
  UpdateAlarmDeviceRequest,
} from '@/types/alarmdevices';

export function useAlarmDeviceMutations() {
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: async (data: CreateAlarmDeviceRequest) => {
      await ensureOrgContext();
      const response = await alarmDevicesApi.create(data);
      return response.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alarmDevices'] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      updates,
    }: {
      id: number;
      updates: UpdateAlarmDeviceRequest;
    }) => {
      await ensureOrgContext();
      const response = await alarmDevicesApi.update(id, updates);
      return response.data.data;
    },
    onSuccess: (device) => {
      queryClient.invalidateQueries({ queryKey: ['alarmDevices'] });
      queryClient.invalidateQueries({ queryKey: ['alarmDevice', device.id] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await alarmDevicesApi.delete(id);
      return id;
    },
    onSuccess: (id) => {
      queryClient.invalidateQueries({ queryKey: ['alarmDevices'] });
      queryClient.invalidateQueries({ queryKey: ['alarmDevice', id] });
    },
  });

  // Test-fire and reset don't mutate stored state, so they don't invalidate
  // the list — they just drive the physical device.
  const testMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await alarmDevicesApi.test(id);
      return id;
    },
  });

  const resetMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await alarmDevicesApi.reset(id);
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
