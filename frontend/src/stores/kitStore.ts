/**
 * Kit Store - Session-local state for the Kits tab (TRA-1033)
 *
 * Deliberately NOT persisted: kit sessions are ephemeral. The verify result
 * lives here (not in component state) so it survives the Locate handoff
 * round-trip — switching to #locate and back must not lose the red banner.
 */
import { create } from 'zustand';
import type { VerifyResponse } from '@/lib/api/kits';

export type KitsView = 'commission' | 'verify';

interface KitState {
  view: KitsView;
  // Commission draft: epc -> free-text role. Reset after a successful save.
  memberRoles: Record<string, string>;
  verifyResult: VerifyResponse | null;

  setView: (view: KitsView) => void;
  setMemberRole: (epc: string, role: string) => void;
  clearMemberRoles: () => void;
  setVerifyResult: (result: VerifyResponse | null) => void;
}

export const useKitStore = create<KitState>((set) => ({
  view: 'commission',
  memberRoles: {},
  verifyResult: null,

  setView: (view) => set({ view }),
  setMemberRole: (epc, role) =>
    set((s) => ({ memberRoles: { ...s.memberRoles, [epc]: role } })),
  clearMemberRoles: () => set({ memberRoles: {} }),
  setVerifyResult: (result) => set({ verifyResult: result }),
}));
