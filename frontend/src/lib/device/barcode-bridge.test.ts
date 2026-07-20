import { describe, it, expect, beforeEach } from 'vitest';
import { routeBarcodeRead } from './barcode-bridge';
import { useTagStore } from '@/stores/tagStore';
import { useBarcodeStore } from '@/stores/barcodeStore';
import { useUIStore } from '@/stores/uiStore';
import { useKitStore } from '@/stores/kitStore';

describe('routeBarcodeRead (TRA-1031)', () => {
  beforeEach(() => {
    useTagStore.setState({ tags: [], _lookupQueue: new Set(), _lookupTimer: null });
    useBarcodeStore.getState().clearBarcodes();
    useUIStore.setState({ activeTab: 'scan', scanTabMode: 'barcode' });
  });

  it('always records the read in barcodeStore, regardless of tab/mode', () => {
    useUIStore.setState({ activeTab: 'assets', scanTabMode: 'rfid' });
    routeBarcodeRead({ barcode: 'ASSET-001', symbology: 'Code128', timestamp: 1000 });
    const { barcodes } = useBarcodeStore.getState();
    expect(barcodes).toHaveLength(1);
    expect(barcodes[0].data).toBe('ASSET-001');
    expect(barcodes[0].type).toBe('Code128');
  });

  it('feeds tagStore with source barcode when scan tab is in barcode mode', () => {
    routeBarcodeRead({ barcode: '00123ABC', timestamp: 1000 });
    const { tags } = useTagStore.getState();
    expect(tags).toHaveLength(1);
    expect(tags[0].epc).toBe('00123ABC');
    expect(tags[0].source).toBe('barcode');
    expect(tags[0].count).toBe(1);
  });

  it('dedupes repeat scans of the same barcode by count-increment', () => {
    routeBarcodeRead({ barcode: '00123ABC', timestamp: 1000 });
    routeBarcodeRead({ barcode: '00123ABC', timestamp: 2000 });
    const { tags } = useTagStore.getState();
    expect(tags).toHaveLength(1);
    expect(tags[0].count).toBe(2);
  });

  it('queues the raw barcode value for async classification', () => {
    routeBarcodeRead({ barcode: '00123ABC', timestamp: 1000 });
    expect(useTagStore.getState()._lookupQueue.has('00123ABC')).toBe(true);
  });

  it('does not feed tagStore in RFID mode', () => {
    useUIStore.setState({ scanTabMode: 'rfid' });
    routeBarcodeRead({ barcode: '00123ABC', timestamp: 1000 });
    expect(useTagStore.getState().tags).toHaveLength(0);
  });

  it('does not feed tagStore when the scan tab is not active', () => {
    useUIStore.setState({ activeTab: 'assets' });
    routeBarcodeRead({ barcode: '00123ABC', timestamp: 1000 });
    expect(useTagStore.getState().tags).toHaveLength(0);
  });
});

describe('routeBarcodeRead on the kits tab (TRA-1033)', () => {
  beforeEach(() => {
    useTagStore.setState({ tags: [], _lookupQueue: new Set(), _lookupTimer: null });
    useBarcodeStore.getState().clearBarcodes();
    useUIStore.setState({ activeTab: 'kits', scanTabMode: 'rfid' });
    useKitStore.setState({ scanMode: 'barcode' });
  });

  it('feeds tagStore when the kits tab is in barcode mode', () => {
    routeBarcodeRead({ barcode: 'LOT-1184015-C1', timestamp: 1000 });
    const { tags } = useTagStore.getState();
    expect(tags).toHaveLength(1);
    expect(tags[0].epc).toBe('LOT-1184015-C1');
    expect(tags[0].source).toBe('barcode');
  });

  it('does not feed tagStore when the kits tab is in rfid mode', () => {
    useKitStore.setState({ scanMode: 'rfid' });
    routeBarcodeRead({ barcode: 'LOT-1184015-C1', timestamp: 1000 });
    expect(useTagStore.getState().tags).toHaveLength(0);
  });
});
