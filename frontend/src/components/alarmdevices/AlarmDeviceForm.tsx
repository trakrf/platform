import { useState, useEffect, FormEvent } from 'react';
import { validateName } from '@/lib/location/validators';
import type {
  AlarmDevice,
  AlarmDeviceType,
  CreateAlarmDeviceRequest,
  UpdateAlarmDeviceRequest,
} from '@/types/alarmdevices';

const DEVICE_TYPES: { value: AlarmDeviceType; label: string }[] = [
  { value: 'shelly_gen4', label: 'Shelly Gen4' },
];

interface AlarmDeviceFormData {
  name: string;
  type: AlarmDeviceType;
  base_url: string;
  switch_id: string; // string in the form; coerced to number on submit
  scan_point_id: string; // optional; blank -> null
  is_active: boolean;
}

interface AlarmDeviceFormProps {
  mode: 'create' | 'edit';
  device?: AlarmDevice;
  onSubmit: (data: CreateAlarmDeviceRequest | UpdateAlarmDeviceRequest) => void;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}

interface FieldErrors {
  name?: string;
  base_url?: string;
  switch_id?: string;
  scan_point_id?: string;
}

const EMPTY_FORM: AlarmDeviceFormData = {
  name: '',
  type: 'shelly_gen4',
  base_url: '',
  switch_id: '0',
  scan_point_id: '',
  is_active: true,
};

function validateBaseURL(url: string): string | null {
  if (url.trim() === '') return 'Base URL is required';
  if (!/^https?:\/\/.+/i.test(url.trim())) return 'Base URL must start with http:// or https://';
  return null;
}

export function AlarmDeviceForm({
  mode,
  device,
  onSubmit,
  onCancel,
  loading = false,
  error = null,
}: AlarmDeviceFormProps) {
  const [formData, setFormData] = useState<AlarmDeviceFormData>(EMPTY_FORM);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});

  useEffect(() => {
    if (mode === 'edit' && device) {
      setFormData({
        name: device.name,
        type: device.type,
        base_url: device.base_url,
        switch_id: String(device.switch_id),
        scan_point_id: device.scan_point_id != null ? String(device.scan_point_id) : '',
        is_active: device.is_active,
      });
    } else if (mode === 'create') {
      setFormData(EMPTY_FORM);
    }
  }, [mode, device]);

  const validateForm = (): boolean => {
    const errors: FieldErrors = {};

    const nameError = validateName(formData.name);
    if (nameError) errors.name = nameError;

    const urlError = validateBaseURL(formData.base_url);
    if (urlError) errors.base_url = urlError;

    if (formData.switch_id.trim() !== '' && !/^\d+$/.test(formData.switch_id.trim())) {
      errors.switch_id = 'Switch ID must be a non-negative integer';
    }
    if (formData.scan_point_id.trim() !== '' && !/^\d+$/.test(formData.scan_point_id.trim())) {
      errors.scan_point_id = 'Scan point ID must be a number';
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleChange = <K extends keyof AlarmDeviceFormData>(
    field: K,
    value: AlarmDeviceFormData[K]
  ) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setFieldErrors((prev) => ({ ...prev, [field]: undefined }));
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!validateForm()) return;

    const switch_id = formData.switch_id.trim() === '' ? 0 : parseInt(formData.switch_id.trim(), 10);
    const scan_point_id =
      formData.scan_point_id.trim() === '' ? null : parseInt(formData.scan_point_id.trim(), 10);

    const common = {
      name: formData.name,
      type: formData.type,
      base_url: formData.base_url.trim(),
      switch_id,
      scan_point_id,
      is_active: formData.is_active,
    };

    if (mode === 'create') {
      onSubmit(common as CreateAlarmDeviceRequest);
    } else {
      onSubmit(common as UpdateAlarmDeviceRequest);
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
          <label htmlFor="name" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Name <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            id="name"
            value={formData.name}
            onChange={(e) => handleChange('name', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.name)}
            placeholder="e.g., Dock Door Strobe"
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
            onChange={(e) => handleChange('type', e.target.value as AlarmDeviceType)}
            disabled={loading}
            className={inputClass(false)}
          >
            {DEVICE_TYPES.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div>
        <label htmlFor="base_url" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Base URL <span className="text-red-500">*</span>
        </label>
        <input
          type="text"
          id="base_url"
          value={formData.base_url}
          onChange={(e) => handleChange('base_url', e.target.value)}
          disabled={loading}
          className={inputClass(!!fieldErrors.base_url)}
          placeholder="http://192.168.50.66"
        />
        {fieldErrors.base_url && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.base_url}</p>
        )}
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Local HTTP address of the device (Shelly Gen2+ RPC).
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <label htmlFor="switch_id" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Switch ID
          </label>
          <input
            type="number"
            id="switch_id"
            min={0}
            value={formData.switch_id}
            onChange={(e) => handleChange('switch_id', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.switch_id)}
            placeholder="0"
          />
          {fieldErrors.switch_id && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.switch_id}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">Shelly relay channel (usually 0).</p>
        </div>

        <div>
          <label
            htmlFor="scan_point_id"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
            Bound Scan Point ID
          </label>
          <input
            type="number"
            id="scan_point_id"
            value={formData.scan_point_id}
            onChange={(e) => handleChange('scan_point_id', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.scan_point_id)}
            placeholder="Optional"
          />
          {fieldErrors.scan_point_id && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.scan_point_id}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Boundary scan point that fires this device. Leave blank to manage manually.
          </p>
        </div>
      </div>

      <div className="flex items-center">
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
          {loading ? 'Saving...' : mode === 'create' ? 'Create Alarm Device' : 'Update Alarm Device'}
        </button>
      </div>
    </form>
  );
}
