import { useState, useEffect, Fragment } from 'react';
import { Plus, ChevronRight, ChevronDown, Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { useOutputDevices, useOutputDeviceMutations } from '@/hooks/outputdevices';
import { useLocations } from '@/hooks/locations';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { useUIStore } from '@/stores';
import { ConfirmModal, InlineEditCell } from '@/components/shared';
import { OutputDeviceFormModal, OutputDeviceEditPanel } from '@/components/outputdevices';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { PaidGate } from '@/components/entitlement';
import { validateName } from '@/lib/location/validators';
import type { OutputDevice } from '@/types/outputdevices';

// Editing happens inline via a single-open row expander (TRA-938): the config
// form plus test-fire/reset controls open under the row rather than in a modal.
export default function OutputDevicesScreen() {
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [deletingDevice, setDeletingDevice] = useState<OutputDevice | null>(null);

  const { outputDevices, isLoading } = useOutputDevices();
  const { locations } = useLocations();
  const { delete: deleteOutputDevice, update: updateOutputDevice } = useOutputDeviceMutations();
  const { setActiveTab } = useUIStore();

  const locationOptions = [
    { value: '', label: '— None —' },
    ...locations.map((l) => ({ value: String(l.id), label: l.name })),
  ];

  const validateSwitchId = (raw: string) =>
    /^\d+$/.test(raw.trim()) ? null : 'Switch ID must be a non-negative integer';

  const toggleExpanded = (id: number) =>
    setExpandedId((current) => (current === id ? null : id));

  const locationName = (id?: number | null) =>
    id == null ? '—' : (locations.find((l) => l.id === id)?.name ?? `#${id}`);

  useEffect(() => {
    setActiveTab('output-devices');
  }, [setActiveTab]);

  const confirmDelete = async () => {
    if (!deletingDevice) return;
    try {
      await deleteOutputDevice(deletingDevice.id);
      toast.success(`Output device "${deletingDevice.name}" deleted successfully`);
      if (expandedId === deletingDevice.id) setExpandedId(null);
      setDeletingDevice(null);
    } catch (error) {
      toast.error(getApiErrorMessage(error, 'Failed to delete output device'));
    }
  };

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Output Devices</h1>
          <PaidGate surface="outputs-crud">
            <button
              type="button"
              onClick={() => setIsCreateModalOpen(true)}
              className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 transition-colors"
            >
              <Plus className="w-4 h-4" />
              New Output Device
            </button>
          </PaidGate>
        </div>

        <div className="flex-1 overflow-auto border border-gray-200 dark:border-gray-700 rounded-lg">
          {isLoading ? (
            <p className="p-4 text-sm text-gray-500 dark:text-gray-400">Loading output devices…</p>
          ) : outputDevices.length === 0 ? (
            <p className="p-4 text-sm text-gray-600 dark:text-gray-400">
              No output devices yet. Create one to wire a Shelly relay.
            </p>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr className="text-left text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-gray-700">
                  <th className="w-8 px-2"></th>
                  <th className="py-2 px-3 font-medium">Name</th>
                  <th className="px-3 font-medium">Type</th>
                  <th className="px-3 font-medium">Transport</th>
                  <th className="px-3 font-medium">Target</th>
                  <th className="px-3 font-medium">Switch</th>
                  <th className="px-3 font-medium">Location</th>
                  <th className="px-3 font-medium">Active</th>
                  <th className="px-3"></th>
                </tr>
              </thead>
              <tbody>
                {outputDevices.map((device) => {
                  const isExpanded = expandedId === device.id;
                  return (
                    <Fragment key={device.id}>
                      <tr className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50">
                        <td className="w-8 px-2">
                          <button
                            type="button"
                            onClick={() => toggleExpanded(device.id)}
                            className="p-1.5 text-gray-500 hover:text-blue-600 dark:text-gray-400 dark:hover:text-blue-400"
                            aria-label={`Edit output device ${device.name}`}
                            aria-expanded={isExpanded}
                          >
                            {isExpanded ? (
                              <ChevronDown className="w-4 h-4" />
                            ) : (
                              <ChevronRight className="w-4 h-4" />
                            )}
                          </button>
                        </td>
                        <td className="py-2 px-3 text-gray-900 dark:text-gray-100">
                          <PaidGate surface="outputs-crud" silentImpression>
                            <InlineEditCell
                              variant="text"
                              value={device.name}
                              ariaLabel={`Edit name for ${device.name}`}
                              validate={validateName}
                              onSave={(name) =>
                                updateOutputDevice({ id: device.id, updates: { name } }).then(
                                  () => undefined
                                )
                              }
                            />
                          </PaidGate>
                        </td>
                        <td className="px-3 text-gray-700 dark:text-gray-300">{device.type}</td>
                        <td className="px-3 text-gray-700 dark:text-gray-300">{device.transport}</td>
                        <td className="px-3 font-mono text-xs text-gray-700 dark:text-gray-300">
                          {device.transport === 'mqtt' ? (device.command_topic || '—') : device.base_url}
                        </td>
                        <td className="px-3 text-gray-700 dark:text-gray-300">
                          <PaidGate surface="outputs-crud" silentImpression>
                            <InlineEditCell
                              variant="number"
                              value={device.switch_id}
                              ariaLabel={`Edit switch ID for ${device.name}`}
                              validate={validateSwitchId}
                              onSave={(raw) =>
                                updateOutputDevice({
                                  id: device.id,
                                  updates: { switch_id: parseInt(raw, 10) },
                                }).then(() => undefined)
                              }
                            />
                          </PaidGate>
                        </td>
                        <td className="px-3 text-gray-700 dark:text-gray-300">
                          <PaidGate surface="outputs-crud" silentImpression>
                            <InlineEditCell
                              variant="select"
                              value={device.location_id != null ? String(device.location_id) : ''}
                              ariaLabel={`Edit location for ${device.name}`}
                              options={locationOptions}
                              display={(v) => locationName(v ? Number(v) : null)}
                              onSave={(raw) =>
                                updateOutputDevice({
                                  id: device.id,
                                  updates: { location_id: raw === '' ? null : parseInt(raw, 10) },
                                }).then(() => undefined)
                              }
                            />
                          </PaidGate>
                        </td>
                        <td className="px-3 text-gray-700 dark:text-gray-300">
                          <PaidGate surface="outputs-crud" silentImpression>
                            <InlineEditCell
                              variant="toggle"
                              value={device.is_active}
                              ariaLabel={`Toggle active for ${device.name}`}
                              onSave={(is_active) =>
                                updateOutputDevice({ id: device.id, updates: { is_active } }).then(
                                  () => undefined
                                )
                              }
                            />
                          </PaidGate>
                        </td>
                        <td className="px-3 text-right whitespace-nowrap">
                          <PaidGate surface="outputs-crud" silentImpression>
                            <button
                              type="button"
                              onClick={() => setDeletingDevice(device)}
                              className="p-1.5 text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400"
                              aria-label={`Delete output device ${device.name}`}
                            >
                              <Trash2 className="w-4 h-4" />
                            </button>
                          </PaidGate>
                        </td>
                      </tr>
                      {isExpanded && (
                        <tr className="border-b border-gray-100 dark:border-gray-800 bg-gray-50/50 dark:bg-gray-800/30">
                          <td colSpan={9} className="px-3 py-4">
                            <OutputDeviceEditPanel
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

        <OutputDeviceFormModal
          isOpen={isCreateModalOpen}
          mode="create"
          onClose={() => setIsCreateModalOpen(false)}
        />

        <ConfirmModal
          isOpen={!!deletingDevice}
          title="Delete Output Device"
          message={`Are you sure you want to delete "${deletingDevice?.name}"? This action cannot be undone.`}
          onConfirm={confirmDelete}
          onCancel={() => setDeletingDevice(null)}
        />
      </div>
    </ProtectedRoute>
  );
}
