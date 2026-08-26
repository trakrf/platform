import '@testing-library/jest-dom';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { ScanControls } from './ScanControls';
import { useDeviceStore } from '@/stores/deviceStore';
import { ReaderState } from '@/worker/types/reader';

/**
 * TRA-1177. Header.tsx and SettingsScreen.tsx both early-return on
 * !isBrowserSupported before calling connect(), and BrowserSupportBanner's
 * button is hardcoded disabled. ScanControls did neither, which made it the one
 * route by which a browser without Web Bluetooth reached TransportFactory —
 * and, before the transport collapse, a MockTransport streaming invented tags.
 */

const mockSupport = vi.hoisted(() => ({ supported: true }));

vi.mock('@/hooks/useBluetoothSupport', () => ({
  useBluetoothSupport: () => ({
    supported: mockSupport.supported,
    reason: mockSupport.supported ? null : 'unsupported-browser',
    recommendation: { browsers: 'Chrome, Edge, or Opera', note: '', links: [] },
    platform: 'macos',
    setupPrerequisite: null,
  }),
}));

afterEach(() => {
  cleanup();
  mockSupport.supported = true;
  useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });
});

describe('ScanControls connect button', () => {
  it('is disabled on a browser without Web Bluetooth', () => {
    mockSupport.supported = false;
    useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });

    render(<ScanControls />);

    expect(screen.getByTestId('kit-reconnect')).toBeDisabled();
  });

  it('stays enabled on a browser that supports Web Bluetooth', () => {
    mockSupport.supported = true;
    useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });

    render(<ScanControls />);

    // The gate must not disable the button for everyone — that would trade one
    // silent failure for a louder one.
    expect(screen.getByTestId('kit-reconnect')).toBeEnabled();
  });
});
