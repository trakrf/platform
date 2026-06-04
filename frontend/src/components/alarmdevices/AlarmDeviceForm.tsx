import { useState, useEffect, FormEvent } from 'react';
import { validateName } from '@/lib/location/validators';
import { useLocations } from '@/hooks/locations';
import type {
  AlarmDevice,
  AlarmDeviceType,
  AlarmTransport,
  CreateAlarmDeviceRequest,
  UpdateAlarmDeviceRequest,
} from '@/types/alarmdevices';

const DEVICE_TYPES: { value: AlarmDeviceType; label: string }[] = [
  { value: 'shelly_gen4', label: 'Shelly Gen4' },
];

interface AlarmDeviceFormData {
  name: string;
  type: AlarmDeviceType;
  transport: AlarmTransport;
  base_url: string; // http transport
  command_topic: string; // mqtt transport (Shelly topic prefix)
  switch_id: string; // string in the form; coerced to number on submit
  location_id: string; // optional; blank -> null (the location, not the antenna)
  is_active: boolean;
}

const TRANSPORTS: { value: AlarmTransport; label: string }[] = [
  { value: 'http', label: 'HTTP (local edge)' },
  { value: 'mqtt', label: 'MQTT (broker)' },
];

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
  command_topic?: string;
  switch_id?: string;
}

const EMPTY_FORM: AlarmDeviceFormData = {
  name: '',
  type: 'shelly_gen4',
  transport: 'http',
  base_url: '',
  command_topic: '',
  switch_id: '0',
  location_id: '',
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
  const { locations } = useLocations();

  useEffect(() => {
    if (mode === 'edit' && device) {
      setFormData({
        name: device.name,
        type: device.type,
        transport: device.transport,
        base_url: device.base_url,
        command_topic: device.command_topic ?? '',
        switch_id: String(device.switch_id),
        location_id: device.location_id != null ? String(device.location_id) : '',
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

    if (formData.transport === 'http') {
      const urlError = validateBaseURL(formData.base_url);
      if (urlError) errors.base_url = urlError;
    } else if (formData.command_topic.trim() === '') {
      errors.command_topic = 'Command topic is required for MQTT transport';
    }

    if (formData.switch_id.trim() !== '' && !/^\d+$/.test(formData.switch_id.trim())) {
      errors.switch_id = 'Switch ID must be a non-negative integer';
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
    const location_id =
      formData.location_id.trim() === '' ? null : parseInt(formData.location_id.trim(), 10);

    const common = {
      name: formData.name,
      type: formData.type,
      transport: formData.transport,
      base_url: formData.transport === 'http' ? formData.base_url.trim() : '',
      command_topic: formData.transport === 'mqtt' ? formData.command_topic.trim() : null,
      switch_id,
      location_id,
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

        <div>
          <label htmlFor="transport" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Transport <span className="text-red-500">*</span>
          </label>
          <select
            id="transport"
            value={formData.transport}
            onChange={(e) => handleChange('transport', e.target.value as AlarmTransport)}
            disabled={loading}
            className={inputClass(false)}
          >
            {TRANSPORTS.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      {formData.transport === 'http' ? (
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
            Local HTTP address of the device (Shelly Gen2+ RPC). Only reachable when the backend is on
            the device&apos;s network.
          </p>
        </div>
      ) : (
        <div>
          <label htmlFor="command_topic" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Command Topic <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            id="command_topic"
            value={formData.command_topic}
            onChange={(e) => handleChange('command_topic', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.command_topic)}
            placeholder="trakrf.id/dock-strobe"
          />
          {fieldErrors.command_topic && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.command_topic}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            The Shelly&apos;s MQTT topic prefix. Must match the prefix configured on the device; the backend
            publishes to <code>&lt;topic&gt;/command/switch:&lt;id&gt;</code>. Firewall-friendly (no inbound).
          </p>
        </div>
      )}

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
            htmlFor="location_id"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
            Location
          </label>
          <select
            id="location_id"
            value={formData.location_id}
            onChange={(e) => handleChange('location_id', e.target.value)}
            disabled={loading}
            className={inputClass(false)}
          >
            <option value="">— None —</option>
            {locations.map((loc) => (
              <option key={loc.id} value={String(loc.id)}>
                {loc.name}
              </option>
            ))}
          </select>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Fires when an asset is seen at this location (any reader/antenna). Leave blank to manage manually.
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
