import { useState, useEffect, FormEvent } from 'react';
import { validateName } from '@/lib/location/validators';
import { useLocations } from '@/hooks/locations';
import { useScanDevices } from '@/hooks/scandevices';
import { PaidGate } from '@/components/entitlement';
import type {
  OutputDevice,
  OutputDeviceType,
  OutputDeviceMode,
  AlarmTransport,
  CreateOutputDeviceRequest,
  UpdateOutputDeviceRequest,
} from '@/types/outputdevices';

// Reader-type scan devices that can host a GPO alarm output. csl_cs463 is the
// CS463 fixed reader family this ships against; over-inclusive is fine here,
// but BLE-gateway types (gl_s10, esp32_ble_generic) never qualify — they have
// no GPO ports.
const GPO_READER_TYPES = ['csl_cs463'];

const DEVICE_TYPES: { value: OutputDeviceType; label: string }[] = [
  { value: 'shelly_gen4', label: 'Shelly Gen4' },
  { value: 'csl_cs463_gpo', label: 'CS463 GPO' },
];

interface OutputDeviceFormData {
  name: string;
  type: OutputDeviceType;
  transport: AlarmTransport;
  base_url: string; // http transport
  command_topic: string; // mqtt transport (Shelly topic prefix)
  scan_device_id: string; // mqtt transport, GPO only (the chosen reader's id)
  switch_id: string; // string in the form; coerced to number on submit
  location_id: string; // optional; blank -> null (the location, not the antenna)
  is_active: boolean;
  // Rule config (TRA-943/935), persisted to metadata. Strings in the form;
  // coerced to numbers on submit. Blank = "org/system default" (omit the key).
  mode: OutputDeviceMode;
  age_out_seconds: string;
  auto_off_seconds: string;
  rssi_threshold: string; // dBm; may be negative
}

const TRANSPORTS: { value: AlarmTransport; label: string }[] = [
  { value: 'http', label: 'HTTP (local edge)' },
  { value: 'mqtt', label: 'MQTT (broker)' },
];

interface OutputDeviceFormProps {
  mode: 'create' | 'edit';
  device?: OutputDevice;
  onSubmit: (data: CreateOutputDeviceRequest | UpdateOutputDeviceRequest) => void;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}

interface FieldErrors {
  name?: string;
  base_url?: string;
  command_topic?: string;
  scan_device_id?: string;
  switch_id?: string;
  age_out_seconds?: string;
  auto_off_seconds?: string;
  rssi_threshold?: string;
}

const EMPTY_FORM: OutputDeviceFormData = {
  name: '',
  type: 'shelly_gen4',
  transport: 'http',
  base_url: '',
  command_topic: '',
  scan_device_id: '',
  switch_id: '0',
  location_id: '',
  is_active: true,
  mode: 'egress',
  age_out_seconds: '',
  auto_off_seconds: '',
  rssi_threshold: '',
};

function validateBaseURL(url: string): string | null {
  if (url.trim() === '') return 'Base URL is required';
  if (!/^https?:\/\/.+/i.test(url.trim())) return 'Base URL must start with http:// or https://';
  return null;
}

// metaNum renders a metadata numeric value as a form string ('' when unset).
function metaNum(v: unknown): string {
  return typeof v === 'number' ? String(v) : '';
}

// validateOptInt validates an optional integer field. '' is allowed (unset).
function validateOptInt(s: string, opts: { allowNegative?: boolean }): string | null {
  const t = s.trim();
  if (t === '') return null;
  const re = opts.allowNegative ? /^-?\d+$/ : /^\d+$/;
  if (!re.test(t)) return opts.allowNegative ? 'Must be an integer' : 'Must be a non-negative integer';
  return null;
}

