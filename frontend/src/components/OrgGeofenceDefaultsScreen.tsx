/**
 * OrgGeofenceDefaultsScreen - org-tier geofence tuning defaults (TRA-955).
 *
 * Edits the middle tier of the three-tier geofence config:
 *   system/code default  ->  ORG DEFAULT (this screen)  ->  per-output override
 *
 * Blank fields fall back to the system default (shown as the input placeholder).
 * Save is full-replace: the whole form is submitted, so a blanked field clears
 * the org override. Admin-only write; the backend enforces the same.
 */

import { useState, useEffect } from 'react';
import { ArrowLeft } from 'lucide-react';
import { useOrgStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';
import type { GeofenceDefaults, GeofenceTuning } from '@/types/org';
import toast from 'react-hot-toast';

type ModeField = '' | 'egress' | 'presence';

export default function OrgGeofenceDefaultsScreen() {
  const { currentOrg, currentRole } = useOrgStore();
  const isAdmin = currentRole === 'owner' || currentRole === 'admin';

  const [loading, setLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [system, setSystem] = useState<GeofenceTuning | null>(null);

  // Form fields are strings so blank can mean "unset". Mode '' = system default.
  const [rssi, setRssi] = useState('');
  const [ageOut, setAgeOut] = useState('');
  const [autoOff, setAutoOff] = useState('');
  const [mode, setMode] = useState<ModeField>('');

  useEffect(() => {
    if (!currentOrg) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    orgsApi
      .getGeofenceDefaults(currentOrg.id)
      .then((res) => {
        if (cancelled) return;
        const { defaults, system_defaults } = res.data.data;
        setSystem(system_defaults);
        setRssi(numToField(defaults.rssi_threshold));
        setAgeOut(numToField(defaults.age_out_seconds));
        setAutoOff(numToField(defaults.auto_off_seconds));
        setMode((defaults.mode ?? '') as ModeField);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(extractErrorMessage(err, 'Failed to load geofence defaults'));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [currentOrg]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentOrg || isSaving || !isAdmin) return;

    const clientErr = validate();
    if (clientErr) {
      setError(clientErr);
      return;
    }

    setError(null);
    setIsSaving(true);
    try {
      const body: GeofenceDefaults = {
        rssi_threshold: fieldToNum(rssi),
        age_out_seconds: fieldToNum(ageOut),
        auto_off_seconds: fieldToNum(autoOff),
        mode: mode === '' ? null : mode,
      };
      const res = await orgsApi.updateGeofenceDefaults(currentOrg.id, body);
      // Reflect the server's canonical view back into the form.
      const { defaults } = res.data.data;
      setRssi(numToField(defaults.rssi_threshold));
      setAgeOut(numToField(defaults.age_out_seconds));
      setAutoOff(numToField(defaults.auto_off_seconds));
      setMode((defaults.mode ?? '') as ModeField);
      toast.success('Geofence defaults updated');
    } catch (err: unknown) {
      setError(extractErrorMessage(err, 'Failed to update geofence defaults'));
    } finally {
      setIsSaving(false);
    }
  };

  const validate = (): string | null => {
    const r = fieldToNum(rssi);
    if (r !== null && (r < -120 || r > 0)) return 'RSSI threshold must be between -120 and 0 dBm.';
    const a = fieldToNum(ageOut);
    if (a !== null && a < 1) return 'Age-out must be at least 1 second.';
    const o = fieldToNum(autoOff);
    if (o !== null && o < 0) return 'Auto-off cannot be negative.';
    return null;
  };

  if (!currentOrg) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <h1 className="text-2xl font-semibold text-white mb-4">No Organization Selected</h1>
          <p className="text-gray-400 mb-6">Please select an organization to configure geofence defaults.</p>
          <a href="#scan" className="inline-flex items-center gap-2 text-blue-400 hover:text-blue-300">
            <ArrowLeft className="w-4 h-4" />
            Go Home
          </a>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        <div className="flex items-center gap-4 mb-2">
          <a href="#scan" className="text-gray-400 hover:text-gray-300 transition-colors">
            <ArrowLeft className="w-5 h-5" />
          </a>
          <h1 className="text-2xl font-semibold text-white">Geofence Defaults</h1>
        </div>
        <p className="text-sm text-gray-400 mb-6">
          Org-wide tuning applied to every portal unless a specific output overrides it. Blank = system default.
        </p>

        {error && (
          <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-6">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {loading ? (
          <p className="text-gray-400">Loading…</p>
        ) : (
          <form onSubmit={handleSave} className="space-y-5">
            {/* RSSI threshold */}
            <div>
              <label htmlFor="gf-rssi" className="block text-sm font-medium text-gray-300 mb-2">
                RSSI threshold (dBm)
              </label>
              <input
                id="gf-rssi"
                type="number"
                value={rssi}
                onChange={(e) => setRssi(e.target.value)}
                placeholder={system ? String(system.rssi_threshold) : 'System default'}
                disabled={!isAdmin || isSaving}
                className={inputClass}
              />
              <p className="mt-1 text-xs text-gray-500">
                Minimum signal strength for an output to react (stronger is closer to 0). Blank = system default.
              </p>
            </div>

            {/* Age-out */}
            <div>
              <label htmlFor="gf-age-out" className="block text-sm font-medium text-gray-300 mb-2">
                Age-out (seconds)
              </label>
              <input
                id="gf-age-out"
                type="number"
                min={1}
                value={ageOut}
                onChange={(e) => setAgeOut(e.target.value)}
                placeholder={system ? String(system.age_out_seconds) : 'System default'}
                disabled={!isAdmin || isSaving}
                className={inputClass}
              />
              <p className="mt-1 text-xs text-gray-500">
                Egress: re-arm window before the same tag can fire again. Presence: how long after the last read
                before the output clears. Blank = system default.
              </p>
            </div>

            {/* Auto-off */}
            <div>
              <label htmlFor="gf-auto-off" className="block text-sm font-medium text-gray-300 mb-2">
                Auto-off (seconds)
              </label>
              <input
                id="gf-auto-off"
                type="number"
                min={0}
                value={autoOff}
                onChange={(e) => setAutoOff(e.target.value)}
                placeholder={system ? String(system.auto_off_seconds) : 'System default'}
                disabled={!isAdmin || isSaving}
                className={inputClass}
              />
              <p className="mt-1 text-xs text-gray-500">
                Egress only: device flips itself off after N seconds. 0 = stay on until manual reset. Blank = system
                default.
              </p>
            </div>

            {/* Mode */}
            <div>
              <label htmlFor="gf-mode" className="block text-sm font-medium text-gray-300 mb-2">
                Mode
              </label>
              <select
                id="gf-mode"
                value={mode}
                onChange={(e) => setMode(e.target.value as ModeField)}
                disabled={!isAdmin || isSaving}
                className={inputClass}
              >
                <option value="">System default{system ? ` (${system.mode})` : ''}</option>
                <option value="egress">Egress — fire on crossing, then latch</option>
                <option value="presence">Presence — on while present, off when clear</option>
              </select>
            </div>

            {!isAdmin && (
              <p className="text-gray-500 text-sm">Only admins can edit geofence defaults.</p>
            )}

            {isAdmin && (
              <button
                type="submit"
                disabled={isSaving}
                className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isSaving ? 'Saving…' : 'Save Changes'}
              </button>
            )}
          </form>
        )}
      </div>
    </div>
  );
}

const inputClass =
  'w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500 disabled:opacity-50';

function numToField(v: number | null | undefined): string {
  return v === null || v === undefined ? '' : String(v);
}

function fieldToNum(v: string): number | null {
  const t = v.trim();
  if (t === '') return null;
  const n = Number(t);
  return Number.isFinite(n) ? n : null;
}

function extractErrorMessage(err: unknown, fallback: string): string {
  const data = (err as { response?: { data?: Record<string, unknown> } }).response?.data;
  const errorObj = (data?.error as Record<string, unknown>) || data;
  let message =
    (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
    (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
    (typeof data?.error === 'string' && data.error.trim()) ||
    (typeof (err as Error).message === 'string' && (err as Error).message.trim()) ||
    fallback;
  if (typeof message !== 'string') {
    message = JSON.stringify(message);
  }
  return message;
}
