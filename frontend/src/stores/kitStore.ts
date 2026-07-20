/**
 * Kit Store - Session-local state for the Kits tab (TRA-1033)
 *
 * Deliberately NOT persisted: kit sessions are ephemeral. The verify result
 * lives here (not in component state) so it survives the Locate handoff
 * round-trip — switching to #locate and back must not lose the red banner.
 */
import { create } from 'zustand';
import type { VerifyResponse } from '@/lib/api/kits';
import type { ScanTabMode } from './uiStore';

export type KitsView = 'commission' | 'verify' | 'find';

export type PairSlot = 'router' | 'coupon';

interface KitState {
  view: KitsView;
  // Per-view RFID|Barcode toggle. Commissioning individual items defaults to
  // barcode/QR; the verify dock check is a bulk RFID scan.
  scanModes: Record<KitsView, ScanTabMode>;
  // Commission draft (TRA-1033 pair model): a kit is one Router + one Coupon.
  // Slots hold the assigned tag EPCs; reset after a successful save.
  pairSlots: Record<PairSlot, string | null>;
  verifyResult: VerifyResponse | null;

  setView: (view: KitsView) => void;
  setScanMode: (view: KitsView, mode: ScanTabMode) => void;
  setPairSlot: (slot: PairSlot, epc: string | null) => void;
  clearPairSlots: () => void;
  setVerifyResult: (result: VerifyResponse | null) => void;
}

export const useKitStore = create<KitState>((set) => ({
  view: 'commission',
  scanModes: { commission: 'barcode', verify: 'rfid', find: 'rfid' },
  pairSlots: { router: null, coupon: null },
  verifyResult: null,

  setView: (view) => set({ view }),
  setScanMode: (view, mode) =>
    set((s) => ({ scanModes: { ...s.scanModes, [view]: mode } })),
  setPairSlot: (slot, epc) =>
    set((s) => {
      const next: Record<PairSlot, string | null> = { ...s.pairSlots, [slot]: epc };
      // A tag holds exactly one role — assigning it here evicts it elsewhere.
      const other: PairSlot = slot === 'router' ? 'coupon' : 'router';
      if (epc !== null && next[other] === epc) {
        next[other] = null;
      }
      return { pairSlots: next };
    }),
  clearPairSlots: () => set({ pairSlots: { router: null, coupon: null } }),
  setVerifyResult: (result) => set({ verifyResult: result }),
}));

/** Effective RFID|Barcode mode for the Kits tab's active view. */
export function getKitsScanMode(state: Pick<KitState, 'view' | 'scanModes'>): ScanTabMode {
  return state.scanModes[state.view];
}
