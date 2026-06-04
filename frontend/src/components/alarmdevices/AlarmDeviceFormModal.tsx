import React, { useState } from 'react';
import { X } from 'lucide-react';
import toast from 'react-hot-toast';
import type {
  AlarmDevice,
  CreateAlarmDeviceRequest,
  UpdateAlarmDeviceRequest,
} from '@/types/alarmdevices';
import { AlarmDeviceForm } from './AlarmDeviceForm';
import { useAlarmDeviceMutations } from '@/hooks/alarmdevices';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { useEscapeToClose } from '@/hooks/useEscapeToClose';

interface AlarmDeviceFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  device?: AlarmDevice;
  onClose: () => void;
}

// Outer gate returns null when closed so the stateful body unmounts each
// open/close cycle (mirrors ScanDeviceFormModal — TRA-817).
export function AlarmDeviceFormModal(props: AlarmDeviceFormModalProps) {
  if (!props.isOpen) {
    return null;
  }
  return <AlarmDeviceFormModalBody {...props} />;
}

function AlarmDeviceFormModalBody({ isOpen, mode, device, onClose }: AlarmDeviceFormModalProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { create, update } = useAlarmDeviceMutations();

  useEscapeToClose(isOpen, onClose, loading);

  const handleSubmit = async (data: CreateAlarmDeviceRequest | UpdateAlarmDeviceRequest) => {
    setLoading(true);
    setError(null);

    try {
      if (mode === 'create') {
        const created = await create(data as CreateAlarmDeviceRequest);
        toast.success(`Alarm device "${created.name}" created successfully`);
      } else if (mode === 'edit' && device) {
        const updated = await update({ id: device.id, updates: data as UpdateAlarmDeviceRequest });
        toast.success(`Alarm device "${updated.name}" updated successfully`);
      }
      onClose();
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to save alarm device. Please try again.'));
    } finally {
      setLoading(false);
    }
  };

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget && !loading) {
      onClose();
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 p-4"
      onClick={handleBackdropClick}
    >
      <div className="relative w-full max-w-2xl bg-white dark:bg-gray-900 rounded-lg shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between z-10">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
            {mode === 'create' ? 'Create New Alarm Device' : `Edit Alarm Device: ${device?.name}`}
          </h2>
          <button
            onClick={onClose}
            disabled={loading}
            className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg transition-colors disabled:opacity-50"
            aria-label="Close modal"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="px-6 py-6">
          <AlarmDeviceForm
            mode={mode}
            device={device}
            onSubmit={handleSubmit}
            onCancel={onClose}
            loading={loading}
            error={error}
          />
        </div>
      </div>
    </div>
  );
}
