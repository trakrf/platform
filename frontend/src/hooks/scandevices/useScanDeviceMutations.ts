import { useMutation, useQueryClient } from '@tanstack/react-query';
import { scanDevicesApi, scanPointsApi } from '@/lib/api/scandevices';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type {
  CreateScanDeviceRequest,
  UpdateScanDeviceRequest,
  CreateScanPointRequest,
  UpdateScanPointRequest,
} from '@/types/scandevices';

export function useScanDeviceMutations() {
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: async (data: CreateScanDeviceRequest) => {
      await ensureOrgContext();
      const response = await scanDevicesApi.create(data);
      return response.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scanDevices'] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      updates,
    }: {
      id: number;
      updates: UpdateScanDeviceRequest;
    }) => {
      await ensureOrgContext();
      const response = await scanDevicesApi.update(id, updates);
      return response.data.data;
    },
    onSuccess: (device) => {
      queryClient.invalidateQueries({ queryKey: ['scanDevices'] });
      queryClient.invalidateQueries({ queryKey: ['scanDevice', device.id] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await scanDevicesApi.delete(id);
      return id;
    },
    onSuccess: (id) => {
      queryClient.invalidateQueries({ queryKey: ['scanDevices'] });
      queryClient.invalidateQueries({ queryKey: ['scanDevice', id] });
    },
  });

  return {
    create: createMutation.mutateAsync,
    update: updateMutation.mutateAsync,
    delete: deleteMutation.mutateAsync,
    isCreating: createMutation.isPending,
    isUpdating: updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
    createError: createMutation.error,
    updateError: updateMutation.error,
    deleteError: deleteMutation.error,
  };
}

export function useScanPointMutations(deviceId: number) {
  const queryClient = useQueryClient();

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['scanPoints', deviceId] });
  };

  const createMutation = useMutation({
    mutationFn: async (data: CreateScanPointRequest) => {
      await ensureOrgContext();
      const response = await scanDevicesApi.createPoint(deviceId, data);
      return response.data.data;
    },
    onSuccess: invalidate,
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      updates,
    }: {
      id: number;
      updates: UpdateScanPointRequest;
    }) => {
      await ensureOrgContext();
      const response = await scanPointsApi.update(id, updates);
      return response.data.data;
    },
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await scanPointsApi.delete(id);
      return id;
    },
    onSuccess: invalidate,
  });

  return {
    create: createMutation.mutateAsync,
    update: updateMutation.mutateAsync,
    delete: deleteMutation.mutateAsync,
    isCreating: createMutation.isPending,
    isUpdating: updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
    createError: createMutation.error,
    updateError: updateMutation.error,
    deleteError: deleteMutation.error,
  };
}
