/**
 * Kit Store - Session-local state for the Kits tab (TRA-1033)
 *
 * One flat surface: scan/search → matched tags. Paired tags render their pair
 * record; unpaired tags feed the pair builder. Deliberately NOT persisted —
 * kit sessions are ephemeral. The check result lives here (not in component
 * state) so it survives the Locate handoff round-trip.
 */
import { create } from 'zustand';
import type { VerifyResponse } from '@/lib/api/kits';
import type { ScanTabMode } from './uiStore';

export type PairSlot = 'router' | 'coupon';

interface KitState {
  // RFID|Barcode toggle for the Kits tab. RFID default — the trigger reads
  // single tags fine for commissioning; barcode/QR is one tap away.
  scanMode: ScanTabMode;
  // Pair being built from uncommissioned tags: one Router + one Coupon (1:1).
  pairSlots: Record<PairSlot, string | null>;
  verifyResult: VerifyResponse | null;

  setScanMode: (mode: ScanTabMode) => void;
  setPairSlot: (slot: PairSlot, epc: string | null) => void;
  clearPairSlots: () => void;
  setVerifyResult: (result: VerifyResponse | null) => void;
}

export const useKitStore = create<KitState>((set) => ({
  scanMode: 'rfid',
  pairSlots: { router: null, coupon: null },
  verifyResult: null,

  setScanMode: (mode) => set({ scanMode: mode }),
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

/** Effective RFID|Barcode mode for the Kits tab. */
export function getKitsScanMode(state: Pick<KitState, 'scanMode'>): ScanTabMode {
  return state.scanMode;
}
