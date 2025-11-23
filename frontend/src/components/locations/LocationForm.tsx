import { useState, useEffect, FormEvent } from 'react';
import { validateIdentifier, validateName } from '@/lib/location/validators';
import { LocationParentSelector } from './LocationParentSelector';
import type { Location } from '@/types/locations';
import { useScanToInput } from '@/hooks/useScanToInput';
import { useDeviceStore } from '@/stores';
import { ScanLine, QrCode, X } from 'lucide-react';

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

  // Scanner integration
  const isConnected = useDeviceStore((s) => s.isConnected);
  const { startRfidScan, startBarcodeScan, stopScan, isScanning, scanType } = useScanToInput({
    onScan: (value) => handleChange('identifier', value),
    autoStop: true,
  });

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
    }
  }, [mode, location]);

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

    const submitData: LocationFormData = {
      ...formData,
      valid_from: formData.valid_from ? formatDateToRFC3339(formData.valid_from) : '',
      valid_to: formData.valid_to ? formatDateToRFC3339(formData.valid_to) : '',
    };

    onSubmit(submitData);
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
            disabled={loading || mode === 'edit' || isScanning}
            className={`block w-full px-3 py-2 border rounded-lg ${
              fieldErrors.identifier
                ? 'border-red-500 focus:ring-red-500'
                : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
            } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50`}
            placeholder={isScanning
              ? (scanType === 'rfid' ? 'Scanning RFID...' : 'Scanning barcode...')
              : 'e.g., warehouse_a'
            }
          />
          {fieldErrors.identifier && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.identifier}</p>
          )}

          {/* Scanner buttons - only show in create mode when device connected */}
          {mode === 'create' && isConnected && !isScanning && (
            <div className="flex gap-2 mt-2">
              <button
                type="button"
                onClick={startRfidScan}
                disabled={loading}
                className="flex items-center gap-2 px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-lg disabled:opacity-50 transition-colors"
              >
                <ScanLine className="w-4 h-4" />
                Scan RFID
              </button>
              <button
                type="button"
                onClick={startBarcodeScan}
                disabled={loading}
                className="flex items-center gap-2 px-3 py-1.5 text-xs font-medium text-white bg-green-600 hover:bg-green-700 rounded-lg disabled:opacity-50 transition-colors"
              >
                <QrCode className="w-4 h-4" />
                Scan Barcode
              </button>
            </div>
          )}

          {/* Scanning state feedback */}
          {isScanning && (
            <div className="flex items-center gap-2 mt-2">
              <p className="text-sm text-blue-600 dark:text-blue-400">
                {scanType === 'rfid' ? 'Scanning for RFID tag...' : 'Scanning for barcode...'}
              </p>
              <button
                type="button"
                onClick={stopScan}
                className="flex items-center gap-1 px-2 py-1 text-xs font-medium text-white bg-red-600 hover:bg-red-700 rounded-lg transition-colors"
              >
                <X className="w-3 h-3" />
                Cancel
              </button>
            </div>
          )}

          {mode === 'create' && !isScanning && (
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
              Lowercase letters, numbers, and underscores only
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
        <LocationParentSelector
          value={formData.parent_location_id}
          onChange={(value) => handleChange('parent_location_id', value)}
          currentLocationId={mode === 'edit' && location ? location.id : undefined}
          disabled={loading}
        />
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Select a parent location or leave as root
        </p>
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
    </form>
  );
}
