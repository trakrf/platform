import React, { useState, useEffect } from 'react';
import type { Asset, CreateAssetRequest, UpdateAssetRequest, AssetType } from '@/types/assets';
import { validateDateRange, validateAssetType } from '@/lib/asset/validators';
import { ErrorBanner } from '@/components/shared';

interface AssetFormProps {
  mode: 'create' | 'edit';
  asset?: Asset;
  onSubmit: (data: CreateAssetRequest | UpdateAssetRequest) => Promise<void>;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}

const ASSET_TYPES: Array<{ value: AssetType; label: string }> = [
  { value: 'person', label: 'Person' },
  { value: 'device', label: 'Device' },
  { value: 'asset', label: 'Asset' },
  { value: 'inventory', label: 'Inventory' },
  { value: 'other', label: 'Other' },
];

export function AssetForm({ mode, asset, onSubmit, onCancel, loading = false, error }: AssetFormProps) {
  const [formData, setFormData] = useState({
    identifier: asset?.identifier || '',
    name: asset?.name || '',
    type: asset?.type || ('device' as AssetType),
    description: asset?.description || '',
    valid_from: asset?.valid_from?.split('T')[0] || new Date().toISOString().split('T')[0],
    valid_to: asset?.valid_to?.split('T')[0] || '',
    is_active: asset?.is_active ?? true,
  });

  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (asset && mode === 'edit') {
      setFormData({
        identifier: asset.identifier,
        name: asset.name,
        type: asset.type,
        description: asset.description,
        valid_from: asset.valid_from?.split('T')[0] || '',
        valid_to: asset.valid_to?.split('T')[0] || '',
        is_active: asset.is_active,
      });
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

    const data: CreateAssetRequest | UpdateAssetRequest = {
      identifier: formData.identifier,
      name: formData.name,
      type: formData.type,
      description: formData.description,
      valid_from: formData.valid_from,
      valid_to: formData.valid_to || new Date('2099-12-31').toISOString().split('T')[0],
      is_active: formData.is_active,
      metadata: {},
    };

    await onSubmit(data);
  };

  const handleChange = (field: string, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    // Clear field error when user starts typing
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

      {/* Two-column grid on desktop */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Identifier */}
        <div>
          <label htmlFor="identifier" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
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
            } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50 disabled:cursor-not-allowed`}
            placeholder="e.g., LAP-001"
          />
          {fieldErrors.identifier && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.identifier}</p>
          )}
        </div>

        {/* Name */}
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

        {/* Type */}
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

        {/* Is Active */}
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

      {/* Description (full width) */}
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

      {/* Date Range */}
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

      {/* Form Actions */}
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