export function OutputDeviceForm({
  mode,
  device,
  onSubmit,
  onCancel,
  loading = false,
  error = null,
}: OutputDeviceFormProps) {
  const [formData, setFormData] = useState<OutputDeviceFormData>(EMPTY_FORM);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const { locations } = useLocations();
  const { scanDevices } = useScanDevices();
  const readerDevices = scanDevices.filter((d) => GPO_READER_TYPES.includes(d.type));

  useEffect(() => {
    if (mode === 'edit' && device) {
      setFormData({
        name: device.name,
        type: device.type,
        transport: device.transport,
        base_url: device.base_url,
        command_topic: device.command_topic ?? '',
        scan_device_id: device.scan_device_id != null ? String(device.scan_device_id) : '',
        switch_id: String(device.switch_id),
        location_id: device.location_id != null ? String(device.location_id) : '',
        is_active: device.is_active,
        mode: device.metadata?.mode === 'presence' ? 'presence' : 'egress',
        age_out_seconds: metaNum(device.metadata?.age_out_seconds),
        auto_off_seconds: metaNum(device.metadata?.auto_off_seconds),
        rssi_threshold: metaNum(device.metadata?.rssi_threshold),
      });
    } else if (mode === 'create') {
      setFormData(EMPTY_FORM);
    }
  }, [mode, device]);

  const validateForm = (): boolean => {
    const errors: FieldErrors = {};

    // In edit mode, name / switch_id / location are owned by the inline row cells.
    if (mode === 'create') {
      const nameError = validateName(formData.name);
      if (nameError) errors.name = nameError;
    }

    if (formData.transport === 'http') {
      const urlError = validateBaseURL(formData.base_url);
      if (urlError) errors.base_url = urlError;
    } else if (formData.type === 'csl_cs463_gpo') {
      if (formData.scan_device_id.trim() === '') {
        errors.scan_device_id = 'Reader is required for a GPO output';
      }
    } else if (formData.command_topic.trim() === '') {
      errors.command_topic = 'Command topic is required for MQTT transport';
    }

    const rawSwitchId = formData.switch_id.trim();
    if (mode === 'create' && rawSwitchId !== '') {
      if (formData.type === 'csl_cs463_gpo') {
        const port = Number(rawSwitchId);
        if (!/^\d+$/.test(rawSwitchId) || port < 1 || port > 4) {
          errors.switch_id = 'GPO port must be between 1 and 4';
        }
      } else if (!/^\d+$/.test(rawSwitchId)) {
        errors.switch_id = 'Switch ID must be a non-negative integer';
      }
    }

    const ageErr = validateOptInt(formData.age_out_seconds, {});
    if (ageErr) errors.age_out_seconds = ageErr;
    if (formData.mode === 'egress') {
      const offErr = validateOptInt(formData.auto_off_seconds, {});
      if (offErr) errors.auto_off_seconds = offErr;
    }
    const rssiErr = validateOptInt(formData.rssi_threshold, { allowNegative: true });
    if (rssiErr) errors.rssi_threshold = rssiErr;

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleChange = <K extends keyof OutputDeviceFormData>(
    field: K,
    value: OutputDeviceFormData[K]
  ) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setFieldErrors((prev) => ({ ...prev, [field]: undefined }));
  };

  const handleTypeChange = (type: OutputDeviceType) => {
    // A GPO is reached only over mqtt-rpc; lock the transport rather than
    // letting an invalid combination reach the server.
    if (type === 'csl_cs463_gpo') {
      // GPO ports are numbered 1-4 on the reader; '0' is not a valid port, so
      // don't leave the field pre-seeded with the Shelly-relay default (M4).
      setFormData((prev) => ({ ...prev, type, transport: 'mqtt', switch_id: '1' }));
      return;
    }
    handleChange('type', type);
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!validateForm()) return;

    // Only submit the field that applies to the selected transport. Sending a
    // stale/empty base_url for mqtt trips the backend's url validation and is
    // an unrecoverable dead end since the field isn't shown (TRA-928).
    // Rule config (TRA-943/935) round-trips through the metadata jsonb. Blank
    // numeric fields are omitted (= org/system default). auto_off only applies to
    // egress — presence owns the OFF edge, so the engine ignores it.
    const metadata: Record<string, number | string> = { mode: formData.mode };
    if (formData.age_out_seconds.trim() !== '')
      metadata.age_out_seconds = parseInt(formData.age_out_seconds.trim(), 10);
    if (formData.rssi_threshold.trim() !== '')
      metadata.rssi_threshold = parseInt(formData.rssi_threshold.trim(), 10);
    if (formData.mode === 'egress' && formData.auto_off_seconds.trim() !== '')
      metadata.auto_off_seconds = parseInt(formData.auto_off_seconds.trim(), 10);

    // Cascade-coupled fields the expander form always owns (transport + its
    // target field, type, and the rule-config metadata). A GPO addresses its
    // reader by FK (scan_device_id) — the backend derives the base topic from
    // it server-side (TRA-1028) — everything else on mqtt still uses a
    // free-text command_topic (Shelly).
    const base = {
      type: formData.type,
      transport: formData.transport,
      metadata,
      ...(formData.transport === 'http'
        ? { base_url: formData.base_url.trim() }
        : formData.type === 'csl_cs463_gpo'
          ? { scan_device_id: parseInt(formData.scan_device_id.trim(), 10) }
          : { command_topic: formData.command_topic.trim() }),
    };

    if (mode === 'create') {
      const switch_id = formData.switch_id.trim() === '' ? 0 : parseInt(formData.switch_id.trim(), 10);
      const location_id =
        formData.location_id.trim() === '' ? null : parseInt(formData.location_id.trim(), 10);
      onSubmit({
        ...base,
        name: formData.name,
        switch_id,
        location_id,
        is_active: formData.is_active,
      } as CreateOutputDeviceRequest);
    } else {
      // TRA-940: name / switch_id / location_id / is_active are owned by the
      // inline row cells, so the expander PATCH omits them to avoid clobbering.
      onSubmit(base as UpdateOutputDeviceRequest);
    }
  };

  const inputClass = (hasError: boolean) =>
    `block w-full px-3 py-2 border rounded-lg ${
      hasError
        ? 'border-red-500 focus:ring-red-500'
        : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
    } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50`;

  return (
    // noValidate: all validation is custom (below), including numeric fields that
    // carry a min for UX hinting (e.g. GPO port min=1). Without this the browser's
    // native constraint validation silently swallows the submit event for an
    // out-of-range value before handleSubmit ever runs, so our error copy never renders.
    <form onSubmit={handleSubmit} noValidate className="space-y-6">
      {error && (
        <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
          <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* TRA-940: name is edited inline in the list row; keep it only for create. */}
        {mode === 'create' && (
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
        )}

        <div>
          <label htmlFor="type" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Type <span className="text-red-500">*</span>
          </label>
          <select
            id="type"
            value={formData.type}
            onChange={(e) => handleTypeChange(e.target.value as OutputDeviceType)}
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
            disabled={loading || formData.type === 'csl_cs463_gpo'}
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
      ) : formData.type === 'csl_cs463_gpo' ? (
        <div>
          <label htmlFor="scan_device_id" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Reader <span className="text-red-500">*</span>
          </label>
          <select
            id="scan_device_id"
            value={formData.scan_device_id}
            onChange={(e) => handleChange('scan_device_id', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.scan_device_id)}
          >
            <option value="">— Select a reader —</option>
            {readerDevices.map((d) => (
              <option key={d.id} value={String(d.id)}>
                {d.name}
              </option>
            ))}
          </select>
          {fieldErrors.scan_device_id && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.scan_device_id}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            The CS463 reader that owns this GPO port. The backend derives its mqtt-rpc topic from the
            reader&apos;s registration — no free-text topic to get wrong.
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
            The Shelly&apos;s MQTT topic prefix. Must match the prefix configured on the device; the backend publishes to <code>&lt;topic&gt;/command/switch:&lt;id&gt;</code>. Firewall-friendly (no inbound).
          </p>
        </div>
      )}

      {/* TRA-940: switch ID and location are edited inline in the list row;
          keep them in the form only for create. */}
      {mode === 'create' && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div>
            <label htmlFor="switch_id" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              {formData.type === 'csl_cs463_gpo' ? 'GPO port (1–4)' : 'Switch ID'}
            </label>
            <input
              type="number"
              id="switch_id"
              min={formData.type === 'csl_cs463_gpo' ? 1 : 0}
              value={formData.switch_id}
              onChange={(e) => handleChange('switch_id', e.target.value)}
              disabled={loading}
              className={inputClass(!!fieldErrors.switch_id)}
              placeholder="0"
            />
            {fieldErrors.switch_id && (
              <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.switch_id}</p>
            )}
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {formData.type === 'csl_cs463_gpo'
                ? 'GPO output port on the reader (1-4).'
                : 'Shelly relay channel (usually 0).'}
            </p>
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
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <label htmlFor="mode" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Mode
          </label>
          <select
            id="mode"
            value={formData.mode}
            onChange={(e) => handleChange('mode', e.target.value as OutputDeviceMode)}
            disabled={loading}
            className={inputClass(false)}
          >
            <option value="egress">Egress — fire on crossing, then latch</option>
            <option value="presence">Presence — on while present, off when clear</option>
          </select>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Egress alerts when an asset crosses. Presence stays on while an asset is here and clears
            it when the last one leaves.
          </p>
        </div>

        <div>
          <label
            htmlFor="rssi_threshold"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
            RSSI threshold (dBm)
          </label>
          <input
            type="number"
            id="rssi_threshold"
            value={formData.rssi_threshold}
            onChange={(e) => handleChange('rssi_threshold', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.rssi_threshold)}
            placeholder="System default"
          />
          {fieldErrors.rssi_threshold && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.rssi_threshold}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Minimum signal strength for this output to react (stronger is closer to 0). Blank = org/system
            default.
          </p>
        </div>

        <div>
          <label
            htmlFor="age_out_seconds"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
            Age-out (seconds)
          </label>
          <input
            type="number"
            id="age_out_seconds"
            min={0}
            value={formData.age_out_seconds}
            onChange={(e) => handleChange('age_out_seconds', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.age_out_seconds)}
            placeholder="System default"
          />
          {fieldErrors.age_out_seconds && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.age_out_seconds}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {formData.mode === 'presence'
              ? 'How long after the last read before the output clears.'
              : 'Re-arm window before the same tag can fire again.'}
          </p>
        </div>

        <div>
          <label
            htmlFor="auto_off_seconds"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
          >
            Auto-off (seconds)
          </label>
          <input
            type="number"
            id="auto_off_seconds"
            min={0}
            value={formData.mode === 'presence' ? '' : formData.auto_off_seconds}
            onChange={(e) => handleChange('auto_off_seconds', e.target.value)}
            disabled={loading || formData.mode === 'presence'}
            className={inputClass(!!fieldErrors.auto_off_seconds)}
            placeholder="0 = until manual reset"
          />
          {fieldErrors.auto_off_seconds && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.auto_off_seconds}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {formData.mode === 'presence'
              ? 'Managed automatically by presence detection.'
              : 'Device flips itself off after N seconds. 0 or blank = stay on until manual reset.'}
          </p>
        </div>
      </div>

      {/* TRA-940: Active is toggled inline in the list row in edit mode. */}
      {mode === 'create' && (
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
      )}

      <div className="flex justify-end gap-3 pt-4 border-t border-gray-200 dark:border-gray-700">
        <button
          type="button"
          onClick={onCancel}
          disabled={loading}
          className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
        >
          Cancel
        </button>
        <PaidGate surface="outputs-crud" silentImpression>
          <button
            type="submit"
            disabled={loading}
            className="px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
          >
            {loading ? 'Saving...' : mode === 'create' ? 'Create Output Device' : 'Update Output Device'}
          </button>
        </PaidGate>
      </div>
    </form>
  );
}
