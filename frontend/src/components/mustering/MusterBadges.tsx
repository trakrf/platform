// MusterBadges — roster of person-assets + badge assignment (TRA-978 Phase 4).
//
// Shows every asset with metadata.person === true. Actions (operator+):
//   - Add person: creates a new asset with metadata:{person:true}
//   - Assign badge: POSTs an rfid tag (MAC) to the asset via assetsApi.addTag
//   - Remove badge: DELETEs the existing rfid tag, then assigns the new one
//   - Deactivate: PATCH is_active=false on the asset
//
// Last-seen column comes from the muster store entries when an event is active
// (entry.last_seen_at keyed by asset_id). When no event is active the column
// is omitted per the privacy spec.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Plus, Tag, UserX, Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { assetsApi } from '@/lib/api/assets';
import { useMusterStore, useOrgStore } from '@/stores';
import type { Asset } from '@/types/assets';
import { OPERATOR_PLUS, relativeAge } from './helpers';

// Uppercase a MAC address so it matches what the backend stores / GL-S10 sends.
function normaliseMac(raw: string): string {
  return raw.trim().toUpperCase();
}

export default function MusterBadges() {
  const event = useMusterStore((s) => s.event);
  const currentRole = useOrgStore((s) => s.currentRole);
  const canOperate = !!currentRole && OPERATOR_PLUS.includes(currentRole);

  const [persons, setPersons] = useState<Asset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Add-person form state
  const [showAddForm, setShowAddForm] = useState(false);
  const [newLabel, setNewLabel] = useState('');
  const [adding, setAdding] = useState(false);

  // Badge-assign state: assetId → input value + pending flag
  const [badgeInputs, setBadgeInputs] = useState<Record<number, string>>({});
  const [assigningId, setAssigningId] = useState<number | null>(null);
  const [deactivatingId, setDeactivatingId] = useState<number | null>(null);

  // Build a lookup: asset_id → last_seen_at from the active event's entries.
  const lastSeenByAsset = useMemo(() => {
    const m = new Map<number, string>();
    if (event?.status === 'active' && event.entries) {
      for (const e of event.entries) {
        if (e.last_seen_at) m.set(e.asset_id, e.last_seen_at);
      }
    }
    return m;
  }, [event]);

  const fetchPersons = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      // POC: limit:200 (the API max) client-filter cliff — server-side person filter is the post-POC fix.
      const { data } = await assetsApi.list({ limit: 200 });
      const personAssets = data.data.filter(
        (a) => a.is_active && a.metadata?.person === true,
      );
      setPersons(personAssets);
    } catch {
      setError('Failed to load person roster.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchPersons();
  }, [fetchPersons]);

  // --- Actions ---

  const handleAddPerson = async () => {
    if (!newLabel.trim()) return;
    setAdding(true);
    try {
      await assetsApi.create({
        name: newLabel.trim(),
        valid_from: new Date().toISOString(),
        valid_to: null,
        is_active: true,
        metadata: { person: true },
      });
      setNewLabel('');
      setShowAddForm(false);
      toast.success('Person added');
      await fetchPersons();
    } catch {
      toast.error('Failed to add person');
    } finally {
      setAdding(false);
    }
  };

  const handleAssignBadge = async (asset: Asset) => {
    const raw = badgeInputs[asset.id] ?? '';
    const value = normaliseMac(raw);
    if (!value) return;

    // Short-circuit if the user re-typed the same value that's already assigned
    // (the partial unique index on (org_id, type, value) would reject it anyway).
    const existingRfid = asset.tags.filter((t) => t.tag_type === 'rfid');
    if (existingRfid.length === 1 && existingRfid[0].value === value) {
      toast('Badge already assigned to this person', { icon: 'ℹ️' });
      return;
    }

    setAssigningId(asset.id);
    try {
      // Add the new badge FIRST so the old one is only removed on success.
      // If addTag fails (e.g. value already used by another asset), the old badge
      // stays intact — the catch below surfaces this to the user.
      await assetsApi.addTag(asset.id, { tag_type: 'rfid', value });
      // Remove old rfid tag(s) only after the new one is confirmed saved.
      for (const t of existingRfid) {
        await assetsApi.removeTag(asset.id, t.id);
      }
      setBadgeInputs((prev) => ({ ...prev, [asset.id]: '' }));
      toast.success('Badge assigned');
      await fetchPersons();
    } catch {
      toast.error('Badge unchanged — could not assign (value may already be in use)');
    } finally {
      setAssigningId(null);
    }
  };

  const handleDeactivate = async (asset: Asset) => {
    setDeactivatingId(asset.id);
    try {
      await assetsApi.update(asset.id, { is_active: false });
      toast.success(`${asset.name} deactivated`);
      await fetchPersons();
    } catch {
      toast.error('Failed to deactivate person');
    } finally {
      setDeactivatingId(null);
    }
  };

  // --- Render ---

  const showLastSeen = event?.status === 'active';

  return (
    <div className="space-y-4" data-testid="muster-badges">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          Person roster ({persons.length})
        </h2>
        {canOperate && (
          <button
            onClick={() => setShowAddForm((v) => !v)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-blue-600 text-white text-sm font-medium hover:bg-blue-700"
            data-testid="muster-badges-add-person"
          >
            <Plus className="w-4 h-4" />
            Add person
          </button>
        )}
      </div>

      {/* Add-person inline form */}
      {canOperate && showAddForm && (
        <div className="rounded-lg border border-blue-200 dark:border-blue-800 bg-blue-50 dark:bg-blue-900/20 p-4 flex items-end gap-3">
          <label className="flex-1">
            <span className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
              Label (display name)
            </span>
            <input
              type="text"
              value={newLabel}
              onChange={(e) => setNewLabel(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && void handleAddPerson()}
              placeholder="e.g. Operator 013"
              autoFocus
              className="w-full rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-1.5 text-sm text-gray-900 dark:text-gray-100"
              data-testid="muster-badges-label-input"
            />
          </label>
          <button
            onClick={() => void handleAddPerson()}
            disabled={adding || !newLabel.trim()}
            className="px-4 py-2 rounded-md bg-blue-600 text-white text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
            data-testid="muster-badges-save-person"
          >
            {adding ? <Loader2 className="w-4 h-4 animate-spin" /> : 'Save'}
          </button>
          <button
            onClick={() => { setShowAddForm(false); setNewLabel(''); }}
            className="px-3 py-2 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-md"
          >
            Cancel
          </button>
        </div>
      )}

      {/* Error / loading */}
      {error && (
        <div className="rounded-md bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 px-4 py-2 text-sm text-red-700 dark:text-red-300">
          {error}
        </div>
      )}
      {loading && (
        <div className="flex items-center justify-center p-8 text-gray-500 dark:text-gray-400">
          <Loader2 className="w-5 h-5 animate-spin mr-2" /> Loading roster…
        </div>
      )}

      {/* Roster table */}
      {!loading && persons.length === 0 && !error && (
        <div className="rounded-lg border border-dashed border-gray-300 dark:border-gray-700 p-8 text-center text-gray-500 dark:text-gray-400">
          No people in the roster yet. Add a person or seed demo data from the Dashboard.
        </div>
      )}

      {!loading && persons.length > 0 && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800/60">
              <tr>
                <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Name</th>
                <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Badge (EPC/MAC)</th>
                {showLastSeen && (
                  <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Last seen</th>
                )}
                {canOperate && (
                  <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Actions</th>
                )}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {persons.map((person) => {
                const rfidTag = person.tags.find((t) => t.tag_type === 'rfid');
                const badgeValue = rfidTag?.value ?? '';
                const lastSeen = lastSeenByAsset.get(person.id);
                const isAssigning = assigningId === person.id;
                const isDeactivating = deactivatingId === person.id;

                return (
                  <tr key={person.id} className="bg-white dark:bg-gray-900">
                    <td className="px-4 py-3 font-medium text-gray-900 dark:text-gray-100">
                      {person.name}
                    </td>
                    <td className="px-4 py-3">
                      {canOperate ? (
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-xs text-gray-500 dark:text-gray-400 min-w-[8rem]">
                            {badgeValue || <span className="italic">none</span>}
                          </span>
                          <input
                            type="text"
                            value={badgeInputs[person.id] ?? ''}
                            onChange={(e) =>
                              setBadgeInputs((prev) => ({ ...prev, [person.id]: e.target.value }))
                            }
                            onKeyDown={(e) => e.key === 'Enter' && void handleAssignBadge(person)}
                            placeholder="AA:BB:CC:DD:EE:FF"
                            aria-label={`Badge MAC for ${person.name}`}
                            className="w-40 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-2 py-1 text-xs font-mono text-gray-900 dark:text-gray-100"
                            data-testid={`muster-badges-badge-input-${person.id}`}
                          />
                          <button
                            onClick={() => void handleAssignBadge(person)}
                            disabled={isAssigning || !(badgeInputs[person.id] ?? '').trim()}
                            className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50"
                            data-testid={`muster-badges-assign-${person.id}`}
                          >
                            {isAssigning ? (
                              <Loader2 className="w-3 h-3 animate-spin" />
                            ) : (
                              <Tag className="w-3 h-3" />
                            )}
                            {badgeValue ? 'Replace' : 'Assign'}
                          </button>
                        </div>
                      ) : (
                        <span className="font-mono text-xs text-gray-700 dark:text-gray-300">
                          {badgeValue || '—'}
                        </span>
                      )}
                    </td>
                    {showLastSeen && (
                      <td className="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
                        {lastSeen ? relativeAge(lastSeen) : '—'}
                      </td>
                    )}
                    {canOperate && (
                      <td className="px-4 py-3 text-right">
                        <button
                          onClick={() => void handleDeactivate(person)}
                          disabled={isDeactivating}
                          className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
                          data-testid={`muster-badges-deactivate-${person.id}`}
                          title="Deactivate this person"
                        >
                          {isDeactivating ? (
                            <Loader2 className="w-3 h-3 animate-spin" />
                          ) : (
                            <UserX className="w-3 h-3" />
                          )}
                          Deactivate
                        </button>
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
