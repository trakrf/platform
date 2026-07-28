/**
 * OrgCapabilitiesSection — superadmin-only capability grant controls (TRA-1027).
 *
 * A checkbox per capability in the backend's vocabulary, checked when the org
 * holds the grant, plus a Save that submits the whole set. Grant = checked,
 * revoke = unchecked; the backend applies the difference (ADR 0002 §4).
 *
 * The checkbox list is built from the `available` array the API returns, never
 * from a list in this file. A capability the backend knows and the frontend
 * does not would otherwise be ungrantable, and would look exactly like a
 * capability nobody has been granted yet.
 *
 * Visibility is the caller's responsibility — OrgSettingsScreen only mounts this
 * for a superadmin. The backend independently rejects non-superadmins (403), so
 * this is a UI affordance, not the security boundary. Nothing here is gated on a
 * capability: the surface that grants capabilities can never require one.
 */

import { useEffect, useState } from 'react';
import { orgsApi } from '@/lib/api/orgs';
import { extractErrorMessage } from '@/lib/asset/helpers';
import toast from 'react-hot-toast';

interface OrgCapabilitiesSectionProps {
  orgId: number;
  /** Called with the saved set, for callers that mirror it elsewhere. */
  onSaved?: (capabilities: string[]) => void;
}

/**
 * Display label for a capability name. Names are lower_snake_case workflow
 * words (ADR 0002), so title-casing them is enough — and it keeps a new backend
 * capability readable here without a frontend change.
 */
function capabilityLabel(name: string): string {
  const words = name.replace(/_/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export function OrgCapabilitiesSection({ orgId, onSaved }: OrgCapabilitiesSectionProps) {
  const [available, setAvailable] = useState<string[] | null>(null);
  const [granted, setGranted] = useState<string[]>([]);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    setError(null);
    setAvailable(null);
    orgsApi
      .getOrgCapabilities(orgId)
      .then((res) => {
        if (!active) return;
        setGranted(res.data.data.capabilities ?? []);
        setAvailable(res.data.data.available ?? []);
      })
      .catch((err: unknown) => {
        if (active) setError(extractErrorMessage(err, 'Failed to load capabilities'));
      });
    return () => {
      active = false;
    };
  }, [orgId]);

  const toggle = (name: string) => {
    setGranted((prev) =>
      prev.includes(name) ? prev.filter((c) => c !== name) : [...prev, name]
    );
  };

  const handleSave = async () => {
    if (isSaving) return;
    setError(null);
    setIsSaving(true);
    try {
      // Sorted so the payload is stable regardless of click order, and always
      // an explicit array — an omitted key is a 400, deliberately, so that a
      // malformed write can never read as "revoke everything".
      const res = await orgsApi.setOrgCapabilities(orgId, {
        capabilities: [...granted].sort(),
      });
      // Adopt the server's set rather than the submitted one: another operator
      // may have changed grants since this screen loaded.
      const saved = res.data.data.capabilities ?? [];
      setGranted(saved);
      toast.success('Capabilities updated');
      onSaved?.(saved);
    } catch (err) {
      setError(extractErrorMessage(err, 'Failed to update capabilities'));
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <section className="mt-8 border-t border-gray-700 pt-6">
      <h2 className="text-lg font-semibold text-white mb-1">Capabilities</h2>
      <p className="text-sm text-gray-400 mb-4">
        Superadmin-only. Granting or revoking takes effect on the org&apos;s next
        request — no sign-out required.
      </p>

      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-4">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {available === null ? (
        !error && <p className="text-gray-400 text-sm">Loading capabilities…</p>
      ) : (
        <div className="space-y-4">
          <div className="space-y-3">
            {available.map((name) => (
              <div key={name} className="flex items-center gap-3">
                <input
                  id={`capability-${name}`}
                  type="checkbox"
                  checked={granted.includes(name)}
                  onChange={() => toggle(name)}
                  disabled={isSaving}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-2 focus:ring-blue-500"
                />
                <label
                  htmlFor={`capability-${name}`}
                  className="text-sm font-medium text-gray-300"
                >
                  {capabilityLabel(name)}
                </label>
              </div>
            ))}
          </div>

          <button
            type="button"
            onClick={handleSave}
            disabled={isSaving}
            className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isSaving ? 'Saving...' : 'Save capabilities'}
          </button>
        </div>
      )}
    </section>
  );
}
