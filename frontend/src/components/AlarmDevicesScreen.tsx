import { useState, useEffect } from 'react';
import { Plus, Pencil, Trash2, Zap, Power } from 'lucide-react';
import toast from 'react-hot-toast';
import { useAlarmDevices, useAlarmDeviceMutations } from '@/hooks/alarmdevices';
import { useLocations } from '@/hooks/locations';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { useUIStore } from '@/stores';
import { ConfirmModal } from '@/components/shared';
import { AlarmDeviceFormModal } from '@/components/alarmdevices';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import type { AlarmDevice } from '@/types/alarmdevices';

export default function AlarmDevicesScreen() {
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [editingDevice, setEditingDevice] = useState<AlarmDevice | null>(null);
  const [deletingDevice, setDeletingDevice] = useState<AlarmDevice | null>(null);
  const [busyId, setBusyId] = useState<number | null>(null);

  const { alarmDevices, isLoading } = useAlarmDevices();
  const { locations } = useLocations();
  const { delete: deleteAlarmDevice, test, reset } = useAlarmDeviceMutations();
  const { setActiveTab } = useUIStore();

  const locationName = (id?: number | null) =>
    id == null ? '—' : (locations.find((l) => l.id === id)?.name ?? `#${id}`);

  useEffect(() => {
    setActiveTab('alarm-devices');
  }, [setActiveTab]);

  const confirmDelete = async () => {
    if (!deletingDevice) return;
    try {
      await deleteAlarmDevice(deletingDevice.id);
      toast.success(`Alarm device "${deletingDevice.name}" deleted successfully`);
      setDeletingDevice(null);
    } catch (error) {
      toast.error(getApiErrorMessage(error, 'Failed to delete alarm device'));
    }
  };

  const handleTest = async (device: AlarmDevice) => {
    setBusyId(device.id);
    try {
      await test(device.id);
      toast.success(`Test-fired "${device.name}"`);
    } catch (error) {
      toast.error(getApiErrorMessage(error, 'Alarm device unreachable'));
    } finally {
      setBusyId(null);
    }
  };

  const handleReset = async (device: AlarmDevice) => {
    setBusyId(device.id);
    try {
      await reset(device.id);
      toast.success(`Reset "${device.name}"`);
    } catch (error) {
      toast.error(getApiErrorMessage(error, 'Alarm device unreachable'));
    } finally {
      setBusyId(null);
    }
  };

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Alarm Devices</h1>
          <button
            type="button"
            onClick={() => setIsCreateModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 transition-colors"
          >
            <Plus className="w-4 h-4" />
            New Alarm Device
          </button>
        </div>

        <div className="flex-1 overflow-auto border border-gray-200 dark:border-gray-700 rounded-lg">
          {isLoading ? (
            <p className="p-4 text-sm text-gray-500 dark:text-gray-400">Loading alarm devices…</p>
          ) : alarmDevices.length === 0 ? (
            <p className="p-4 text-sm text-gray-600 dark:text-gray-400">
              No alarm devices yet. Create one to wire a Shelly relay.
            </p>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr className="text-left text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-gray-700">
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
                {alarmDevices.map((device) => (
                  <tr
                    key={device.id}
                    className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50"
                  >
                    <td className="py-2 px-3 text-gray-900 dark:text-gray-100">{device.name}</td>
                    <td className="px-3 text-gray-700 dark:text-gray-300">{device.type}</td>
                    <td className="px-3 text-gray-700 dark:text-gray-300">{device.transport}</td>
                    <td className="px-3 font-mono text-xs text-gray-700 dark:text-gray-300">
                      {device.transport === 'mqtt' ? (device.command_topic || '—') : device.base_url}
                    </td>
                    <td className="px-3 text-gray-700 dark:text-gray-300">{device.switch_id}</td>
                    <td className="px-3 text-gray-700 dark:text-gray-300">{locationName(device.location_id)}</td>
                    <td className="px-3 text-gray-700 dark:text-gray-300">{device.is_active ? 'Yes' : 'No'}</td>
                    <td className="px-3 text-right whitespace-nowrap">
                      <button
                        type="button"
                        onClick={() => handleTest(device)}
                        disabled={busyId === device.id}
                        className="p-1.5 text-gray-500 hover:text-amber-600 dark:text-gray-400 dark:hover:text-amber-400 disabled:opacity-40"
                        aria-label={`Test-fire alarm device ${device.name}`}
                        title="Test-fire"
                      >
                        <Zap className="w-4 h-4" />
                      </button>
                      <button
                        type="button"
                        onClick={() => handleReset(device)}
                        disabled={busyId === device.id}
                        className="p-1.5 text-gray-500 hover:text-green-600 dark:text-gray-400 dark:hover:text-green-400 disabled:opacity-40"
                        aria-label={`Reset alarm device ${device.name}`}
                        title="Reset (off)"
                      >
                        <Power className="w-4 h-4" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setEditingDevice(device)}
                        className="p-1.5 text-gray-500 hover:text-blue-600 dark:text-gray-400 dark:hover:text-blue-400"
                        aria-label={`Edit alarm device ${device.name}`}
                      >
                        <Pencil className="w-4 h-4" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setDeletingDevice(device)}
                        className="p-1.5 text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400"
                        aria-label={`Delete alarm device ${device.name}`}
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <AlarmDeviceFormModal
          isOpen={isCreateModalOpen}
          mode="create"
          onClose={() => setIsCreateModalOpen(false)}
        />

        {editingDevice && (
          <AlarmDeviceFormModal
            isOpen={true}
            mode="edit"
            device={editingDevice}
            onClose={() => setEditingDevice(null)}
          />
        )}

        <ConfirmModal
          isOpen={!!deletingDevice}
          title="Delete Alarm Device"
          message={`Are you sure you want to delete "${deletingDevice?.name}"? This action cannot be undone.`}
          onConfirm={confirmDelete}
          onCancel={() => setDeletingDevice(null)}
        />
      </div>
    </ProtectedRoute>
  );
}
