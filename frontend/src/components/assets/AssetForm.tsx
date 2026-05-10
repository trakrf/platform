import React, { useState, useEffect, useMemo } from 'react';
import type { Asset, CreateAssetRequest, UpdateAssetRequest, TagInput } from '@/types/assets';
import { validateDateRange } from '@/lib/asset/validators';
import { ErrorBanner } from '@/components/shared';
import { useDeviceStore, useLocationStore } from '@/stores';
import { useLocations } from '@/hooks/locations';
import { useScanToInput } from '@/hooks/useScanToInput';
import { ReaderMode } from '@/worker/types/reader';
import { lookupApi } from '@/lib/api/lookup';
import { ConfirmModal } from '@/components/shared/modals/ConfirmModal';
import { Plus, QrCode, Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { TagInputRow } from './TagInputRow';

interface AssetFormProps {
  mode: 'create' | 'edit';
  asset?: Asset;
  onSubmit: (data: CreateAssetRequest | UpdateAssetRequest) => Promise<void>;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
  initialIdentifier?: string;
}

export function AssetForm({ mode, asset, onSubmit, onCancel, loading = false, error, initialIdentifier }: AssetFormProps) {
  // Resolve location_id from natural key for write path (POST/PUT unchanged)
  const getLocationByIdentifier = useLocationStore((state) => state.getLocationByIdentifier);
  const resolvedLocationId = asset?.location_external_key
    ? (getLocationByIdentifier(asset.location_external_key)?.id ?? null)
    : null;

  const [formData, setFormData] = useState({
    external_key: asset?.external_key || initialIdentifier || '',
    name: asset?.name || '',
    description: asset?.description || '',
    location_id: resolvedLocationId as number | null,
    valid_from: asset?.valid_from?.split('T')[0] || new Date().toISOString().split('T')[0],
    valid_to: asset?.valid_to?.split('T')[0] || '',
    is_active: asset?.is_active ?? true,
  });

  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [tagInputs, setTagInputs] = useState<TagInput[]>([]);

  // Load locations for dropdown
  useLocations({ enabled: true });
  const locationCache = useLocationStore((state) => state.cache.byId);
  const locations = useMemo(() => Array.from(locationCache.values()), [locationCache]);

  // Barcode scanning for RFID tags
  const isConnected = useDeviceStore((s) => s.isConnected);
  const [confirmModal, setConfirmModal] = useState<{
    isOpen: boolean;
    epc: string;
    assignedTo: string;
  } | null>(null);
  const [isScanning, setIsScanning] = useState(false);
  const [focusedTagIndex, setFocusedTagIndex] = useState<number | null>(null);
  const [autoFocusIndex, setAutoFocusIndex] = useState<number | null>(null);

  const { startBarcodeScan, stopScan, setFocused } = useScanToInput({
    onScan: (epc) => handleBarcodeScan(epc),
    onPreview: (value) => {
      // Live preview: update focused tag input value directly (no API calls)
      if (focusedTagIndex !== null) {
        setTagInputs((prev) => {
          const updated = [...prev];
          if (updated[focusedTagIndex]) {
            updated[focusedTagIndex] = { ...updated[focusedTagIndex], value };
          }
          return updated;
        });
      }
    },
    autoStop: true,
    returnMode: ReaderMode.BARCODE,
    triggerEnabled: true,
  });

  // Sync focus state with hook for trigger scanning
  useEffect(() => {
    setFocused(focusedTagIndex !== null);
  }, [focusedTagIndex, setFocused]);

  useEffect(() => {
    if (asset && mode === 'edit') {
      setFormData({
        external_key: asset.external_key,
        name: asset.name,
        description: asset.description,
        location_id: asset.location_external_key
          ? (getLocationByIdentifier(asset.location_external_key)?.id ?? null)
          : null,
        valid_from: asset.valid_from?.split('T')[0] || '',
        valid_to: asset.valid_to?.split('T')[0] || '',
        is_active: asset.is_active,
      });
      // Initialize tags from existing asset + add blank row for new entry
      const existingTags = (asset.tags || []).map((id) => ({
        id: id.id,
        type: 'rfid' as const,
        value: id.value,
      }));
      setTagInputs([...existingTags, { type: 'rfid', value: '' }]);
      // Auto-focus removed - only Add Tag button triggers focus
    } else if (mode === 'create') {
      // Start with one blank tag row for create mode
      setTagInputs([{ type: 'rfid', value: '' }]);
      // Auto-focus removed - only Add Tag button triggers focus
    }
  }, [asset, mode]);

  // Handle barcode scan for tags
  const handleBarcodeScan = async (epc: string) => {
    setIsScanning(false);

    // Validate EPC is non-empty (can happen if scanner returns only AIM identifier)
    if (!epc || epc.trim() === '') {
      toast.error('No tag data received from scanner');
      return;
    }

    // If a tag row is focused (trigger scan), update that row's value
    if (focusedTagIndex !== null && tagInputs[focusedTagIndex]) {
      // Local duplicate check (excluding current row)
      if (tagInputs.some((t, i) => i !== focusedTagIndex && t.value === epc)) {
        toast.error('This tag is already in the list');
        return;
      }

      // Cross-asset duplicate check
      try {
        const response = await lookupApi.byTag('rfid', epc);
        const result = response.data.data;
        const name =
          result.asset?.name || result.location?.name || `${result.entity_type} #${result.entity_id}`;
        setConfirmModal({ isOpen: true, epc, assignedTo: name });
      } catch (error: unknown) {
        const axiosError = error as { response?: { status: number } };
        if (axiosError.response?.status === 404) {
          // Not found = no duplicate, update focused row directly
          const updated = [...tagInputs];
          updated[focusedTagIndex] = { ...updated[focusedTagIndex], value: epc };
          setTagInputs(updated);
          toast.success('Tag updated');
        } else {
          toast.error('Failed to check tag assignment');
        }
      }
      return;
    }

    // Original behavior: append new row (button-initiated scan)
    // Local duplicate check
    if (tagInputs.some((t) => t.value === epc)) {
      toast.error('This tag is already in the list');
      return;
    }

    // Cross-asset duplicate check via lookup API
    try {
      const response = await lookupApi.byTag('rfid', epc);
      // 200 = found, show reassign confirmation
      const result = response.data.data;
      const name =
        result.asset?.name || result.location?.name || `${result.entity_type} #${result.entity_id}`;
      setConfirmModal({ isOpen: true, epc, assignedTo: name });
    } catch (error: unknown) {
      const axiosError = error as { response?: { status: number } };
      if (axiosError.response?.status === 404) {
        // Not found = no duplicate, add directly
        setTagInputs([...tagInputs, { type: 'rfid', value: epc }]);
        toast.success('Tag added');
      } else {
        toast.error('Failed to check tag assignment');
      }
    }
  };

  const handleConfirmReassign = () => {
    if (confirmModal) {
      if (focusedTagIndex !== null && tagInputs[focusedTagIndex]) {
        // Update focused row
        const updated = [...tagInputs];
        updated[focusedTagIndex] = { ...updated[focusedTagIndex], value: confirmModal.epc };
        setTagInputs(updated);
        toast.success('Tag updated (will be reassigned on save)');
      } else {
        // Original: append new row
        setTagInputs([...tagInputs, { type: 'rfid', value: confirmModal.epc }]);
        toast.success('Tag added (will be reassigned on save)');
      }
    }
    setConfirmModal(null);
  };

  const handleStartScan = () => {
    setIsScanning(true);
    startBarcodeScan();
  };

  const handleStopScan = () => {
    setIsScanning(false);
    stopScan();
  };

  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    // Asset ID is optional - backend auto-generates if empty
    // Only validate format if a value is provided
    if (formData.external_key.trim() && !/^[a-zA-Z0-9-_]+$/.test(formData.external_key)) {
      errors.external_key = 'Asset ID must contain only letters, numbers, hyphens, and underscores';
    }

    if (!formData.name.trim()) {
      errors.name = 'Name is required';
    } else if (formData.name.trim().length < 2) {
      errors.name = 'Name must be at least 2 characters';
    }

    // Date range validation
    const dateError = validateDateRange(formData.valid_from, formData.valid_to || null);
    if (dateError) {
      errors.valid_to = dateError;
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    const toRFC3339 = (dateStr: string): string => {
      if (!dateStr.includes('T')) {
        return `${dateStr}T00:00:00Z`;
      }
      return dateStr;
    };

    // Filter out empty tags and include in request
    const validTags = tagInputs.filter((id) => id.value.trim() !== '');

    // Omit external_key entirely when blank on create — the backend
    // auto-mints ASSET-NNNN only on absence; an explicit empty string is
    // rejected as 400 too_short (TRA-650 / BB23 F3).
    const trimmedExternalKey = formData.external_key.trim();
    // TRA-649 / BB23 F2: the body date validator now rejects empty strings.
    // Omit valid_from when blank so the backend applies its server default;
    // send valid_to as null when blank to clear the column.
    const data: CreateAssetRequest | UpdateAssetRequest = {
      ...(trimmedExternalKey ? { external_key: formData.external_key } : {}),
      name: formData.name,
      description: formData.description,
      location_id: formData.location_id,
      ...(formData.valid_from ? { valid_from: toRFC3339(formData.valid_from) } : {}),
      valid_to: formData.valid_to ? toRFC3339(formData.valid_to) : null,
      is_active: formData.is_active,
      metadata: {},
    };

    // Include tags for the modal to handle (modal extracts and processes separately)
    await onSubmit({ ...data, tags: validTags } as unknown as CreateAssetRequest | UpdateAssetRequest);
  };

  const handleChange = (field: string, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    if (fieldErrors[field]) {
      setFieldErrors((prev) => {
        const updated = { ...prev };
        delete updated[field];
        return updated;
      });
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {error && <ErrorBanner error={error} />}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <label htmlFor="external_key" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Asset ID
          </label>
          <input
            type="text"
            id="external_key"
            value={formData.external_key}
            onChange={(e) => handleChange('external_key', e.target.value)}
            disabled={loading}
            className={`block w-full px-3 py-2 border rounded-lg ${
              fieldErrors.external_key
                ? 'border-red-500 focus:ring-red-500'
                : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
            } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50 disabled:cursor-not-allowed`}
            placeholder="Leave blank to auto-generate"
          />
          {/* Show hint when field is empty */}
          {!formData.external_key.trim() && mode === 'create' && (
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Will be auto-generated as ASSET-XXXX
            </p>
          )}
          {fieldErrors.external_key && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.external_key}</p>
          )}
        </div>

        <div>
          <label htmlFor="name" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Name <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            id="name"
            value={formData.name}
            onChange={(e) => handleChange('name', e.target.value)}
            disabled={loading}
            className={`block w-full px-3 py-2 border rounded-lg ${
              fieldErrors.name
                ? 'border-red-500 focus:ring-red-500'
                : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
            } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50`}
            placeholder="e.g., Engineering Laptop"
          />
          {fieldErrors.name && <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.name}</p>}
        </div>
      </div>

      <div>
        <label htmlFor="description" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Description
        </label>
        <textarea
          id="description"
          value={formData.description}
          onChange={(e) => handleChange('description', e.target.value)}
          disabled={loading}
          rows={3}
          className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
          placeholder="Optional description..."
        />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <label htmlFor="location" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Location
          </label>
          <select
            id="location"
            value={formData.location_id ?? ''}
            onChange={(e) => handleChange('location_id', e.target.value ? Number(e.target.value) : null)}
            disabled={loading}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
          >
            <option value="">No location assigned</option>
            {locations
              .filter(loc => loc.is_active)
              .sort((a, b) => a.name.localeCompare(b.name))
              .map((location) => (
                <option key={location.id} value={location.id}>
                  {location.name}
                </option>
              ))}
          </select>
        </div>

        <div className="flex items-center pt-8">
          <input
            type="checkbox"
            id="is_active"
            checked={formData.is_active}
            onChange={(e) => handleChange('is_active', e.target.checked)}
            disabled={loading}
            className="h-4 w-4 text-blue-600 border-gray-300 dark:border-gray-600 rounded focus:ring-blue-500 disabled:opacity-50"
          />
          <label htmlFor="is_active" className="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">
            Active
          </label>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <label htmlFor="valid_from" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Valid From
          </label>
          <input
            type="date"
            id="valid_from"
            value={formData.valid_from}
            onChange={(e) => handleChange('valid_from', e.target.value)}
            disabled={loading}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
          />
        </div>

        <div>
          <label htmlFor="valid_to" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Valid To
          </label>
          <input
            type="date"
            id="valid_to"
            value={formData.valid_to}
            onChange={(e) => handleChange('valid_to', e.target.value)}
            disabled={loading}
            className={`block w-full px-3 py-2 border rounded-lg ${
              fieldErrors.valid_to
                ? 'border-red-500 focus:ring-red-500'
                : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
            } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50`}
          />
          {fieldErrors.valid_to && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.valid_to}</p>
          )}
        </div>
      </div>

      {/* RFID Tags Section */}
      <div className="border-t border-gray-200 dark:border-gray-700 pt-6">
        <div className="flex items-center justify-between mb-4">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            RFID Tags
          </label>
          <div className="flex items-center gap-2">
            {isConnected && (
              <button
                type="button"
                onMouseDown={(e) => {
                  // Prevent blur from firing before click - keeps focus in tag input
                  if (focusedTagIndex !== null) {
                    e.preventDefault();
                  }
                }}
                onClick={isScanning ? handleStopScan : handleStartScan}
                disabled={loading || (!isScanning && focusedTagIndex === null)}
                className={`flex items-center gap-1 px-3 py-1.5 text-sm font-medium rounded-lg transition-colors disabled:opacity-50 ${
                  isScanning
                    ? 'text-red-600 hover:text-red-700 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20'
                    : focusedTagIndex !== null
                      ? 'text-green-600 hover:text-green-700 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/20'
                      : 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
                }`}
                title={focusedTagIndex === null && !isScanning ? 'Click in a tag field first' : undefined}
              >
                {isScanning ? (
                  <>
                    <Loader2 className="w-4 h-4 animate-spin" />
                    Cancel
                  </>
                ) : (
                  <>
                    <QrCode className="w-4 h-4" />
                    Scan
                  </>
                )}
              </button>
            )}
            <button
              type="button"
              onClick={() => {
                const newIndex = tagInputs.length;
                setTagInputs([...tagInputs, { type: 'rfid', value: '' }]);
                setAutoFocusIndex(newIndex);
              }}
              disabled={loading}
              className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded-lg transition-colors disabled:opacity-50"
            >
              <Plus className="w-4 h-4" />
              Add Tag
            </button>
          </div>
        </div>

        {/* Scanning feedback */}
        {isScanning && (
          <div className="flex items-center gap-2 mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
            <Loader2 className="w-4 h-4 animate-spin text-blue-600 dark:text-blue-400" />
            <span className="text-sm text-blue-600 dark:text-blue-400">
              Scanning barcode... Point at tag barcode
            </span>
          </div>
        )}

        {tagInputs.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 italic">
            No RFID tags linked. Click &quot;Add Tag&quot; to associate RFID tag IDs.
          </p>
        ) : (
          <div className="space-y-3">
            {tagInputs.map((tagInput, index) => (
              <TagInputRow
                key={tagInput.id ?? `new-${index}`}
                type={tagInput.type}
                value={tagInput.value}
                autoFocus={index === autoFocusIndex}
                onFocus={() => {
                  setFocusedTagIndex(index);
                  setAutoFocusIndex(null); // Clear after focus fires
                }}
                onBlur={() => setFocusedTagIndex(null)}
                isFocused={focusedTagIndex === index}
                onTypeChange={(type) => {
                  const updated = [...tagInputs];
                  updated[index] = { ...updated[index], type };
                  setTagInputs(updated);
                }}
                onValueChange={(value) => {
                  const updated = [...tagInputs];
                  updated[index] = { ...updated[index], value };
                  setTagInputs(updated);
                }}
                onRemove={() => {
                  setTagInputs(tagInputs.filter((_, i) => i !== index));
                }}
                disabled={loading}
              />
            ))}
          </div>
        )}
      </div>

      <div className="flex justify-end gap-3 pt-4 border-t border-gray-200 dark:border-gray-700">
        <button
          type="button"
          onClick={onCancel}
          disabled={loading}
          className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={loading}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
        >
          {loading ? 'Saving...' : mode === 'create' ? 'Create Asset' : 'Update Asset'}
        </button>
      </div>

      {/* Reassign confirmation modal */}
      {confirmModal && (
        <ConfirmModal
          isOpen={confirmModal.isOpen}
          title="Tag Already Assigned"
          message={`This tag is currently assigned to "${confirmModal.assignedTo}". Do you want to reassign it to this asset?`}
          confirmText="Reassign"
          onConfirm={handleConfirmReassign}
          onCancel={() => setConfirmModal(null)}
        />
      )}
    </form>
  );
}
