import { useState, useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useUploadStore } from '@/stores/uploadStore';
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';
import { ProcessingAlert } from './ProcessingAlert';
import { SuccessAlert } from './SuccessAlert';
import { ErrorAlert } from './ErrorAlert';
import { ErrorDetailsModal } from './ErrorDetailsModal';

export function GlobalUploadAlert() {
  const activeJobId = useUploadStore((state) => state.activeJobId);
  const clearActiveJobId = useUploadStore((state) => state.clearActiveJobId);
  const setPage = useAssetStore((state) => state.setPage);
  const queryClient = useQueryClient();

  const [showErrorModal, setShowErrorModal] = useState(false);

  const {
    data: jobStatus,
    error,
    refetch,
  } = useQuery({
    queryKey: ['job-status', activeJobId],
    queryFn: async () => {
      if (!activeJobId) return null;
      const response = await assetsApi.getJobStatus(activeJobId);
      return response.data;
    },
    enabled: !!activeJobId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === 'pending' || status === 'processing') {
        return 2000;
      }
      return false;
    },
    retry: 3,
    staleTime: 0,
    gcTime: 0,
  });

  useEffect(() => {
    if (jobStatus?.status === 'completed') {
      queryClient.invalidateQueries({
        queryKey: ['assets'],
        refetchType: 'all'
      });
      setPage(1);
    }
  }, [jobStatus?.status, queryClient, setPage]);

  const handleDismiss = () => {
    clearActiveJobId();
    setShowErrorModal(false);
  };

  const handleRetry = () => {
    refetch();
  };

  const handleViewErrors = () => {
    setShowErrorModal(true);
  };

  const handleCloseErrorModal = () => {
    setShowErrorModal(false);
  };

  if (!activeJobId) {
    return null;
  }

  if (error) {
    return (
      <ErrorAlert
        error={error as Error}
        onDismiss={handleDismiss}
        onRetry={handleRetry}
      />
    );
  }

  if (!jobStatus) {
    return null;
  }

  switch (jobStatus.status) {
    case 'pending':
    case 'processing':
      return <ProcessingAlert jobStatus={jobStatus} onDismiss={handleDismiss} />;

    case 'completed':
      return (
        <>
          <SuccessAlert
            jobStatus={jobStatus}
            onDismiss={handleDismiss}
            onViewErrors={handleViewErrors}
          />
          {jobStatus.errors && (
            <ErrorDetailsModal
              isOpen={showErrorModal}
              errors={jobStatus.errors}
              onClose={handleCloseErrorModal}
            />
          )}
        </>
      );

    case 'failed':
      return (
        <>
          <ErrorAlert
            jobStatus={jobStatus}
            onDismiss={handleDismiss}
            onRetry={handleRetry}
            onViewDetails={jobStatus.errors ? handleViewErrors : undefined}
          />
          {jobStatus.errors && (
            <ErrorDetailsModal
              isOpen={showErrorModal}
              errors={jobStatus.errors}
              onClose={handleCloseErrorModal}
            />
          )}
        </>
      );

    default:
      return null;
  }
}
