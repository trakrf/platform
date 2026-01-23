import { useState, useEffect, FormEvent } from 'react';
import { validateIdentifier, validateName } from '@/lib/location/validators';
import { LocationParentSelector } from './LocationParentSelector';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location, TagIdentifierInput } from '@/types/locations';
import { useScanToInput } from '@/hooks/useScanToInput';
import { useDeviceStore } from '@/stores';
import { lookupApi } from '@/lib/api/lookup';
import { ConfirmModal } from '@/components/shared/modals/ConfirmModal';
import { Plus, QrCode, Loader2, MapPin } from 'lucide-react';
import toast from 'react-hot-toast';
import { TagIdentifierInputRow } from '@/components/assets';

interface LocationFormData {
  identifier: string;
  name: string;
  description: string;
  parent_location_id: number | null;
  valid_from: string;
  valid_to: string;
  is_active: boolean;
}

interface LocationFormProps {
  mode: 'create' | 'edit';
  location?: Location;
  parentLocationId?: number | null;
  onSubmit: (data: LocationFormData) => void;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}

interface FieldErrors {
  identifier?: string;
  name?: string;
  valid_to?: string;
}

function formatDateForInput(dateString: string | undefined | null): string {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    if (isNaN(date.getTime())) return '';
    return date.toISOString().split('T')[0];
  } catch {
    return '';
  }
}

function formatDateToRFC3339(dateString: string): string {
  if (!dateString) return '';
  const date = new Date(dateString);
  return date.toISOString();
}

