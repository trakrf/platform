/**
 * Type definitions for e2e tests
 *
 * ## The store shapes are DERIVED, not re-typed (TRA-1253)
 *
 * These were hand-written copies of the Zustand store shapes, and they had
 * drifted badly by the time `tsc` was first pointed at this tree:
 *
 *   declared here          actually on the store
 *   readerState: number    readerState: ReaderStateType  (a string — 'Connected')
 *   batteryLevel: number   batteryPercentage: number | null
 *   (absent)               readerMode: ReaderModeType | null
 *
 * A helper reading `batteryLevel` off the real store gets `undefined`, and an
 * assertion on `undefined` fails in a way that names the assertion rather than
 * the missing field. `tsconfig.json` excluded `tests/**`, so nothing could say
 * so — the copies looked authoritative and were simply wrong.
 *
 * Deriving from the store's own `getState` is what stops it happening again: a
 * field renamed in `src/stores/` now fails the typecheck here instead of quietly
 * reading `undefined` in a spec. These are `import type`, erased at run time, so
 * nothing about the browser/Node boundary changes.
 */

import type { useDeviceStore } from '@/stores/deviceStore';
import type { useTagStore } from '@/stores/tagStore';
import type { useUIStore } from '@/stores/uiStore';
import type { useBarcodeStore } from '@/stores/barcodeStore';
import type { useLocateStore } from '@/stores/locateStore';

export type DeviceStoreState = ReturnType<typeof useDeviceStore.getState>;
export type UIStoreState = ReturnType<typeof useUIStore.getState>;

export type BarcodeStoreState = ReturnType<typeof useBarcodeStore.getState>;
export type TagStoreState = ReturnType<typeof useTagStore.getState>;
export type LocateStoreState = ReturnType<typeof useLocateStore.getState>;

export interface ZustandStores {
  deviceStore: {
    getState: () => DeviceStoreState;
  };
  barcodeStore: {
    getState: () => BarcodeStoreState;
  };
  tagStore: {
    getState: () => TagStoreState;
  };
  locateStore: {
    getState: () => LocateStoreState;
  };
  // `main.tsx` exposes this alongside the others, and `helpers/device-state.ts`
  // has always read `activeTab` off it. It was simply missing from this list, so
  // those reads were type errors nobody could see.
  uiStore: {
    getState: () => UIStoreState;
  };
  tabStore?: {
    getState: () => {
      activeTab: string;
      setActiveTab: (tab: string) => void;
    };
  };
}

/**
 * The members this file re-declares with a NARROWER type than the global
 * `Window` carries.
 *
 * `src/types/` declares `__ZUSTAND_STORES__` twice — `window.d.ts` as all-
 * optional `any`, `web-bluetooth.d.ts` as a required-property `unknown` bag —
 * and `WebBleMock` once, with a different shape than the e2e helpers use. An
 * interface cannot narrow an inherited member, so extending `Window` directly
 * was a type error for as long as `tests/**` was excluded from `tsc` and nobody
 * could see it (TRA-1253).
 *
 * Omitting before re-declaring is the honest form: it says out loud that this is
 * a test-side override of a global, rather than pretending the two agree. The
 * duplicate declaration in `src/types/` is a real defect in its own right and is
 * reported on the ticket — it is not fixed here, because unifying a global that
 * `src/` depends on is a much larger change than typechecking the test tree.
 */
type NarrowedWindowMembers =
  | '__ZUSTAND_STORES__'
  | 'WebBleMock'
  | 'getRfidManager'
  // The globals type these as the real `DeviceManager` / `TransportManager`
  // classes. A spec only ever reaches for the two or three members it pokes, and
  // asserting the whole class here would oblige every test double to implement
  // 26 properties it never touches.
  | '__DEVICE_MANAGER__'
  | '__TRANSPORT_MANAGER__';

export interface WindowWithStores extends Omit<Window, NarrowedWindowMembers> {
  __ZUSTAND_STORES__?: ZustandStores;
  WebBleMock?: {
    requestDevice?: (options?: unknown) => Promise<BluetoothDevice>;
    getDevices?: () => Promise<BluetoothDevice[]>;
  };
  __TRANSPORT_MANAGER__?: {
    notifyCharacteristic?: { simulateNotification?: (data: Uint8Array) => void };
    emit?: (event: string, data: Uint8Array) => void;
    device?: BluetoothDevice;
  };
  __DEVICE_MANAGER__?: {
    transportManager?: {
      characteristic?: { simulateNotification?: (data: Uint8Array) => void };
      emit?: (event: string, data: Uint8Array) => void;
    };
  };
  __webBluetoothBridged?: boolean;
  getRfidManager?: () => Promise<{
    startInventory?: () => Promise<boolean>;
    stopInventory?: () => Promise<boolean>;
    isInventoryRunning?: () => boolean;
  }>;
}

// Type guard for checking if stores exist.
//
// Takes `unknown` rather than `Window`: a predicate's asserted type must be
// assignable to its parameter's type, and `WindowWithStores` deliberately is NOT
// assignable to `Window` — it narrows three of its members (see above). Widening
// the parameter is what lets the narrowing be expressed at all, and costs
// nothing, because every caller passes a `window`.
export function hasZustandStores(window: unknown): window is WindowWithStores {
  return typeof window === 'object' && window !== null && '__ZUSTAND_STORES__' in window;
}