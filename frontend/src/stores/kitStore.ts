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

export type KitsView = 'commission' | 'verify';

interface KitState {
  view: KitsView;
  // Per-view RFID|Barcode toggle. Commissioning individual items defaults to
  // barcode/QR; the verify dock check is a bulk RFID scan.
  scanModes: Record<KitsView, ScanTabMode>;
  // Commission draft: epc -> free-text role. Reset after a successful save.
  memberRoles: Record<string, string>;
  verifyResult: VerifyResponse | null;

  setView: (view: KitsView) => void;
  setScanMode: (view: KitsView, mode: ScanTabMode) => void;
  setMemberRole: (epc: string, role: string) => void;
  clearMemberRoles: () => void;
  setVerifyResult: (result: VerifyResponse | null) => void;
}

export const useKitStore = create<KitState>((set) => ({
  view: 'commission',
  scanModes: { commission: 'barcode', verify: 'rfid' },
  memberRoles: {},
  verifyResult: null,

  setView: (view) => set({ view }),
  setScanMode: (view, mode) =>
    set((s) => ({ scanModes: { ...s.scanModes, [view]: mode } })),
  setMemberRole: (epc, role) =>
    set((s) => ({ memberRoles: { ...s.memberRoles, [epc]: role } })),
  clearMemberRoles: () => set({ memberRoles: {} }),
  setVerifyResult: (result) => set({ verifyResult: result }),
}));

/** Effective RFID|Barcode mode for the Kits tab's active view. */
export function getKitsScanMode(state: Pick<KitState, 'view' | 'scanModes'>): ScanTabMode {
  return state.scanModes[state.view];
}
