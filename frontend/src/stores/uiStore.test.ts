import { describe, it, expect, beforeEach } from 'vitest';
import { useUIStore } from './uiStore';

describe('UIStore - scan tab mode (TRA-1031)', () => {
  beforeEach(() => {
    useUIStore.setState({ scanTabMode: 'rfid' });
  });

  it('defaults to rfid', () => {
    expect(useUIStore.getState().scanTabMode).toBe('rfid');
  });

  it('setScanTabMode switches to barcode', () => {
    useUIStore.getState().setScanTabMode('barcode');
    expect(useUIStore.getState().scanTabMode).toBe('barcode');
  });
});
