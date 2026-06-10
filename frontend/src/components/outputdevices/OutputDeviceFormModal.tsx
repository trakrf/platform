import React, { useState } from 'react';
import { X, Zap, Power } from 'lucide-react';
import toast from 'react-hot-toast';
import type {
  OutputDevice,
  CreateOutputDeviceRequest,
  UpdateOutputDeviceRequest,
} from '@/types/outputdevices';
import { OutputDeviceForm } from './OutputDeviceForm';
import { PaidGate } from '@/components/entitlement';
import { useOutputDeviceMutations } from '@/hooks/outputdevices';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { useEscapeToClose } from '@/hooks/useEscapeToClose';

interface OutputDeviceFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  device?: OutputDevice;
  onClose: () => void;
  // 'modal' (default) renders the full backdrop + chrome; 'inline' renders the
  // same form (plus test-fire/reset in edit mode) bare, for embedding in a row
  // expander (TRA-938). Submit/loading/error logic is shared across both.
  variant?: 'modal' | 'inline';
}

// Outer gate returns null when closed so the stateful body unmounts each
// open/close cycle (mirrors ScanDeviceFormModal — TRA-817).
export function OutputDeviceFormModal(props: OutputDeviceFormModalProps) {
  if (!props.isOpen) {
    return null;
  }
  return <OutputDeviceFormModalBody {...props} />;
}

// Inline (row-expander) presentation of the device edit surface (TRA-938).
// Thin wrapper so call sites read clearly; all logic lives in the shared body.
export function OutputDeviceEditPanel({
  device,
  onClose,
}: {
  device: OutputDevice;
  onClose: () => void;
}) {
  return (
    <OutputDeviceFormModal isOpen mode="edit" device={device} onClose={onClose} variant="inline" />
  );
}

function OutputDeviceFormModalBody({
  isOpen,
  mode,
  device,
  onClose,
  variant = 'modal',
}: OutputDeviceFormModalProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const { create, update, test, reset } = useOutputDeviceMutations();

  // Escape-to-close only applies to the modal; collapsing a row mid-edit on a
  // stray Escape would be surprising.
  useEscapeToClose(isOpen, onClose, loading || variant !== 'modal');

  const handleSubmit = async (data: CreateOutputDeviceRequest | UpdateOutputDeviceRequest) => {
    setLoading(true);
    setError(null);

    try {
      if (mode === 'create') {
        const created = await create(data as CreateOutputDeviceRequest);
        toast.success(`Output device "${created.name}" created successfully`);
      } else if (mode === 'edit' && device) {
        const updated = await update({ id: device.id, updates: data as UpdateOutputDeviceRequest });
        toast.success(`Output device "${updated.name}" updated successfully`);
      }
      onClose();
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to save output device. Please try again.'));
    } finally {
      setLoading(false);
    }
  };

  // Test-fire / reset drive the physical device; they don't touch stored state,
  // so they live alongside the edit form rather than gating it (TRA-938).
  const runDeviceAction = async (action: 'test' | 'reset') => {
    if (!device) return;
    setBusy(true);
    try {
      if (action === 'test') {
        await test(device.id);
        toast.success(`Test-fired "${device.name}"`);
      } else {
        await reset(device.id);
        toast.success(`Reset "${device.name}"`);
      }
    } catch (err) {
      toast.error(getApiErrorMessage(err, 'Output device unreachable'));
    } finally {
      setBusy(false);
    }
  };

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget && !loading) {
      onClose();
    }
  };

  const content = (
    <>
      <OutputDeviceForm
        mode={mode}
        device={device}
        onSubmit={handleSubmit}
        onCancel={onClose}
        loading={loading}
        error={error}
      />

      {mode === 'edit' && device && (
        <section className="mt-8 pt-6 border-t border-gray-200 dark:border-gray-700">
          <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-3">
            Test
          </h3>
          <PaidGate surface="outputs-crud" silentImpression>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => runDeviceAction('test')}
                disabled={busy}
                className="flex items-center gap-2 px-3 py-2 text-sm font-medium text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/30 rounded-lg hover:bg-amber-100 dark:hover:bg-amber-900/50 transition-colors disabled:opacity-40"
                aria-label={`Test-fire output device ${device.name}`}
              >
                <Zap className="w-4 h-4" />
                Test-fire
              </button>
              <button
                type="button"
                onClick={() => runDeviceAction('reset')}
                disabled={busy}
                className="flex items-center gap-2 px-3 py-2 text-sm font-medium text-green-700 dark:text-green-300 bg-green-50 dark:bg-green-900/30 rounded-lg hover:bg-green-100 dark:hover:bg-green-900/50 transition-colors disabled:opacity-40"
                aria-label={`Reset output device ${device.name}`}
              >
                <Power className="w-4 h-4" />
                Reset (off)
              </button>
            </div>
          </PaidGate>
        </section>
      )}
    </>
  );

  if (variant === 'inline') {
    return <div className="px-1 py-2">{content}</div>;
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 p-4"
      onClick={handleBackdropClick}
    >
      <div className="relative w-full max-w-2xl bg-white dark:bg-gray-900 rounded-lg shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between z-10">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
            {mode === 'create' ? 'Create New Output Device' : `Edit Output Device: ${device?.name}`}
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

        <div className="px-6 py-6">{content}</div>
      </div>
    </div>
  );
}
