import { useMemo, useState } from 'react';
import { Plus, Pencil, Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { useScanPoints, useScanPointMutations } from '@/hooks/scandevices';
import { useLocations } from '@/hooks/locations/useLocations';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { ConfirmModal } from '@/components/shared';
import { ScanPointForm } from './ScanPointForm';
import type {
  ScanPoint,
  CreateScanPointRequest,
  UpdateScanPointRequest,
} from '@/types/scandevices';

interface ScanPointsPanelProps {
  deviceId: number;
}

export function ScanPointsPanel({ deviceId }: ScanPointsPanelProps) {
  const { scanPoints, isLoading } = useScanPoints(deviceId);
  const { create, update, delete: deletePoint } = useScanPointMutations(deviceId);
  const { locations } = useLocations();

  // Map location_id → display name so the antenna's 1:1 location assignment is
  // visible inline (the geofence-relevant attribute per TRA-931).
  const locationName = useMemo(() => {
    const byId = new Map(locations.map((l) => [l.id, l.name || l.external_key]));
    return (id: number | null | undefined) => (id != null ? byId.get(id) : undefined);
  }, [locations]);

  const [formMode, setFormMode] = useState<'create' | 'edit' | null>(null);
  const [editingPoint, setEditingPoint] = useState<ScanPoint | null>(null);
  const [deletingPoint, setDeletingPoint] = useState<ScanPoint | null>(null);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const openCreate = () => {
    setEditingPoint(null);
    setFormError(null);
    setFormMode('create');
  };

  const openEdit = (point: ScanPoint) => {
    setEditingPoint(point);
    setFormError(null);
    setFormMode('edit');
  };

  const closeForm = () => {
    setFormMode(null);
    setEditingPoint(null);
    setFormError(null);
  };

  const handleSubmit = async (data: CreateScanPointRequest | UpdateScanPointRequest) => {
    setSaving(true);
    setFormError(null);
    try {
      if (formMode === 'create') {
        const created = await create(data as CreateScanPointRequest);
        toast.success(`Scan point "${created.external_key}" added successfully`);
      } else if (formMode === 'edit' && editingPoint) {
        const updated = await update({ id: editingPoint.id, updates: data as UpdateScanPointRequest });
        toast.success(`Scan point "${updated.external_key}" updated successfully`);
      }
      closeForm();
    } catch (err) {
      setFormError(getApiErrorMessage(err, 'Failed to save scan point. Please try again.'));
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!deletingPoint) return;
    try {
      await deletePoint(deletingPoint.id);
      toast.success(`Scan point "${deletingPoint.external_key}" deleted successfully`);
      setDeletingPoint(null);
    } catch (err) {
      toast.error(getApiErrorMessage(err, 'Failed to delete scan point'));
    }
  };

  return (
    <div className="bg-gray-50 dark:bg-gray-800/50 border-t border-gray-200 dark:border-gray-700 p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">Scan Points</h3>
        <button
          type="button"
          onClick={openCreate}
          className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded-lg transition-colors"
        >
          <Plus className="w-4 h-4" />
          Add Scan Point
        </button>
      </div>

      {isLoading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading scan points…</p>
      ) : scanPoints.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400 italic">
          No scan points yet. Add one to map this device&apos;s antennas to locations.
        </p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left border-b border-gray-200 dark:border-gray-700 text-gray-500 dark:text-gray-400">
              <th className="py-2 font-medium">External Key</th>
              <th className="font-medium">Name</th>
              <th className="font-medium">Location</th>
              <th className="font-medium">Antenna Port</th>
              <th className="font-medium">Active</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {scanPoints.map((sp) => (
              <tr key={sp.id} className="border-b border-gray-100 dark:border-gray-800">
                <td className="py-2 font-mono text-xs text-gray-900 dark:text-gray-100">{sp.external_key}</td>
                <td className="text-gray-900 dark:text-gray-100">{sp.name}</td>
                <td className="text-gray-700 dark:text-gray-300">{locationName(sp.location_id) ?? '—'}</td>
                <td className="text-gray-700 dark:text-gray-300">{sp.antenna_port ?? '—'}</td>
                <td className="text-gray-700 dark:text-gray-300">{sp.is_active ? 'Yes' : 'No'}</td>
                <td className="text-right whitespace-nowrap">
                  <button
                    type="button"
                    onClick={() => openEdit(sp)}
                    className="p-1.5 text-gray-500 hover:text-blue-600 dark:text-gray-400 dark:hover:text-blue-400"
                    aria-label={`Edit scan point ${sp.external_key}`}
                  >
                    <Pencil className="w-4 h-4" />
                  </button>
                  <button
                    type="button"
                    onClick={() => setDeletingPoint(sp)}
                    className="p-1.5 text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400"
                    aria-label={`Delete scan point ${sp.external_key}`}
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {formMode && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 p-4"
          onClick={(e) => {
            if (e.target === e.currentTarget && !saving) closeForm();
          }}
        >
          <div className="relative w-full max-w-2xl bg-white dark:bg-gray-900 rounded-lg shadow-xl max-h-[90vh] overflow-y-auto">
            <div className="border-b border-gray-200 dark:border-gray-700 px-6 py-4">
              <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
                {formMode === 'create'
                  ? 'Add Scan Point'
                  : `Edit Scan Point: ${editingPoint?.external_key}`}
              </h2>
            </div>
            <div className="px-6 py-6">
              <ScanPointForm
                mode={formMode}
                point={editingPoint ?? undefined}
                onSubmit={handleSubmit}
                onCancel={closeForm}
                loading={saving}
                error={formError}
              />
            </div>
          </div>
        </div>
      )}

      <ConfirmModal
        isOpen={!!deletingPoint}
        title="Delete Scan Point"
        message={`Are you sure you want to delete "${deletingPoint?.external_key}"? This action cannot be undone.`}
        onConfirm={confirmDelete}
        onCancel={() => setDeletingPoint(null)}
      />
    </div>
  );
}
