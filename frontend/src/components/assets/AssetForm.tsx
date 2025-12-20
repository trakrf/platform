import React, { useState, useEffect, useMemo } from 'react';
import type { Asset, CreateAssetRequest, UpdateAssetRequest, AssetType, TagIdentifierInput } from '@/types/assets';
import { validateDateRange, validateAssetType } from '@/lib/asset/validators';
import { ErrorBanner } from '@/components/shared';
import { useScanToInput } from '@/hooks/useScanToInput';
import { useDeviceStore, useLocationStore } from '@/stores';
import { useLocations } from '@/hooks/locations';
import { ScanLine, QrCode, X, Plus } from 'lucide-react';
import { TagIdentifierInputRow } from './TagIdentifierInputRow';

interface AssetFormProps {
  mode: 'create' | 'edit';
  asset?: Asset;
  onSubmit: (data: CreateAssetRequest | UpdateAssetRequest) => Promise<void>;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
  initialIdentifier?: string;
}

const ASSET_TYPES: Array<{ value: AssetType; label: string }> = [
  { value: 'asset', label: 'Asset' },
];

export function AssetForm({ mode, asset, onSubmit, onCancel, loading = false, error, initialIdentifier }: AssetFormProps) {
  const [formData, setFormData] = useState({
    identifier: asset?.identifier || initialIdentifier || '',
    name: asset?.name || '',
    type: asset?.type || ('asset' as AssetType),
    description: asset?.description || '',
    current_location_id: asset?.current_location_id ?? null as number | null,
    valid_from: asset?.valid_from?.split('T')[0] || new Date().toISOString().split('T')[0],
    valid_to: asset?.valid_to?.split('T')[0] || '',
    is_active: asset?.is_active ?? true,
  });

  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [tagIdentifiers, setTagIdentifiers] = useState<TagIdentifierInput[]>([]);

  // Load locations for dropdown
  useLocations({ enabled: true });
  const locationCache = useLocationStore((state) => state.cache.byId);
  const locations = useMemo(() => Array.from(locationCache.values()), [locationCache]);

  // Scanner integration
  const isConnected = useDeviceStore((s) => s.isConnected);
  const { startRfidScan, startBarcodeScan, stopScan, isScanning, scanType } = useScanToInput({
    onScan: (value) => handleChange('identifier', value),
    autoStop: true,
  });

  useEffect(() => {
    if (asset && mode === 'edit') {
      setFormData({
        identifier: asset.identifier,
        name: asset.name,
        type: asset.type,
        description: asset.description,
        current_location_id: asset.current_location_id ?? null,
        valid_from: asset.valid_from?.split('T')[0] || '',
        valid_to: asset.valid_to?.split('T')[0] || '',
        is_active: asset.is_active,
      });
      // Initialize tag identifiers from existing asset
      if (asset.identifiers && asset.identifiers.length > 0) {
        setTagIdentifiers(
          asset.identifiers.map((id) => ({
            id: id.id,
            type: 'rfid' as const,
            value: id.value,
          }))
        );
      }
    }
  }, [asset, mode]);

  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    // Required fields
    if (!formData.identifier.trim()) {
      errors.identifier = 'Identifier is required';
    } else if (!/^[a-zA-Z0-9-_]+$/.test(formData.identifier)) {
      errors.identifier = 'Identifier must contain only letters, numbers, hyphens, and underscores';
    }

    if (!formData.name.trim()) {
      errors.name = 'Name is required';
    } else if (formData.name.trim().length < 2) {
      errors.name = 'Name must be at least 2 characters';
    }

    if (!validateAssetType(formData.type)) {
      errors.type = 'Invalid asset type';
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
      if (!dateStr) return '';
      if (!dateStr.includes('T')) {
        return `${dateStr}T00:00:00Z`;
      }
      return dateStr;
    };

    // Filter out empty tag identifiers and include in request
    const validIdentifiers = tagIdentifiers.filter((id) => id.value.trim() !== '');

    const data: CreateAssetRequest | UpdateAssetRequest = {
      identifier: formData.identifier,
      name: formData.name,
      type: formData.type,
      description: formData.description,
      current_location_id: formData.current_location_id,
      valid_from: toRFC3339(formData.valid_from),
      valid_to: toRFC3339(formData.valid_to || '2099-12-31'),
      is_active: formData.is_active,
      metadata: {},
    };

    // Include identifiers for the modal to handle (modal extracts and processes separately)
    await onSubmit({ ...data, identifiers: validIdentifiers } as unknown as CreateAssetRequest | UpdateAssetRequest);
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
          <label htmlFor="identifier" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
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
            } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50 disabled:cursor-not-allowed`}
            placeholder={isScanning
              ? (scanType === 'rfid' ? 'Scanning RFID...' : 'Scanning barcode...')
              : 'e.g., LAP-001'
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

        <div>
          <label htmlFor="type" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Type <span className="text-red-500">*</span>
          </label>
          <select
            id="type"
            value={formData.type}
            onChange={(e) => handleChange('type', e.target.value as AssetType)}
            disabled={loading}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
          >
            {ASSET_TYPES.map((type) => (
              <option key={type.value} value={type.value}>
                {type.label}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label htmlFor="location" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Location
          </label>
          <select
            id="location"
            value={formData.current_location_id ?? ''}
            onChange={(e) => handleChange('current_location_id', e.target.value ? Number(e.target.value) : null)}
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

      {/* Tag Identifiers Section */}
      <div className="border-t border-gray-200 dark:border-gray-700 pt-6">
        <div className="flex items-center justify-between mb-4">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Tag Identifiers
          </label>
          <button
            type="button"
            onClick={() =>
              setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: '' }])
            }
            disabled={loading}
            className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded-lg transition-colors disabled:opacity-50"
          >
            <Plus className="w-4 h-4" />
            Add Tag
          </button>
        </div>

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
          {loading ? 'Saving...' : mode === 'create' ? 'Create Asset' : 'Update Asset'}
        </button>
      </div>
    </form>
  );
}
