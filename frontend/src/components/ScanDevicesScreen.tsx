import { useState, useEffect, Fragment } from 'react';
import { Plus, ChevronRight, ChevronDown, Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { useScanDevices, useScanDeviceMutations } from '@/hooks/scandevices';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { useUIStore } from '@/stores';
import { ConfirmModal } from '@/components/shared';
import { ScanDeviceFormModal, ScanDeviceEditPanel } from '@/components/scandevices';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import type { ScanDevice } from '@/types/scandevices';

// The reader list is flat (TRA-931): scan_devices only, no scan_point tree.
// Editing happens inline via a single-open row expander (TRA-938) — the
// commissioning surface (antennas/location + scoped live feed) opens under the
// row rather than in a modal, so tuning stays a stay-on-the-list activity.
export default function ScanDevicesScreen() {
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [deletingDevice, setDeletingDevice] = useState<ScanDevice | null>(null);

  const toggleExpanded = (id: number) =>
    setExpandedId((current) => (current === id ? null : id));

  const { scanDevices, isLoading } = useScanDevices();
  const { delete: deleteScanDevice } = useScanDeviceMutations();
  const { setActiveTab } = useUIStore();

  useEffect(() => {
    setActiveTab('scan-devices');
  }, [setActiveTab]);

  const confirmDelete = async () => {
    if (!deletingDevice) return;
    try {
      await deleteScanDevice(deletingDevice.id);
      toast.success(`Scan device "${deletingDevice.external_key}" deleted successfully`);
      if (expandedId === deletingDevice.id) setExpandedId(null);
      setDeletingDevice(null);
    } catch (error) {
      toast.error(getApiErrorMessage(error, 'Failed to delete scan device'));
    }
  };

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Scan Devices</h1>
          <button
            type="button"
            onClick={() => setIsCreateModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 transition-colors"
          >
            <Plus className="w-4 h-4" />
            New Scan Device
          </button>
        </div>

        <div className="flex-1 overflow-auto border border-gray-200 dark:border-gray-700 rounded-lg">
          {isLoading ? (
            <p className="p-4 text-sm text-gray-500 dark:text-gray-400">Loading scan devices…</p>
          ) : scanDevices.length === 0 ? (
            <p className="p-4 text-sm text-gray-600 dark:text-gray-400">
              No scan devices yet. Create one to register a reader.
            </p>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr className="text-left text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-gray-700">
                  <th className="py-2 px-3 font-medium">External Key</th>
                  <th className="px-3 font-medium">Name</th>
                  <th className="px-3 font-medium">Type</th>
                  <th className="px-3 font-medium">Transport</th>
                  <th className="px-3 font-medium">Publish Topic</th>
                  <th className="px-3 font-medium">Active</th>
                  <th className="px-3"></th>
                </tr>
              </thead>
              <tbody>
                {scanDevices.map((device) => {
                  const isExpanded = expandedId === device.id;
                  return (
                    <Fragment key={device.id}>
                      <tr className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50">
                        <td className="py-2 px-3 font-mono text-xs text-gray-900 dark:text-gray-100">
                          {device.external_key}
                        </td>
                        <td className="px-3 text-gray-900 dark:text-gray-100">{device.name}</td>
                        <td className="px-3 text-gray-700 dark:text-gray-300">{device.type}</td>
                        <td className="px-3 text-gray-700 dark:text-gray-300">{device.transport}</td>
                        <td className="px-3 font-mono text-xs text-gray-700 dark:text-gray-300">
                          {device.publish_topic || '—'}
                        </td>
                        <td className="px-3 text-gray-700 dark:text-gray-300">
                          {device.is_active ? 'Yes' : 'No'}
                        </td>
                        <td className="px-3 text-right whitespace-nowrap">
                          <button
                            type="button"
                            onClick={() => toggleExpanded(device.id)}
                            className="p-1.5 text-gray-500 hover:text-blue-600 dark:text-gray-400 dark:hover:text-blue-400"
                            aria-label={`Edit scan device ${device.external_key}`}
                            aria-expanded={isExpanded}
                          >
                            {isExpanded ? (
                              <ChevronDown className="w-4 h-4" />
                            ) : (
                              <ChevronRight className="w-4 h-4" />
                            )}
                          </button>
                          <button
                            type="button"
                            onClick={() => setDeletingDevice(device)}
                            className="p-1.5 text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400"
                            aria-label={`Delete scan device ${device.external_key}`}
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </td>
                      </tr>
                      {isExpanded && (
                        <tr className="border-b border-gray-100 dark:border-gray-800 bg-gray-50/50 dark:bg-gray-800/30">
                          <td colSpan={7} className="px-3 py-4">
                            <ScanDeviceEditPanel
                              device={device}
                              onClose={() => setExpandedId(null)}
                            />
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>

        <ScanDeviceFormModal
          isOpen={isCreateModalOpen}
          mode="create"
          onClose={() => setIsCreateModalOpen(false)}
        />

        <ConfirmModal
          isOpen={!!deletingDevice}
          title="Delete Scan Device"
          message={`Are you sure you want to delete "${deletingDevice?.external_key}"? This action cannot be undone.`}
          onConfirm={confirmDelete}
          onCancel={() => setDeletingDevice(null)}
        />
      </div>
    </ProtectedRoute>
  );
}
