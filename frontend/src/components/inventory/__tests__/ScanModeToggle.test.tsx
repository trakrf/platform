import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { ScanModeToggle } from '../ScanModeToggle';
import { useUIStore } from '@/stores/uiStore';
import { useDeviceStore } from '@/stores/deviceStore';

afterEach(cleanup);

describe('ScanModeToggle (TRA-1031)', () => {
  beforeEach(() => {
    useUIStore.setState({ scanTabMode: 'rfid' });
    useDeviceStore.setState({ scanButtonActive: false });
  });

  it('renders with RFID selected by default', () => {
    render(<ScanModeToggle />);
    expect(screen.getByRole('button', { name: 'RFID' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Barcode' })).toHaveAttribute('aria-pressed', 'false');
  });

  it('clicking Barcode switches the mode', () => {
    render(<ScanModeToggle />);
    fireEvent.click(screen.getByRole('button', { name: 'Barcode' }));
    expect(useUIStore.getState().scanTabMode).toBe('barcode');
  });

  it('stops a running scan round before switching modes', () => {
    useDeviceStore.setState({ scanButtonActive: true });
    render(<ScanModeToggle />);
    fireEvent.click(screen.getByRole('button', { name: 'Barcode' }));
    expect(useDeviceStore.getState().scanButtonActive).toBe(false);
    expect(useUIStore.getState().scanTabMode).toBe('barcode');
  });

  it('re-clicking the active mode is a no-op', () => {
    useDeviceStore.setState({ scanButtonActive: true });
    render(<ScanModeToggle />);
    fireEvent.click(screen.getByRole('button', { name: 'RFID' }));
    expect(useDeviceStore.getState().scanButtonActive).toBe(true);
    expect(useUIStore.getState().scanTabMode).toBe('rfid');
  });
});
