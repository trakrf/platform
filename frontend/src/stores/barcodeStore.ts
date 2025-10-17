/**
 * Barcode Store - Manages barcode data and scanning state
 */
import { create } from 'zustand';

export interface BarcodeData {
  data: string;
  type: string;      // From Code ID
  aimId?: string;    // From AIM ID
  timestamp: number;
  raw?: Uint8Array;  // For debugging
}

interface BarcodeStore {
  // State
  barcodes: BarcodeData[];
  scanning: boolean;
  lastScanTime: number;
  buffer: Uint8Array | null; // For partial packet buffering
  
  // Actions
  addBarcode: (barcode: BarcodeData) => void;
  clearBarcodes: () => void;
  setScanning: (scanning: boolean) => void;
  setBuffer: (buffer: Uint8Array | null) => void;
  appendToBuffer: (data: Uint8Array) => void;
}

export const useBarcodeStore = create<BarcodeStore>((set) => ({
  barcodes: [],
  scanning: false,
  lastScanTime: 0,
  buffer: null,
  
  addBarcode: (barcode) => {
    console.warn('[BarcodeStore] Adding barcode:', barcode);
    return set((state) => ({
      barcodes: [barcode, ...state.barcodes], // Add new barcode to beginning
      lastScanTime: Date.now()
    }));
  },
  
  clearBarcodes: () => set({ barcodes: [], buffer: null }),
  
  setScanning: (scanning) => set({ scanning }),
  
  setBuffer: (buffer) => set({ buffer }),
  
  appendToBuffer: (data) => set((state) => {
    if (!state.buffer) return { buffer: data };
    const combined = new Uint8Array(state.buffer.length + data.length);
    combined.set(state.buffer);
    combined.set(data, state.buffer.length);
    return { buffer: combined };
  })
}));