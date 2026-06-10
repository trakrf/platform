import { useState, useEffect, FormEvent } from 'react';
import { validateName } from '@/lib/location/validators';
import { useOrgStore } from '@/stores/orgStore';
import { PaidGate } from '@/components/entitlement';
import type {
  ScanDevice,
  ScanDeviceType,
  ScanTransport,
  CreateScanDeviceRequest,
  UpdateScanDeviceRequest,
} from '@/types/scandevices';

const DEVICE_TYPES: { value: ScanDeviceType; label: string }[] = [
  { value: 'csl_cs463', label: 'CSL CS463' },
  { value: 'gl_s10', label: 'GL S10' },
  { value: 'esp32_ble_generic', label: 'ESP32 BLE (generic)' },
  { value: 'csl_cs108', label: 'CSL CS108' },
];

const TRANSPORTS: { value: ScanTransport; label: string }[] = [
  { value: 'mqtt', label: 'MQTT' },
  { value: 'web_ble', label: 'Web BLE' },
];

interface ScanDeviceFormData {
  name: string;
  type: ScanDeviceType;
  transport: ScanTransport;
  publish_topic: string;
  serial_number: string;
  model: string;
  description: string;
  is_active: boolean;
}

interface ScanDeviceFormProps {
  mode: 'create' | 'edit';
  device?: ScanDevice;
  onSubmit: (data: CreateScanDeviceRequest | UpdateScanDeviceRequest) => void;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}

interface FieldErrors {
  name?: string;
  publish_topic?: string;
}

const EMPTY_FORM: ScanDeviceFormData = {
  name: '',
  type: 'csl_cs463',
  transport: 'mqtt',
  publish_topic: '',
  serial_number: '',
  model: '',
  description: '',
  is_active: true,
};

export function ScanDeviceForm({
  mode,
  device,
  onSubmit,
  onCancel,
  loading = false,
  error = null,
}: ScanDeviceFormProps) {
  const [formData, setFormData] = useState<ScanDeviceFormData>(EMPTY_FORM);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});

  // TRA-922: every publish_topic must start with {org_slug}/. Pre-fill the
  // create form with that prefix so operators only type the device-specific tail.
  const orgSlug = useOrgStore((s) => s.currentOrg?.identifier ?? '');

  useEffect(() => {
    if (mode === 'edit' && device) {
      setFormData({
        name: device.name,
        type: device.type,
        transport: device.transport,
        publish_topic: device.publish_topic ?? '',
        serial_number: device.serial_number ?? '',
        model: device.model ?? '',
        description: device.description ?? '',
        is_active: device.is_active,
      });
    } else if (mode === 'create') {
      setFormData({ ...EMPTY_FORM, publish_topic: orgSlug ? `${orgSlug}/` : '' });
    }
  }, [mode, device, orgSlug]);

  const validateForm = (): boolean => {
    const errors: FieldErrors = {};

    const nameError = validateName(formData.name);
    if (nameError) {
      errors.name = nameError;
    }

    // publish_topic is the MQTT routing key (TRA-956) — required for an mqtt
    // device to ingest. web_ble handhelds don't publish, so it stays optional.
    if (formData.transport === 'mqtt' && formData.publish_topic.trim() === '') {
      errors.publish_topic = 'Publish topic is required for MQTT devices';
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleChange = <K extends keyof ScanDeviceFormData>(
    field: K,
    value: ScanDeviceFormData[K]
  ) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setFieldErrors((prev) => ({ ...prev, [field]: undefined }));
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    // Nullable string fields: send null when blank so the backend clears the
    // column on PATCH and stores no value on POST.
    const publish_topic = formData.publish_topic.trim() === '' ? null : formData.publish_topic.trim();
    const serial_number = formData.serial_number.trim() === '' ? null : formData.serial_number.trim();
    const model = formData.model.trim() === '' ? null : formData.model.trim();
    const description = formData.description.trim() === '' ? null : formData.description.trim();

    if (mode === 'create') {
      const submitData: CreateScanDeviceRequest = {
        name: formData.name,
        type: formData.type,
        transport: formData.transport,
        publish_topic,
        serial_number,
        model,
        description,
        is_active: formData.is_active,
      };
      onSubmit(submitData);
    } else {
      const submitData: UpdateScanDeviceRequest = {
        name: formData.name,
        type: formData.type,
        transport: formData.transport,
        publish_topic,
        serial_number,
        model,
        description,
        is_active: formData.is_active,
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
            placeholder="e.g., Dock Door Reader 1"
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
            onChange={(e) => handleChange('type', e.target.value as ScanDeviceType)}
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
            Transport
          </label>
          <select
            id="transport"
            value={formData.transport}
            onChange={(e) => handleChange('transport', e.target.value as ScanTransport)}
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

      <div>
        <label htmlFor="publish_topic" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Publish Topic {formData.transport === 'mqtt' && <span className="text-red-500">*</span>}
        </label>
        <input
          type="text"
          id="publish_topic"
          value={formData.publish_topic}
          onChange={(e) => handleChange('publish_topic', e.target.value)}
          disabled={loading}
          className={inputClass(!!fieldErrors.publish_topic)}
          placeholder={orgSlug ? `e.g., ${orgSlug}/dock-reader-1/reads` : 'e.g., your-org/dock-reader-1/reads'}
        />
        {fieldErrors.publish_topic && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.publish_topic}</p>
        )}
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          The MQTT topic this device publishes reads on — the routing key that ties wire traffic to this reader.
          {orgSlug && (
            <>
              {' '}Must start with <code className="font-mono">{orgSlug}/</code>.
            </>
          )}
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <label htmlFor="serial_number" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Serial Number
          </label>
          <input
            type="text"
            id="serial_number"
            value={formData.serial_number}
            onChange={(e) => handleChange('serial_number', e.target.value)}
            disabled={loading}
            className={inputClass(false)}
            placeholder="Optional"
          />
        </div>

        <div>
          <label htmlFor="model" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Model
          </label>
          <input
            type="text"
            id="model"
            value={formData.model}
            onChange={(e) => handleChange('model', e.target.value)}
            disabled={loading}
            className={inputClass(false)}
            placeholder="Optional"
          />
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
        <PaidGate surface="readers-crud" silentImpression>
          <button
            type="submit"
            disabled={loading}
            className="px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
          >
            {loading ? 'Saving...' : mode === 'create' ? 'Create Scan Device' : 'Update Scan Device'}
          </button>
        </PaidGate>
      </div>
    </form>
  );
}
