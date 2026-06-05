import React, { useState } from 'react';
import { X } from 'lucide-react';
import toast from 'react-hot-toast';
import type {
  ScanDevice,
  CreateScanDeviceRequest,
  UpdateScanDeviceRequest,
} from '@/types/scandevices';
import { ScanDeviceForm } from './ScanDeviceForm';
import { ReaderPointsSection } from './ReaderPointsSection';
import { LiveReadsFeed } from '@/components/readerfeed/LiveReadsFeed';
import { useScanDeviceMutations } from '@/hooks/scandevices';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { readerKeyForDevice } from '@/lib/scandevices/deviceProfile';
import { useEscapeToClose } from '@/hooks/useEscapeToClose';

interface ScanDeviceFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  device?: ScanDevice;
  onClose: () => void;
}

// Outer gate returns null when closed so the stateful body unmounts each
// open/close cycle (mirrors LocationFormModal / AssetFormModal — TRA-817).
export function ScanDeviceFormModal(props: ScanDeviceFormModalProps) {
  if (!props.isOpen) {
    return null;
  }
  return <ScanDeviceFormModalBody {...props} />;
}

function ScanDeviceFormModalBody({ isOpen, mode, device, onClose }: ScanDeviceFormModalProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { create, update } = useScanDeviceMutations();

  useEscapeToClose(isOpen, onClose, loading);

  const handleSubmit = async (data: CreateScanDeviceRequest | UpdateScanDeviceRequest) => {
    setLoading(true);
    setError(null);

    try {
      if (mode === 'create') {
        const created = await create(data as CreateScanDeviceRequest);
        toast.success(`Scan device "${created.external_key}" created successfully`);
      } else if (mode === 'edit' && device) {
        const updated = await update({ id: device.id, updates: data as UpdateScanDeviceRequest });
        toast.success(`Scan device "${updated.external_key}" updated successfully`);
      }
      onClose();
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to save scan device. Please try again.'));
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
      <div
        className={`relative w-full ${
          mode === 'edit' ? 'max-w-4xl' : 'max-w-2xl'
        } bg-white dark:bg-gray-900 rounded-lg shadow-xl max-h-[90vh] overflow-y-auto`}
      >
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between z-10">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
            {mode === 'create' ? 'Create New Scan Device' : `Edit Scan Device: ${device?.external_key}`}
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
          <ScanDeviceForm
            mode={mode}
            device={device}
            onSubmit={handleSubmit}
            onCancel={onClose}
            loading={loading}
            error={error}
          />

          {/* Reader edit is the single commissioning surface (TRA-931): once the
              device exists, manage its antennas/location and watch its live feed
              right here. Both are skipped on create — they key off the saved
              device id. */}
          {mode === 'edit' && device && (
            <>
              <section className="mt-8 pt-6 border-t border-gray-200 dark:border-gray-700">
                <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-4">
                  Antennas &amp; Location
                </h3>
                <ReaderPointsSection device={device} />
              </section>

              <section className="mt-8 pt-6 border-t border-gray-200 dark:border-gray-700">
                <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-4">
                  Live Reads
                </h3>
                <p className="text-xs text-gray-500 dark:text-gray-400 mb-3">
                  Reads off this reader only — for antenna placement and RSSI tuning.
                </p>
                <LiveReadsFeed filterReaderKey={readerKeyForDevice(device)} compact />
              </section>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
