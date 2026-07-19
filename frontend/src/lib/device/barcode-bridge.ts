/**
 * Barcode read routing (TRA-1031)
 *
 * Every barcode read lands in barcodeStore (feeds useScanToInput form
 * scanning). When the Scan tab is in Barcode mode, the read additionally
 * enters the inventory pipeline as if it were an EPC: tagStore dedupes it,
 * queues it for asset/location classification, and the Scan screen renders
 * it in the read list.
 */
import { useBarcodeStore } from '../../stores/barcodeStore';
import { useTagStore } from '../../stores/tagStore';
import { useUIStore } from '../../stores/uiStore';

export interface BarcodeReadPayload {
  barcode: string;
  symbology?: string;
  timestamp?: number;
}

export function routeBarcodeRead(payload: BarcodeReadPayload): void {
  const timestamp = payload.timestamp ?? Date.now();

  useBarcodeStore.getState().addBarcode({
    data: payload.barcode,
    type: payload.symbology || 'Unknown',
    timestamp,
  });

  const { activeTab, scanTabMode } = useUIStore.getState();
  if (activeTab === 'scan' && scanTabMode === 'barcode') {
    useTagStore.getState().addTag({
      epc: payload.barcode,
      count: 1,
      timestamp,
      source: 'barcode',
    });
  }
}