export function LocationForm({
  mode,
  location,
  parentLocationId,
  onSubmit,
  onCancel,
  loading = false,
  error = null,
}: LocationFormProps) {
  const [formData, setFormData] = useState<LocationFormData>({
    identifier: '',
    name: '',
    description: '',
    parent_location_id: null,
    valid_from: '',
    valid_to: '',
    is_active: true,
  });

  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [tagIdentifiers, setTagIdentifiers] = useState<TagIdentifierInput[]>([]);

  // Get parent location info for context message
  const getLocationById = useLocationStore((state) => state.getLocationById);
  const parentLocation = parentLocationId ? getLocationById(parentLocationId) : null;

  // Barcode scanning for tag identifiers
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
    autoStop: true,
    triggerEnabled: true,
  });

  // Sync focus state with hook for trigger scanning
  useEffect(() => {
    setFocused(focusedTagIndex !== null);
  }, [focusedTagIndex, setFocused]);

  useEffect(() => {
    if (mode === 'edit' && location) {
      setFormData({
        identifier: location.identifier,
        name: location.name,
        description: location.description || '',
        parent_location_id: location.parent_location_id,
        valid_from: formatDateForInput(location.valid_from),
        valid_to: formatDateForInput(location.valid_to),
        is_active: location.is_active,
      });
      // Initialize tag identifiers from existing location + add blank row for new entry
      const existingTags = (location.identifiers || []).map((id) => ({
        id: id.id,
        type: 'rfid' as const,
        value: id.value,
      }));
      setTagIdentifiers([...existingTags, { type: 'rfid', value: '' }]);
      setAutoFocusIndex(existingTags.length); // Focus the new blank row
    } else if (mode === 'create') {
      // Reset form data for create mode
      setFormData({
        identifier: '',
        name: '',
        description: '',
        parent_location_id: parentLocationId ?? null,
        valid_from: '',
        valid_to: '',
        is_active: true,
      });
      // Start with one blank tag row for create mode
      setTagIdentifiers([{ type: 'rfid', value: '' }]);
      setAutoFocusIndex(0);
    }
  }, [mode, location, parentLocationId]);

  // Handle barcode scan for tag identifiers
  const handleBarcodeScan = async (epc: string) => {
    setIsScanning(false);

    // Validate EPC is non-empty (can happen if scanner returns only AIM identifier)
    if (!epc || epc.trim() === '') {
      toast.error('No tag data received from scanner');
      return;
    }

    // If a tag row is focused (trigger scan), update that row's value
    if (focusedTagIndex !== null && tagIdentifiers[focusedTagIndex]) {
      // Local duplicate check (excluding current row)
      if (tagIdentifiers.some((t, i) => i !== focusedTagIndex && t.value === epc)) {
        toast.error('This tag is already in the list');
        return;
      }

      // Cross-entity duplicate check
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
          const updated = [...tagIdentifiers];
          updated[focusedTagIndex] = { ...updated[focusedTagIndex], value: epc };
          setTagIdentifiers(updated);
          toast.success('Tag updated');
        } else {
          toast.error('Failed to check tag assignment');
        }
      }
      return;
    }

    // Original behavior: append new row (button-initiated scan)
    // Local duplicate check
    if (tagIdentifiers.some((t) => t.value === epc)) {
      toast.error('This tag is already in the list');
      return;
    }

    // Cross-entity duplicate check via lookup API
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
        setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: epc }]);
        toast.success('Tag added');
      } else {
        toast.error('Failed to check tag assignment');
      }
    }
  };

  const handleConfirmReassign = () => {
    if (confirmModal) {
      if (focusedTagIndex !== null && tagIdentifiers[focusedTagIndex]) {
        // Update focused row
        const updated = [...tagIdentifiers];
        updated[focusedTagIndex] = { ...updated[focusedTagIndex], value: confirmModal.epc };
        setTagIdentifiers(updated);
        toast.success('Tag updated (will be reassigned on save)');
      } else {
        // Original: append new row
        setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: confirmModal.epc }]);
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
    const errors: FieldErrors = {};

    const identifierError = validateIdentifier(formData.identifier);
    if (identifierError) {
      errors.identifier = identifierError;
    }

    const nameError = validateName(formData.name);
    if (nameError) {
      errors.name = nameError;
    }

    if (formData.valid_from && formData.valid_to) {
      const fromDate = new Date(formData.valid_from);
      const toDate = new Date(formData.valid_to);
      if (toDate < fromDate) {
        errors.valid_to = 'Valid To must be after Valid From';
      }
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleChange = <K extends keyof LocationFormData>(
    field: K,
    value: LocationFormData[K]
  ) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setFieldErrors((prev) => ({ ...prev, [field]: undefined }));
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    // Filter out empty tag identifiers
    const validIdentifiers = tagIdentifiers.filter((id) => id.value.trim() !== '');

    const submitData = {
      ...formData,
      valid_from: formData.valid_from ? formatDateToRFC3339(formData.valid_from) : '',
      valid_to: formData.valid_to ? formatDateToRFC3339(formData.valid_to) : '',
      identifiers: validIdentifiers,
    };

    onSubmit(submitData as LocationFormData);
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {error && (
        <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
          <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <label
            htmlFor="identifier"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
            Identifier <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            id="identifier"
            value={formData.identifier}
            onChange={(e) => handleChange('identifier', e.target.value)}
            disabled={loading || mode === 'edit'}
            className={`block w-full px-3 py-2 border rounded-lg ${
              fieldErrors.identifier
                ? 'border-red-500 focus:ring-red-500'
                : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
            } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50`}
            placeholder="e.g., warehouse_a"
          />
          {fieldErrors.identifier && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.identifier}</p>
          )}

          {mode === 'create' && (
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
              Letters, numbers, hyphens, and underscores only (no spaces)
            </p>
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
            placeholder="e.g., Main Warehouse"
          />
          {fieldErrors.name && <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.name}</p>}
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

      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Parent Location
        </label>
        {mode === 'create' ? (
          <div className="p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
            {parentLocationId && parentLocation ? (
              <div className="flex items-center gap-2">
                <MapPin className="h-4 w-4 text-blue-600 dark:text-blue-400" />
                <span className="text-sm text-blue-700 dark:text-blue-300">
                  Creating inside: <span className="font-medium">{parentLocation.identifier}</span>
                </span>
              </div>
            ) : (
              <span className="text-sm text-blue-700 dark:text-blue-300">
                Creating a top-level location
              </span>
            )}
          </div>
        ) : (
          <>
            <LocationParentSelector
              value={formData.parent_location_id}
              onChange={(value) => handleChange('parent_location_id', value)}
              currentLocationId={location?.id}
              disabled={loading}
            />
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
              Select a parent location or leave as root
            </p>
          </>
        )}
      </div>

      <div>
        <label
          htmlFor="description"
          className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
        >
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
          <label
            htmlFor="valid_from"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
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
          <label
            htmlFor="valid_to"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
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

      {/* Tag Identifiers Section */}
      <div className="border-t border-gray-200 dark:border-gray-700 pt-6">
        <div className="flex items-center justify-between mb-4">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Tag Identifiers
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
                const newIndex = tagIdentifiers.length;
                setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: '' }]);
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

        {tagIdentifiers.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 italic">
            No tag identifiers added. Click &quot;Add Tag&quot; to link RFID tags.
          </p>
        ) : (
          <div className="space-y-3">
            {tagIdentifiers.map((identifier, index) => (
              <TagIdentifierInputRow
                key={identifier.id ?? `new-${index}`}
                type={identifier.type}
                value={identifier.value}
                autoFocus={index === autoFocusIndex}
                onFocus={() => {
                  setFocusedTagIndex(index);
                  setAutoFocusIndex(null); // Clear after focus fires
                }}
                onBlur={() => setFocusedTagIndex(null)}
                isFocused={focusedTagIndex === index}
                onTypeChange={(type) => {
                  const updated = [...tagIdentifiers];
                  updated[index] = { ...updated[index], type };
                  setTagIdentifiers(updated);
                }}
                onValueChange={(value) => {
                  const updated = [...tagIdentifiers];
                  updated[index] = { ...updated[index], value };
                  setTagIdentifiers(updated);
                }}
                onRemove={() => {
                  setTagIdentifiers(tagIdentifiers.filter((_, i) => i !== index));
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
          {loading ? 'Saving...' : mode === 'create' ? 'Create Location' : 'Update Location'}
        </button>
      </div>

      {/* Reassign confirmation modal */}
      {confirmModal && (
        <ConfirmModal
          isOpen={confirmModal.isOpen}
          title="Tag Already Assigned"
          message={`This tag is currently assigned to "${confirmModal.assignedTo}". Do you want to reassign it to this location?`}
          confirmText="Reassign"
          onConfirm={handleConfirmReassign}
          onCancel={() => setConfirmModal(null)}
        />
      )}
    </form>
  );
}
