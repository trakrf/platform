import React from 'react';
import toast from 'react-hot-toast';
import { useQueryClient } from '@tanstack/react-query';
import { useTagStore } from '@/stores/tagStore';
import { useKitStore } from '@/stores/kitStore';
import { useKitCommission } from '@/hooks/kits/useKitCommission';
import { useKitMemberships, KIT_MEMBERSHIP_QUERY_KEY } from '@/hooks/kits/useKitMemberships';
import { selectKitMemberTags, buildCommissionRequest } from '@/utils/kitUtils';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { ErrorBanner } from '@/components/banners/ErrorBanner';
import { ScanControls } from './ScanControls';

const ROLE_SUGGESTIONS = ['coupon', 'tote', 'traveler', 'case'];

/**
 * Commission flow (TRA-1033): scan asset tags, key the Lot # from the paper
 * envelope, save. Label is the ONLY typed field by design — Part#/Heat#/etc.
 * stay on the envelope; double entry is the compliance killer this replaces.
 *
 * Tags already in an active kit are flagged inline and excluded from the
 * save — an asset can belong to one active kit, so including them could only
 * end in the server's 409.
 */
const CommissionKit: React.FC = () => {
  const tags = useTagStore((state) => state.tags);
  const clearTags = useTagStore((state) => state.clearTags);
  const memberRoles = useKitStore((state) => state.memberRoles);
  const setMemberRole = useKitStore((state) => state.setMemberRole);
  const clearMemberRoles = useKitStore((state) => state.clearMemberRoles);
  const { commission, isSaving } = useKitCommission();
  const queryClient = useQueryClient();

  const [label, setLabel] = React.useState('');
  const [error, setError] = React.useState<string | null>(null);

  const members = selectKitMemberTags(tags);
  const memberships = useKitMemberships(members.map((t) => t.epc));
  const eligible = members.filter((t) => !memberships.has(t.epc));
  const canSave = label.trim() !== '' && eligible.length >= 2 && !isSaving;

  const handleSave = async () => {
    setError(null);
    try {
      const kit = await commission(buildCommissionRequest(label, eligible, memberRoles));
      toast.success(`Kit ${kit.label} created with ${kit.members.length} members`);
      clearTags();
      clearMemberRoles();
      setLabel('');
      // Freshly kitted tags must show as already-kitted on the next scan.
      queryClient.invalidateQueries({ queryKey: [KIT_MEMBERSHIP_QUERY_KEY] });
    } catch (err) {
      // 409 detail names the kit that already owns a member — keep it visible.
      setError(getApiErrorMessage(err, 'Failed to create kit'));
    }
  };

  const handleClear = () => {
    clearTags();
    clearMemberRoles();
    setError(null);
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <p className="text-gray-600 dark:text-gray-400">
          Scan the kit&apos;s tags, enter the Lot #, save.
        </p>
        <ScanControls />
      </div>

      <div className="mb-4">
        <ErrorBanner error={error} />
      </div>

      <div className="mb-4">
        <label className="block text-sm font-medium mb-2 text-gray-700 dark:text-gray-300">
          Label (Lot #)
        </label>
        <input
          type="text"
          data-testid="kit-label-input"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          placeholder="Lot # (e.g. 1184015)"
          className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
        />
      </div>

      <datalist id="kit-role-suggestions">
        {ROLE_SUGGESTIONS.map((role) => (
          <option key={role} value={role} />
        ))}
      </datalist>

      <div className="mb-4 bg-white dark:bg-gray-800 rounded-lg shadow divide-y divide-gray-200 dark:divide-gray-700">
        <div className="px-4 py-2 text-sm font-medium text-gray-600 dark:text-gray-400">
          Members ({eligible.length}) — scan to add; location tags are excluded
        </div>
        {members.length === 0 && (
          <div className="px-4 py-6 text-center text-gray-500 dark:text-gray-400">
            No tags scanned yet
          </div>
        )}
        {members.map((tag) => {
          const owningKit = memberships.get(tag.epc);
          return (
            <div key={tag.epc} className="px-4 py-2 flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div
                  className={`font-mono text-sm truncate ${
                    owningKit
                      ? 'text-gray-400 dark:text-gray-500'
                      : 'text-gray-900 dark:text-gray-100'
                  }`}
                >
                  {tag.displayEpc || tag.epc}
                </div>
                {tag.assetName && (
                  <div className="text-xs text-gray-500 dark:text-gray-400 truncate">
                    {tag.assetName}
                  </div>
                )}
              </div>
              {owningKit ? (
                <span
                  data-testid={`kit-member-owned-${tag.epc}`}
                  className="flex-shrink-0 px-2 py-1 text-xs font-medium rounded bg-amber-100 dark:bg-amber-900/30 text-amber-900 dark:text-amber-300"
                >
                  in kit {owningKit.label}
                </span>
              ) : (
                <input
                  type="text"
                  data-testid={`kit-role-input-${tag.epc}`}
                  list="kit-role-suggestions"
                  value={memberRoles[tag.epc] ?? ''}
                  onChange={(e) => setMemberRole(tag.epc, e.target.value)}
                  placeholder="role (optional)"
                  className="w-32 px-2 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                />
              )}
            </div>
          );
        })}
      </div>

      <div className="flex items-center gap-3">
        <button
          data-testid="kit-save"
          onClick={handleSave}
          disabled={!canSave}
          className={`px-4 py-2 rounded-lg font-medium text-white transition-colors ${
            canSave ? 'bg-blue-600 hover:bg-blue-700' : 'bg-blue-400 opacity-50 cursor-not-allowed'
          }`}
        >
          {isSaving ? 'Saving…' : 'Save Kit'}
        </button>
        <button
          data-testid="kit-commission-clear"
          onClick={handleClear}
          className="px-4 py-2 rounded-lg font-medium text-gray-700 dark:text-gray-300 bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 dark:hover:bg-gray-600 transition-colors"
        >
          Clear
        </button>
        {eligible.length < 2 && (
          <span className="text-sm text-gray-500 dark:text-gray-400">
            A kit needs at least 2 members
          </span>
        )}
      </div>
    </div>
  );
};

export default CommissionKit;
