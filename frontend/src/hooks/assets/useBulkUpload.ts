import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, useEffect } from 'react';
import { assetsApi } from '@/lib/api/assets';
import { useAssetStore } from '@/stores/assets/assetStore';

export function useBulkUpload() {
  const queryClient = useQueryClient();
  const [jobId, setJobId] = useState<string | null>(null);

  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
      const response = await assetsApi.uploadCSV(file);
      return response.data;
    },
    onSuccess: (data) => {
      setJobId(data.job_id);
    },
  });

  const statusQuery = useQuery({
    queryKey: ['bulk-upload-status', jobId],
    queryFn: async () => {
      if (!jobId) return null;
      const response = await assetsApi.getJobStatus(jobId);
      return response.data;
    },
    enabled: !!jobId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === 'processing' || status === 'pending' ? 2000 : false;
    },
  });

  useEffect(() => {
    if (statusQuery.data?.status === 'completed') {
      queryClient.invalidateQueries({ queryKey: ['assets'] });
      useAssetStore.getState().invalidateCache();
    }
  }, [statusQuery.data?.status, queryClient]);

  const progress =
    statusQuery.data?.total_rows && statusQuery.data.total_rows > 0
      ? Math.round(
          (statusQuery.data.processed_rows / statusQuery.data.total_rows) * 100
        )
      : null;

  const reset = () => {
    setJobId(null);
    uploadMutation.reset();
  };

  return {
    upload: uploadMutation.mutateAsync,
    isUploading: uploadMutation.isPending,
    uploadError: uploadMutation.error,
    jobStatus: statusQuery.data?.status ?? null,
    jobProgress: progress,
    jobData: statusQuery.data,
    isPolling: statusQuery.isFetching,
    reset,
  };
}
