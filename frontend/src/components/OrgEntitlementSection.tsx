/**
 * OrgEntitlementSection — superadmin-only entitlement controls (TRA-949).
 *
 * Renders the two TRA-947 columns as editable fields:
 *  - subscription_enabled (toggle)
 *  - subscription_expires_at (datetime, clearable; empty = never expires)
 *
 * Visibility is the caller's responsibility — OrgSettingsScreen only mounts this
 * for a superadmin. The backend independently rejects non-superadmins (403), so
 * this is a UI affordance, not the security boundary.
 */

import { useState } from 'react';
import { orgsApi } from '@/lib/api/orgs';
import { extractErrorMessage } from '@/lib/asset/helpers';
import toast from 'react-hot-toast';

interface OrgEntitlementSectionProps {
  orgId: number;
  initialEnabled: boolean;
  initialExpiresAt?: string | null;
  /** Called with the updated org after a successful save. */
  onSaved?: (subscriptionEnabled: boolean, subscriptionExpiresAt: string | null) => void;
}

// datetime-local inputs use "YYYY-MM-DDTHH:mm" in local time. Convert to/from
// the RFC3339 instant the API stores.
function toLocalInput(iso?: string | null): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fromLocalInput(local: string): string | null {
  if (!local) return null;
  const d = new Date(local);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

export function OrgEntitlementSection({
  orgId,
  initialEnabled,
  initialExpiresAt,
  onSaved,
}: OrgEntitlementSectionProps) {
  const [enabled, setEnabled] = useState(initialEnabled);
  const [expiresLocal, setExpiresLocal] = useState(toLocalInput(initialExpiresAt));
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = async () => {
    if (isSaving) return;
    setError(null);
    setIsSaving(true);
    const subscription_expires_at = fromLocalInput(expiresLocal);
    try {
      await orgsApi.updateEntitlement(orgId, {
        subscription_enabled: enabled,
        subscription_expires_at,
      });
      toast.success('Entitlement updated');
      onSaved?.(enabled, subscription_expires_at);
    } catch (err) {
      setError(extractErrorMessage(err, 'Failed to update entitlement'));
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <section className="mt-8 border-t border-gray-700 pt-6">
      <h2 className="text-lg font-semibold text-white mb-1">Entitlement</h2>
      <p className="text-sm text-gray-400 mb-4">
        Superadmin-only. Changes take effect on the next entitlement check.
      </p>

      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-4">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      <div className="space-y-4">
        <div className="flex items-center gap-3">
          <input
            id="subscription-enabled"
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            disabled={isSaving}
            className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-2 focus:ring-blue-500"
          />
          <label htmlFor="subscription-enabled" className="text-sm font-medium text-gray-300">
            Subscription enabled
          </label>
        </div>

        <div>
          <label
            htmlFor="subscription-expires-at"
            className="block text-sm font-medium text-gray-300 mb-2"
          >
            Expires at
          </label>
          <div className="flex items-center gap-2">
            <input
              id="subscription-expires-at"
              type="datetime-local"
              value={expiresLocal}
              onChange={(e) => setExpiresLocal(e.target.value)}
              disabled={isSaving}
              className="px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500 disabled:opacity-50"
            />
            {expiresLocal && (
              <button
                type="button"
                onClick={() => setExpiresLocal('')}
                disabled={isSaving}
                className="px-3 py-2 text-sm text-gray-400 hover:text-white border border-gray-600 rounded-lg transition-colors disabled:opacity-50"
              >
                Clear
              </button>
            )}
          </div>
          <p className="text-gray-500 text-sm mt-1">
            Leave empty for no expiry (never expires).
          </p>
        </div>

        <button
          type="button"
          onClick={handleSave}
          disabled={isSaving}
          className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {isSaving ? 'Saving...' : 'Save entitlement'}
        </button>
      </div>
    </section>
  );
}
