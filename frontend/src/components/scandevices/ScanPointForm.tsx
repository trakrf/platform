import { useState, useEffect, useMemo, FormEvent } from 'react';
import { validateIdentifier, validateName } from '@/lib/location/validators';
import { useLocations } from '@/hooks/locations/useLocations';
import type { Location } from '@/types/locations';
import type {
  ScanPoint,
  CreateScanPointRequest,
  UpdateScanPointRequest,
} from '@/types/scandevices';

/**
 * Human-readable label for a location in the scan-point picker.
 * Mirrors how locations are surfaced elsewhere (name as primary, external_key
 * as the disambiguating natural key — see LocationCard).
 */
function locationLabel(location: Location): string {
  if (location.name && location.name !== location.external_key) {
    return `${location.name} (${location.external_key})`;
  }
  return location.external_key;
}

interface ScanPointFormData {
  external_key: string;
  name: string;
  antenna_port: string;
  location_id: string;
  description: string;
}

interface ScanPointFormProps {
  mode: 'create' | 'edit';
  point?: ScanPoint;
  onSubmit: (data: CreateScanPointRequest | UpdateScanPointRequest) => void;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}

interface FieldErrors {
  external_key?: string;
  name?: string;
  antenna_port?: string;
}

const EMPTY_FORM: ScanPointFormData = {
  external_key: '',
  name: '',
  antenna_port: '',
  location_id: '',
  description: '',
};

export function ScanPointForm({
  mode,
  point,
  onSubmit,
  onCancel,
  loading = false,
  error = null,
}: ScanPointFormProps) {
  const [formData, setFormData] = useState<ScanPointFormData>(EMPTY_FORM);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});

  const { locations, isLoading: locationsLoading } = useLocations();

  const sortedLocations = useMemo(
    () => [...locations].sort((a, b) => locationLabel(a).localeCompare(locationLabel(b))),
    [locations]
  );

  useEffect(() => {
    if (mode === 'edit' && point) {
      setFormData({
        external_key: point.external_key,
        name: point.name,
        antenna_port: point.antenna_port != null ? String(point.antenna_port) : '',
        location_id: point.location_id != null ? String(point.location_id) : '',
        description: point.description ?? '',
      });
    } else if (mode === 'create') {
      setFormData(EMPTY_FORM);
    }
  }, [mode, point]);

  const validateForm = (): boolean => {
    const errors: FieldErrors = {};

    if (mode === 'create' && formData.external_key.trim() === '') {
      errors.external_key = 'External key is required';
    } else {
      const identifierError = validateIdentifier(formData.external_key);
      if (identifierError) {
        errors.external_key = identifierError;
      }
    }

    const nameError = validateName(formData.name);
    if (nameError) {
      errors.name = nameError;
    }

    if (formData.antenna_port.trim() !== '') {
      const port = Number(formData.antenna_port);
      if (!Number.isInteger(port) || port < 0) {
        errors.antenna_port = 'Antenna port must be a non-negative integer';
      }
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleChange = <K extends keyof ScanPointFormData>(
    field: K,
    value: ScanPointFormData[K]
  ) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setFieldErrors((prev) => ({ ...prev, [field]: undefined }));
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    const antenna_port = formData.antenna_port.trim() === '' ? null : Number(formData.antenna_port);
    const location_id = formData.location_id.trim() === '' ? null : Number(formData.location_id);
    const description = formData.description.trim() === '' ? null : formData.description.trim();

    if (mode === 'create') {
      const submitData: CreateScanPointRequest = {
        external_key: formData.external_key.trim(),
        name: formData.name,
        location_id,
        antenna_port,
        description,
      };
      onSubmit(submitData);
    } else {
      // external_key is immutable on PATCH — omit it.
      const submitData: UpdateScanPointRequest = {
        name: formData.name,
        location_id,
        antenna_port,
        description,
      };
      onSubmit(submitData);
    }
  };

  const inputClass = (hasError: boolean) =>
    `block w-full px-3 py-2 border rounded-lg ${
      hasError
        ? 'border-red-500 focus:ring-red-500'
        : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
    } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50`;

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
            htmlFor="sp_external_key"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
            External Key <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            id="sp_external_key"
            value={formData.external_key}
            onChange={(e) => handleChange('external_key', e.target.value)}
            disabled={loading || mode === 'edit'}
            className={inputClass(!!fieldErrors.external_key)}
            placeholder="e.g., dock_1_port_1"
          />
          {fieldErrors.external_key && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.external_key}</p>
          )}
        </div>

        <div>
          <label htmlFor="sp_name" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Name <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            id="sp_name"
            value={formData.name}
            onChange={(e) => handleChange('name', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.name)}
            placeholder="e.g., Dock Door 1 - Port 1"
          />
          {fieldErrors.name && <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.name}</p>}
        </div>

        <div>
          <label htmlFor="sp_antenna_port" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Antenna Port
          </label>
          <input
            type="number"
            id="sp_antenna_port"
            min={0}
            value={formData.antenna_port}
            onChange={(e) => handleChange('antenna_port', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.antenna_port)}
            placeholder="Optional"
          />
          {fieldErrors.antenna_port && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.antenna_port}</p>
          )}
        </div>

        <div>
          <label htmlFor="sp_location_id" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Location
          </label>
          <select
            id="sp_location_id"
            value={formData.location_id}
            onChange={(e) => handleChange('location_id', e.target.value)}
            disabled={loading || locationsLoading}
            className={inputClass(false)}
          >
            {locationsLoading ? (
              <option value="">Loading locations…</option>
            ) : (
              <>
                <option value="">— None —</option>
                {sortedLocations.map((location) => (
                  <option key={location.id} value={String(location.id)}>
                    {locationLabel(location)}
                  </option>
                ))}
              </>
            )}
          </select>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Optional but recommended — the zone this antenna is mounted in.
          </p>
        </div>
      </div>

      <div>
        <label htmlFor="sp_description" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Description
        </label>
        <textarea
          id="sp_description"
          value={formData.description}
          onChange={(e) => handleChange('description', e.target.value)}
          disabled={loading}
          rows={2}
          className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
          placeholder="Optional description..."
        />
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
          {loading ? 'Saving...' : mode === 'create' ? 'Add Scan Point' : 'Update Scan Point'}
        </button>
      </div>
    </form>
  );
}
