import React from 'react';
import toast from 'react-hot-toast';
import { useQueryClient } from '@tanstack/react-query';
import { ArrowLeftRight } from 'lucide-react';
import { useTagStore } from '@/stores/tagStore';
import { useKitStore, type PairSlot } from '@/stores/kitStore';
import { useKitCommission } from '@/hooks/kits/useKitCommission';
import { useKitMemberships, KIT_MEMBERSHIP_QUERY_KEY } from '@/hooks/kits/useKitMemberships';
import { selectKitMemberTags, buildPairCommissionRequest, KIT_QA_FIELDS } from '@/utils/kitUtils';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { ErrorBanner } from '@/components/banners/ErrorBanner';
import { ScanControls } from './ScanControls';

/**
 * Commission flow (TRA-1033 pair model): a kit is one Router + one Coupon,
 * 1:1. Scan both tags — the first eligible scan fills the Router slot, the
 * second the Coupon slot (reassignable per row, Swap to flip). Lot # plus the
 * optional QA fields describe the PAIR; several pairs may share a Lot #.
 *
 * Tags already in an active pair are flagged inline and never auto-assigned —
 * a tag belongs to one active pair, so including them could only end in the
 * server's 409.
 */
const CommissionKit: React.FC = () => {
  const tags = useTagStore((state) => state.tags);
  const clearTags = useTagStore((state) => state.clearTags);
  const pairSlots = useKitStore((state) => state.pairSlots);
  const setPairSlot = useKitStore((state) => state.setPairSlot);
  const clearPairSlots = useKitStore((state) => state.clearPairSlots);
  const { commission, isSaving } = useKitCommission();
  const queryClient = useQueryClient();

  const [label, setLabel] = React.useState('');
  const [qaFields, setQaFields] = React.useState<Record<string, string>>({});
  const [error, setError] = React.useState<string | null>(null);

  const members = selectKitMemberTags(tags);
  const memberships = useKitMemberships(members.map((t) => t.epc));
  const eligible = members.filter((t) => !memberships.has(t.epc));

  // Auto-assign in scan order: first eligible tag → Router, second → Coupon.
  // Only fills EMPTY slots; manual assignments are never overridden. Slots
  // referencing tags that left the session (Clear) are released.
  React.useEffect(() => {
    const eligibleEpcs = new Set(eligible.map((t) => t.epc));
    const current = useKitStore.getState().pairSlots;
    (['router', 'coupon'] as PairSlot[]).forEach((slot) => {
      if (current[slot] !== null && !eligibleEpcs.has(current[slot]!)) {
        setPairSlot(slot, null);
      }
    });
    const assigned = new Set(
      Object.values(useKitStore.getState().pairSlots).filter(Boolean) as string[]
    );
    for (const tag of eligible) {
      if (assigned.has(tag.epc)) continue;
      const slots = useKitStore.getState().pairSlots;
      if (slots.router === null) {
        setPairSlot('router', tag.epc);
        assigned.add(tag.epc);
      } else if (slots.coupon === null) {
        setPairSlot('coupon', tag.epc);
        assigned.add(tag.epc);
      } else {
        break;
      }
    }
  }, [eligible, setPairSlot]);

  const canSave =
    label.trim() !== '' && pairSlots.router !== null && pairSlots.coupon !== null && !isSaving;

  const handleSave = async () => {
    if (pairSlots.router === null || pairSlots.coupon === null) return;
    setError(null);
    try {
      const kit = await commission(
        buildPairCommissionRequest(label, pairSlots.router, pairSlots.coupon, qaFields)
      );
      toast.success(`Pair saved — Lot ${kit.label} (Router + Coupon linked)`);
      clearTags();
      clearPairSlots();
      setLabel('');
      setQaFields({});
      // Freshly paired tags must show as already-paired on the next scan.
      queryClient.invalidateQueries({ queryKey: [KIT_MEMBERSHIP_QUERY_KEY] });
    } catch (err) {
      // 409 detail names the pair that already owns a tag — keep it visible.
      setError(getApiErrorMessage(err, 'Failed to save pair'));
    }
  };

  const handleClear = () => {
    clearTags();
    clearPairSlots();
    setQaFields({});
    setError(null);
  };

  const handleSwap = () => {
    const { router, coupon } = useKitStore.getState().pairSlots;
    useKitStore.setState({ pairSlots: { router: coupon, coupon: router } });
  };

  const slotDisplay = (epc: string | null) => {
    if (!epc) return null;
    const tag = members.find((t) => t.epc === epc);
    return tag ? tag.displayEpc || tag.epc : epc;
  };

  const slotBadge = (slot: PairSlot, title: string) => (
    <div
      data-testid={`kit-slot-${slot}`}
      className={`flex-1 rounded-lg border-2 px-3 py-2 ${
        pairSlots[slot]
          ? 'border-green-400 dark:border-green-700 bg-green-50 dark:bg-green-900/20'
          : 'border-dashed border-gray-300 dark:border-gray-600'
      }`}
    >
      <div className="text-xs font-medium text-gray-500 dark:text-gray-400">{title}</div>
      <div className="font-mono text-sm text-gray-900 dark:text-gray-100 truncate">
        {slotDisplay(pairSlots[slot]) ?? <span className="text-gray-400">scan tag…</span>}
      </div>
    </div>
  );

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <p className="text-gray-600 dark:text-gray-400">
          Scan the Router and Coupon tags, enter the Lot #, save the pair.
        </p>
        <ScanControls />
      </div>

      <div className="mb-4">
        <ErrorBanner error={error} />
      </div>

      {/* The pair being built: first scan → Router, second → Coupon */}
      <div className="mb-4 flex items-center gap-2">
        {slotBadge('router', 'Router')}
        <button
          data-testid="kit-pair-swap"
          onClick={handleSwap}
          title="Swap Router and Coupon"
          className="p-2 rounded-lg text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
        >
          <ArrowLeftRight className="w-5 h-5" />
        </button>
        {slotBadge('coupon', 'Coupon')}
      </div>

      <div className="mb-4">
        <label className="block text-sm font-medium mb-2 text-gray-700 dark:text-gray-300">
          Lot #
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

      {/* Howmet QA fields — characteristics of this pair, all optional. */}
      <div className="mb-4 grid grid-cols-2 md:grid-cols-3 gap-3">
        {KIT_QA_FIELDS.map(({ key, label: fieldLabel }) => (
          <div key={key}>
            <label className="block text-xs font-medium mb-1 text-gray-500 dark:text-gray-400">
              {fieldLabel} <span className="font-normal">(optional)</span>
            </label>
            <input
              type="text"
              data-testid={`kit-qa-${key}`}
              value={qaFields[key] ?? ''}
              onChange={(e) => setQaFields((prev) => ({ ...prev, [key]: e.target.value }))}
              className="w-full px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
            />
          </div>
        ))}
      </div>

      <div className="mb-4 bg-white dark:bg-gray-800 rounded-lg shadow divide-y divide-gray-200 dark:divide-gray-700">
        <div className="px-4 py-2 text-sm font-medium text-gray-600 dark:text-gray-400">
          Scanned tags ({members.length}) — location tags are excluded
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
                  in Lot {owningKit.label}
                </span>
              ) : (
                <div className="inline-flex rounded-lg border border-gray-300 dark:border-gray-600 overflow-hidden flex-shrink-0">
                  {(['router', 'coupon'] as PairSlot[]).map((slot) => (
                    <button
                      key={slot}
                      data-testid={`kit-assign-${slot}-${tag.epc}`}
                      onClick={() => setPairSlot(slot, tag.epc)}
                      className={`px-2.5 py-1 text-xs font-medium capitalize transition-colors ${
                        pairSlots[slot] === tag.epc
                          ? 'bg-blue-600 text-white'
                          : 'bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
                      }`}
                    >
                      {slot}
                    </button>
                  ))}
                </div>
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
          {isSaving ? 'Saving…' : 'Save Pair'}
        </button>
        <button
          data-testid="kit-commission-clear"
          onClick={handleClear}
          className="px-4 py-2 rounded-lg font-medium text-gray-700 dark:text-gray-300 bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 dark:hover:bg-gray-600 transition-colors"
        >
          Clear
        </button>
        {(pairSlots.router === null || pairSlots.coupon === null) && (
          <span className="text-sm text-gray-500 dark:text-gray-400">
            Assign a Router and a Coupon tag
          </span>
        )}
      </div>
    </div>
  );
};

export default CommissionKit;
